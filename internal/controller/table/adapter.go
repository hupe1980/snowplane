package table

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

// adapter implements reconciler.ResourceAdapter for Table.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "table" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, table *snowplanev1alpha1.Table) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, table,
		table.Namespace, table.Spec.DatabaseRef, table.Spec.DatabaseName, table.Status.DatabaseName)
	if err != nil {
		return err
	}

	table.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, table,
		table.Namespace, table.Spec.SchemaRef, table.Spec.SchemaName, table.Status.SchemaName)
	if err != nil {
		return err
	}

	table.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(table, table.Spec.DatabaseRef, table.Spec.DatabaseName, table.Spec.SchemaRef, table.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(table *snowplanev1alpha1.Table) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(table.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(table.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, table.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Table{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Table)
				if !ok || t.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{t.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Table{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Table)
				if !ok || t.Spec.SchemaRef == nil {
					return nil
				}

				return []string{t.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.databaseRef.name", "listing tables for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.schemaRef.name", "listing tables for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TableObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.TableObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Table, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterTableOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, table *snowplanev1alpha1.Table) error {
	if reconciler.ShouldSkipImmutableValidation(table) {
		return nil
	}

	if table.Status.ShowOutput != nil {
		if table.Status.ShowOutput.Name != "" && !strings.EqualFold(table.Spec.Name, table.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", table.Status.ShowOutput.Name, table.Spec.Name)
		}

		isTransient := table.Status.ShowOutput.Kind == "TRANSIENT"
		if table.Spec.Transient != isTransient {
			return fmt.Errorf("spec.transient is immutable after creation (current: %v, desired: %v)", isTransient, table.Spec.Transient)
		}

		if table.Status.ShowOutput.DatabaseName != "" && table.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(table.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, table.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", table.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if table.Status.ShowOutput.SchemaName != "" && table.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(table.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, table.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", table.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Table, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.TableObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Table, obs *reconciler.Observation[*snowflake.TableObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Table) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Table, obs *reconciler.Observation[*snowflake.TableObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.Table)                                    {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.Table, _ bool, _ reconciler.AlterOptions) {}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] = (*adapter)(nil)
