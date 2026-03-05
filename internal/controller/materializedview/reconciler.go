// Package materializedview implements the reconciler for MaterializedView resources.
package materializedview

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
	finalizerName = "snowplane.hupe1980.github.io/materializedview"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake materialized views.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error)
	Create(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error
	Alter(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new MaterializedView reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewMaterializedViewClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for MaterializedView resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation]{
		ResourceNameVal:  "materializedview",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.MaterializedView { return &snowplanev1alpha1.MaterializedView{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(mv *snowplanev1alpha1.MaterializedView) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(mv.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(mv.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, mv.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.MaterializedViewObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterMaterializedViewOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.MaterializedViewObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.MaterializedView, obs *reconciler.Observation[*snowflake.MaterializedViewObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.MaterializedView, obs *reconciler.Observation[*snowflake.MaterializedViewObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, mv *snowplanev1alpha1.MaterializedView) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, mv,
				mv.Namespace, mv.Spec.DatabaseRef, mv.Spec.DatabaseName, mv.Status.DatabaseName)
			if err != nil {
				return err
			}

			mv.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, mv,
				mv.Namespace, mv.Spec.SchemaRef, mv.Spec.SchemaName, mv.Status.SchemaName)
			if err != nil {
				return err
			}

			mv.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(mv, mv.Spec.DatabaseRef, mv.Spec.DatabaseName, mv.Spec.SchemaRef, mv.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.MaterializedView{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					mv, ok := o.(*snowplanev1alpha1.MaterializedView)
					if !ok || mv.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{mv.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.MaterializedView{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					mv, ok := o.(*snowplanev1alpha1.MaterializedView)
					if !ok || mv.Spec.SchemaRef == nil {
						return nil
					}

					return []string{mv.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.MaterializedViewList{} }, ".spec.databaseRef.name", "listing materialized views for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.MaterializedViewList{} }, ".spec.schemaRef.name", "listing materialized views for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, mv *snowplanev1alpha1.MaterializedView) error {
	if reconciler.ShouldSkipImmutableValidation(mv) {
		return nil
	}

	if mv.Status.ShowOutput != nil {
		if mv.Status.ShowOutput.Name != "" && !strings.EqualFold(mv.Spec.Name, mv.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", mv.Status.ShowOutput.Name, mv.Spec.Name)
		}

		if mv.Status.ShowOutput.DatabaseName != "" && mv.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(mv.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, mv.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", mv.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if mv.Status.ShowOutput.SchemaName != "" && mv.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(mv.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, mv.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", mv.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(mv *snowplanev1alpha1.MaterializedView, obs *snowflake.MaterializedViewObservation) {
	if obs.ShowOutput != nil {
		mv.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		mv.Status.DatabaseName = obs.ShowOutput.DatabaseName
		mv.Status.SchemaName = obs.ShowOutput.SchemaName

		mv.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(mv *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier) snowflake.CreateMaterializedViewOptions {
	return snowflake.CreateMaterializedViewOptions{
		Name:      id,
		Statement: mv.Spec.Statement,
		Secure:    mv.Spec.Secure,
		Comment:   mv.Spec.Comment,
		ClusterBy: mv.Spec.ClusterBy,
	}
}

func buildAlterOptions(mv *snowplanev1alpha1.MaterializedView, id snowflake.SchemaObjectIdentifier, obs *snowflake.MaterializedViewObservation) snowflake.AlterMaterializedViewOptions {
	opts := snowflake.AlterMaterializedViewOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&mv.Spec, mv.Status.TrackedParameters)

	if mv.Spec.Comment != nil {
		if obs.ShowOutput == nil || *mv.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = mv.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Secure toggle: compare bool values.
		desiredSecure := mv.Spec.Secure
		observedSecure := strings.EqualFold(obs.ShowOutput.IsSecure, "true")

		if desiredSecure != observedSecure {
			opts.Secure = &desiredSecure
		}
	}

	return opts
}

func detectDrift(mv *snowplanev1alpha1.MaterializedView, obs *snowflake.MaterializedViewObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", mv.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(mv.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(mv.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("STATEMENT", mv.Spec.Statement, obs.ShowOutput.Text, true)

		// Mutable fields.
		d.CompareString("COMMENT", mv.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareBoolValue("IS_SECURE", mv.Spec.Secure, strings.EqualFold(obs.ShowOutput.IsSecure, "true"), false)
	}

	return d.Result()
}
