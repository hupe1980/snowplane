package alert

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

// adapter implements reconciler.ResourceAdapter for Alert.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "alert" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Alert {
	return &snowplanev1alpha1.Alert{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, alert *snowplanev1alpha1.Alert) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, alert,
		alert.Namespace, alert.Spec.DatabaseRef, alert.Spec.DatabaseName, alert.Status.DatabaseName)
	if err != nil {
		return err
	}

	alert.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, alert,
		alert.Namespace, alert.Spec.SchemaRef, alert.Spec.SchemaName, alert.Status.SchemaName)
	if err != nil {
		return err
	}

	alert.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(alert, alert.Spec.DatabaseRef, alert.Spec.DatabaseName, alert.Spec.SchemaRef, alert.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(alert *snowplanev1alpha1.Alert) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(alert.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(alert.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, alert.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Alert{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				al, ok := o.(*snowplanev1alpha1.Alert)
				if !ok || al.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{al.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Alert{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				al, ok := o.(*snowplanev1alpha1.Alert)
				if !ok || al.Spec.SchemaRef == nil {
					return nil
				}

				return []string{al.Spec.SchemaRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.AlertList{} }, ".spec.databaseRef.name", "listing alerts for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.AlertList{} }, ".spec.schemaRef.name", "listing alerts for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.AlertObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.AlertObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Alert, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterAlertOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, alert *snowplanev1alpha1.Alert) error {
	if reconciler.ShouldSkipImmutableValidation(alert) {
		return nil
	}

	if alert.Status.ShowOutput != nil {
		if alert.Status.ShowOutput.Name != "" && !strings.EqualFold(alert.Spec.Name, alert.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", alert.Status.ShowOutput.Name, alert.Spec.Name)
		}

		if alert.Status.ShowOutput.DatabaseName != "" && alert.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(alert.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, alert.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", alert.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if alert.Status.ShowOutput.SchemaName != "" && alert.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(alert.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, alert.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", alert.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Alert, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.AlertObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Alert, obs *reconciler.Observation[*snowflake.AlertObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Alert) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Alert, obs *reconciler.Observation[*snowflake.AlertObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] = (*adapter)(nil)
