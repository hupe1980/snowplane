// Package networkrule implements the reconciler for NetworkRule resources.
package networkrule

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/networkrule"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake network rules.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error)
	Create(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error
	Alter(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new NetworkRule reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewNetworkRuleClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for NetworkRule resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation]{
		ResourceNameVal:  "networkrule",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.NetworkRule { return &snowplanev1alpha1.NetworkRule{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(nr *snowplanev1alpha1.NetworkRule) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(nr.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(nr.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, nr.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.NetworkRuleObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterNetworkRuleOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.NetworkRule, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.NetworkRule, obs *reconciler.Observation[*snowflake.NetworkRuleObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA: false,
		PreReconcileFn: func(ctx context.Context, nr *snowplanev1alpha1.NetworkRule) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, nr,
				nr.Namespace, nr.Spec.DatabaseRef, nr.Spec.DatabaseName, nr.Status.DatabaseName)
			if err != nil {
				return err
			}

			nr.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, nr,
				nr.Namespace, nr.Spec.SchemaRef, nr.Spec.SchemaName, nr.Status.SchemaName)
			if err != nil {
				return err
			}

			nr.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(nr, nr.Spec.DatabaseRef, nr.Spec.DatabaseName, nr.Spec.SchemaRef, nr.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.NetworkRule{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					nr, ok := o.(*snowplanev1alpha1.NetworkRule)
					if !ok || nr.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{nr.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.NetworkRule{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					nr, ok := o.(*snowplanev1alpha1.NetworkRule)
					if !ok || nr.Spec.SchemaRef == nil {
						return nil
					}

					return []string{nr.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.NetworkRuleList{} }, ".spec.databaseRef.name", "listing network rules for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.NetworkRuleList{} }, ".spec.schemaRef.name", "listing network rules for schema watch")),
			)

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not been changed.
func validateImmutableFields(_ context.Context, nr *snowplanev1alpha1.NetworkRule) error {
	if reconciler.ShouldSkipImmutableValidation(nr) {
		return nil
	}

	if nr.Status.ShowOutput != nil {
		if nr.Status.ShowOutput.Name != "" && !strings.EqualFold(nr.Spec.Name, nr.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Name, nr.Spec.Name)
		}

		if nr.Status.ShowOutput.DatabaseName != "" && nr.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(nr.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, nr.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", nr.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if nr.Status.ShowOutput.SchemaName != "" && nr.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(nr.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, nr.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", nr.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		if nr.Status.ShowOutput.Type != "" && !strings.EqualFold(string(nr.Spec.Type), nr.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Type, nr.Spec.Type)
		}

		if nr.Status.ShowOutput.Mode != "" && !strings.EqualFold(string(nr.Spec.Mode), nr.Status.ShowOutput.Mode) {
			return fmt.Errorf("spec.mode is immutable after creation (current: %q, desired: %q)", nr.Status.ShowOutput.Mode, nr.Spec.Mode)
		}
	}

	return nil
}

func applyObservation(nr *snowplanev1alpha1.NetworkRule, obs *snowflake.NetworkRuleObservation) {
	if obs.ShowOutput != nil {
		nr.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		nr.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(nr *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier) snowflake.CreateNetworkRuleOptions {
	return snowflake.CreateNetworkRuleOptions{
		Name:      id,
		Type:      string(nr.Spec.Type),
		Mode:      string(nr.Spec.Mode),
		ValueList: nr.Spec.ValueList,
		Comment:   nr.Spec.Comment,
	}
}

func buildAlterOptions(nr *snowplanev1alpha1.NetworkRule, id snowflake.SchemaObjectIdentifier, obs *snowflake.NetworkRuleObservation) snowflake.AlterNetworkRuleOptions {
	opts := snowflake.AlterNetworkRuleOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&nr.Spec, nr.Status.TrackedParameters)

	// ValueList: compare against DESCRIBE output when available.
	if obs != nil && obs.DescribeOutput != nil {
		if descValues, ok := obs.DescribeOutput["value_list"]; ok {
			specSorted := sortedValues(nr.Spec.ValueList)
			obsSorted := sortedValues(strings.Split(descValues, ","))
			if !strings.EqualFold(specSorted, obsSorted) {
				valueList := make([]string, len(nr.Spec.ValueList))
				copy(valueList, nr.Spec.ValueList)
				opts.ValueList = &valueList
			}
		} else {
			// DESCRIBE available but value_list key missing — send to converge.
			valueList := make([]string, len(nr.Spec.ValueList))
			copy(valueList, nr.Spec.ValueList)
			opts.ValueList = &valueList
		}
	} else {
		// No DESCRIBE output — send unconditionally to converge.
		valueList := make([]string, len(nr.Spec.ValueList))
		copy(valueList, nr.Spec.ValueList)
		opts.ValueList = &valueList
	}

	if nr.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *nr.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = nr.Spec.Comment
		}
	}

	return opts
}

func detectDrift(nr *snowplanev1alpha1.NetworkRule, obs *snowflake.NetworkRuleObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", nr.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("TYPE", string(nr.Spec.Type), obs.ShowOutput.Type, true)
		d.CompareStringValueFold("MODE", string(nr.Spec.Mode), obs.ShowOutput.Mode, true)

		// Mutable fields.
		d.CompareString("COMMENT", nr.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: VALUE_LIST is not available in SHOW output; drift detection for
		// value list relies on DESCRIBE output or spec-hash comparison.
	}

	// If DESCRIBE output contains value_list, compare it (sorted for order-independence).
	if obs.DescribeOutput != nil {
		if descValues, ok := obs.DescribeOutput["value_list"]; ok {
			specSorted := sortedValues(nr.Spec.ValueList)
			obsSorted := sortedValues(strings.Split(descValues, ","))
			d.CompareStringValueFold("VALUE_LIST", specSorted, obsSorted, false)
		}
	}

	return d.Result()
}

// sortedValues returns a sorted, comma-joined canonical string for order-independent comparison.
func sortedValues(vals []string) string {
	if len(vals) == 0 {
		return ""
	}

	sorted := make([]string, len(vals))
	copy(sorted, vals)

	// Trim whitespace from each value before sorting.
	for i := range sorted {
		sorted[i] = strings.TrimSpace(sorted[i])
	}

	sort.Strings(sorted)

	return strings.Join(sorted, ",")
}
