// Package rowaccesspolicy implements the reconciler for RowAccessPolicy resources.
package rowaccesspolicy

import (
	"context"
	"fmt"
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
	finalizerName = "snowplane.hupe1980.github.io/rowaccesspolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake row access policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new RowAccessPolicy reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewRowAccessPolicyClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for RowAccessPolicy resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation]{
		ResourceNameVal:  "rowaccesspolicy",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.RowAccessPolicy { return &snowplanev1alpha1.RowAccessPolicy{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(rap *snowplanev1alpha1.RowAccessPolicy) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(rap.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(rap.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, rap.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.RowAccessPolicyObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterRowAccessPolicyOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.RowAccessPolicy, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.RowAccessPolicy, obs *reconciler.Observation[*snowflake.RowAccessPolicyObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA: true,
		PreReconcileFn: func(ctx context.Context, rap *snowplanev1alpha1.RowAccessPolicy) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, rap,
				rap.Namespace, rap.Spec.DatabaseRef, rap.Spec.DatabaseName, rap.Status.DatabaseName)
			if err != nil {
				return err
			}

			rap.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, rap,
				rap.Namespace, rap.Spec.SchemaRef, rap.Spec.SchemaName, rap.Status.SchemaName)
			if err != nil {
				return err
			}

			rap.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(rap, rap.Spec.DatabaseRef, rap.Spec.DatabaseName, rap.Spec.SchemaRef, rap.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.RowAccessPolicy{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					rap, ok := o.(*snowplanev1alpha1.RowAccessPolicy)
					if !ok || rap.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{rap.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.RowAccessPolicy{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					rap, ok := o.(*snowplanev1alpha1.RowAccessPolicy)
					if !ok || rap.Spec.SchemaRef == nil {
						return nil
					}

					return []string{rap.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.RowAccessPolicyList{} }, ".spec.databaseRef.name", "listing row access policies for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.RowAccessPolicyList{} }, ".spec.schemaRef.name", "listing row access policies for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, rap *snowplanev1alpha1.RowAccessPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(rap) {
		return nil
	}

	if rap.Status.ShowOutput != nil {
		if rap.Status.ShowOutput.Name != "" && !strings.EqualFold(rap.Spec.Name, rap.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", rap.Status.ShowOutput.Name, rap.Spec.Name)
		}

		if rap.Status.ShowOutput.DatabaseName != "" && rap.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(rap.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, rap.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", rap.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if rap.Status.ShowOutput.SchemaName != "" && rap.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(rap.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, rap.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", rap.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(rap *snowplanev1alpha1.RowAccessPolicy, obs *snowflake.RowAccessPolicyObservation) {
	if obs.ShowOutput != nil {
		rap.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		rap.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(rap *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreateRowAccessPolicyOptions {
	sig := make([]snowflake.RowAccessPolicyArgument, len(rap.Spec.Signature))
	for i, arg := range rap.Spec.Signature {
		sig[i] = snowflake.RowAccessPolicyArgument{
			Name: arg.Name,
			Type: arg.Type,
		}
	}

	return snowflake.CreateRowAccessPolicyOptions{
		Name:      id,
		Signature: sig,
		Body:      rap.Spec.Body,
		Comment:   rap.Spec.Comment,
	}
}

func buildAlterOptions(rap *snowplanev1alpha1.RowAccessPolicy, id snowflake.SchemaObjectIdentifier, obs *snowflake.RowAccessPolicyObservation) snowflake.AlterRowAccessPolicyOptions {
	opts := snowflake.AlterRowAccessPolicyOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&rap.Spec, rap.Status.TrackedParameters)

	// Body is always sent to ensure convergence (not in SHOW output).
	body := rap.Spec.Body
	opts.Body = &body

	if rap.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *rap.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = rap.Spec.Comment
		}
	}

	return opts
}

func detectDrift(rap *snowplanev1alpha1.RowAccessPolicy, obs *snowflake.RowAccessPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", rap.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", rap.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: Body is not available in SHOW output, so drift detection for body
		// relies on spec-hash comparison.
	}

	return d.Result()
}
