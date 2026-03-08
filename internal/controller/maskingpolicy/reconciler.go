// Package maskingpolicy implements the reconciler for MaskingPolicy resources.
package maskingpolicy

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
	finalizerName = "snowplane.hupe1980.github.io/maskingpolicy"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake masking policies.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error)
	Create(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error
	Alter(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new MaskingPolicy reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewMaskingPolicyClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for MaskingPolicy resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation]{
		ResourceNameVal:  "maskingpolicy",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.MaskingPolicy { return &snowplanev1alpha1.MaskingPolicy{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(mp *snowplanev1alpha1.MaskingPolicy) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(mp.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(mp.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, mp.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.MaskingPolicyObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterMaskingPolicyOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.MaskingPolicyObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.MaskingPolicy, obs *reconciler.Observation[*snowflake.MaskingPolicyObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.MaskingPolicy, obs *reconciler.Observation[*snowflake.MaskingPolicyObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		SupportsCoA: false,
		PreReconcileFn: func(ctx context.Context, mp *snowplanev1alpha1.MaskingPolicy) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, mp,
				mp.Namespace, mp.Spec.DatabaseRef, mp.Spec.DatabaseName, mp.Status.DatabaseName)
			if err != nil {
				return err
			}

			mp.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, mp,
				mp.Namespace, mp.Spec.SchemaRef, mp.Spec.SchemaName, mp.Status.SchemaName)
			if err != nil {
				return err
			}

			mp.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(mp, mp.Spec.DatabaseRef, mp.Spec.DatabaseName, mp.Spec.SchemaRef, mp.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.MaskingPolicy{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					mp, ok := o.(*snowplanev1alpha1.MaskingPolicy)
					if !ok || mp.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{mp.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.MaskingPolicy{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					mp, ok := o.(*snowplanev1alpha1.MaskingPolicy)
					if !ok || mp.Spec.SchemaRef == nil {
						return nil
					}

					return []string{mp.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.MaskingPolicyList{} }, ".spec.databaseRef.name", "listing masking policies for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.MaskingPolicyList{} }, ".spec.schemaRef.name", "listing masking policies for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, mp *snowplanev1alpha1.MaskingPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(mp) {
		return nil
	}

	if mp.Status.ShowOutput != nil {
		if mp.Status.ShowOutput.Name != "" && !strings.EqualFold(mp.Spec.Name, mp.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", mp.Status.ShowOutput.Name, mp.Spec.Name)
		}

		if mp.Status.ShowOutput.DatabaseName != "" && mp.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(mp.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, mp.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", mp.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if mp.Status.ShowOutput.SchemaName != "" && mp.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(mp.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, mp.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", mp.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(mp *snowplanev1alpha1.MaskingPolicy, obs *snowflake.MaskingPolicyObservation) {
	if obs.ShowOutput != nil {
		mp.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()

		mp.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(mp *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier) snowflake.CreateMaskingPolicyOptions {
	sig := make([]snowflake.MaskingPolicyArgument, len(mp.Spec.Signature))
	for i, arg := range mp.Spec.Signature {
		sig[i] = snowflake.MaskingPolicyArgument{
			Name: arg.Name,
			Type: arg.Type,
		}
	}

	return snowflake.CreateMaskingPolicyOptions{
		Name:                id,
		Signature:           sig,
		Body:                mp.Spec.Body,
		ExemptOtherPolicies: mp.Spec.ExemptOtherPolicies,
		Comment:             mp.Spec.Comment,
	}
}

func buildAlterOptions(mp *snowplanev1alpha1.MaskingPolicy, id snowflake.SchemaObjectIdentifier, obs *snowflake.MaskingPolicyObservation) snowflake.AlterMaskingPolicyOptions {
	opts := snowflake.AlterMaskingPolicyOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&mp.Spec, mp.Status.TrackedParameters)

	// Body is always sent to ensure convergence (not in SHOW output).
	body := mp.Spec.Body
	opts.Body = &body

	if mp.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *mp.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = mp.Spec.Comment
		}
	}

	return opts
}

func detectDrift(mp *snowplanev1alpha1.MaskingPolicy, obs *snowflake.MaskingPolicyObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", mp.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", mp.Spec.Comment, obs.ShowOutput.Comment, false)

		// Note: Body is not available in SHOW output, so drift detection for body
		// relies on spec-hash comparison. Comment drift is detectable from SHOW.
	}

	return d.Result()
}
