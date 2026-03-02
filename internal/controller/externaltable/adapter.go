package externaltable

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
	"github.com/hupe1980/snowplane/internal/tracked"
)

// adapter implements reconciler.ResourceAdapter for ExternalTable.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "externaltable" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.ExternalTable {
	return &snowplanev1alpha1.ExternalTable{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, et *snowplanev1alpha1.ExternalTable) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, et,
		et.Namespace, et.Spec.DatabaseRef, et.Spec.DatabaseName, et.Status.DatabaseName)
	if err != nil {
		return err
	}

	et.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, et,
		et.Namespace, et.Spec.SchemaRef, et.Spec.SchemaName, et.Status.SchemaName)
	if err != nil {
		return err
	}

	et.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(et, et.Spec.DatabaseRef, et.Spec.DatabaseName, et.Spec.SchemaRef, et.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(et *snowplanev1alpha1.ExternalTable) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, et.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.ExternalTable{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.ExternalTable)
				if !ok || s.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{s.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.ExternalTable{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.ExternalTable)
				if !ok || s.Spec.SchemaRef == nil {
					return nil
				}

				return []string{s.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalTableList{} }, ".spec.databaseRef.name", "listing external tables for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalTableList{} }, ".spec.schemaRef.name", "listing external tables for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.ExternalTableObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.ExternalTableObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.ExternalTable, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterExternalTableOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, et *snowplanev1alpha1.ExternalTable) error {
	if reconciler.ShouldSkipImmutableValidation(et) {
		return nil
	}

	if et.Status.ShowOutput != nil {
		if et.Status.ShowOutput.Name != "" && !strings.EqualFold(et.Spec.Name, et.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", et.Status.ShowOutput.Name, et.Spec.Name)
		}

		if et.Status.ShowOutput.DatabaseName != "" && et.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, et.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", et.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if et.Status.ShowOutput.SchemaName != "" && et.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, et.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", et.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.ExternalTable, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.ExternalTable, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.ExternalTable) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.ExternalTable, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return false }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] = (*adapter)(nil)
