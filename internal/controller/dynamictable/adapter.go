package dynamictable

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

// adapter implements reconciler.ResourceAdapter for DynamicTable.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "dynamictable" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.DynamicTable {
	return &snowplanev1alpha1.DynamicTable{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, dt *snowplanev1alpha1.DynamicTable) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, dt,
		dt.Namespace, dt.Spec.DatabaseRef, dt.Spec.DatabaseName, dt.Status.DatabaseName)
	if err != nil {
		return err
	}

	dt.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, dt,
		dt.Namespace, dt.Spec.SchemaRef, dt.Spec.SchemaName, dt.Status.SchemaName)
	if err != nil {
		return err
	}

	dt.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(dt, dt.Spec.DatabaseRef, dt.Spec.DatabaseName, dt.Spec.SchemaRef, dt.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(dt *snowplanev1alpha1.DynamicTable) reconciler.Identifier {
	dbName := snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, dt.Spec.Name)
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.DynamicTable{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				dt, ok := o.(*snowplanev1alpha1.DynamicTable)
				if !ok || dt.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{dt.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.DynamicTable{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				dt, ok := o.(*snowplanev1alpha1.DynamicTable)
				if !ok || dt.Spec.SchemaRef == nil {
					return nil
				}

				return []string{dt.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }, ".spec.databaseRef.name", "listing dynamic tables for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }, ".spec.schemaRef.name", "listing dynamic tables for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.DynamicTableObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.DynamicTableObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.DynamicTable, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterDynamicTableOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, dt *snowplanev1alpha1.DynamicTable) error {
	if reconciler.ShouldSkipImmutableValidation(dt) {
		return nil
	}

	if dt.Status.ShowOutput != nil {
		if dt.Status.ShowOutput.Name != "" && !strings.EqualFold(dt.Spec.Name, dt.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.Name, dt.Spec.Name)
		}

		if dt.Status.ShowOutput.DatabaseName != "" && dt.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, dt.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", dt.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if dt.Status.ShowOutput.SchemaName != "" && dt.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, dt.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", dt.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		// query is immutable — defines the dynamic table's SQL content.
		if dt.Status.ShowOutput.Text != "" && dt.Spec.Query != dt.Status.ShowOutput.Text {
			return fmt.Errorf("spec.query is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.Text, dt.Spec.Query)
		}

		// refreshMode is immutable — set at CREATE time only.
		if dt.Status.ShowOutput.RefreshMode != "" && dt.Spec.RefreshMode != nil {
			if !strings.EqualFold(string(*dt.Spec.RefreshMode), dt.Status.ShowOutput.RefreshMode) {
				return fmt.Errorf("spec.refreshMode is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.RefreshMode, *dt.Spec.RefreshMode)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.DynamicTable, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.DynamicTable, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.DynamicTable) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.DynamicTable, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.DynamicTable) {}

func (a *adapter) PostUpdate(_ *snowplanev1alpha1.DynamicTable, _ bool, _ reconciler.AlterOptions) {}

func (a *adapter) SupportsCreateOrAlter() bool { return false }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] = (*adapter)(nil)
