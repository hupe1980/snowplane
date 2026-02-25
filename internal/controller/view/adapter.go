package view

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
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for View.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "view" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, view *snowplanev1alpha1.View) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, view,
		view.Namespace, view.Spec.DatabaseRef, view.Spec.DatabaseName, view.Status.DatabaseName)
	if err != nil {
		return err
	}

	view.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, view,
		view.Namespace, view.Spec.SchemaRef, view.Spec.SchemaName, view.Status.SchemaName)
	if err != nil {
		return err
	}

	view.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(view, view.Spec.DatabaseRef, view.Spec.DatabaseName, view.Spec.SchemaRef, view.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(view *snowplanev1alpha1.View) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, view.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.View{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				v, ok := o.(*snowplanev1alpha1.View)
				if !ok || v.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{v.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.View{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				v, ok := o.(*snowplanev1alpha1.View)
				if !ok || v.Spec.SchemaRef == nil {
					return nil
				}

				return []string{v.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.databaseRef.name", "listing views for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.schemaRef.name", "listing views for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.ViewObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.ViewObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.View, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterViewOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, sid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, view *snowplanev1alpha1.View) error {
	if reconciler.ShouldSkipImmutableValidation(view) {
		return nil
	}

	if view.Status.ShowOutput != nil {
		if view.Status.ShowOutput.Name != "" && !strings.EqualFold(view.Spec.Name, view.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", view.Status.ShowOutput.Name, view.Spec.Name)
		}

		if view.Status.ShowOutput.DatabaseName != "" && view.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(view.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, view.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", view.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if view.Status.ShowOutput.SchemaName != "" && view.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(view.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, view.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", view.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.View, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.ViewObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.View, obs *reconciler.Observation[*snowflake.ViewObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.View) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.View, obs *reconciler.Observation[*snowflake.ViewObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.View) {}

func (a *adapter) PostUpdate(_ *snowplanev1alpha1.View, specChanged bool, opts reconciler.AlterOptions) {
	// No-op: statement changes are handled transparently in buildAlterOptions
	// via ReplaceStatement, which triggers CREATE OR REPLACE VIEW in the
	// Snowflake client layer (R9-1).
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] = (*adapter)(nil)
