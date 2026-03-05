// Package alert implements the reconciler for Alert resources.
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
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/alert"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake alerts.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAlertOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAlertOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Alert reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewAlertClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Alert resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation]{
		ResourceNameVal:  "alert",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Alert { return &snowplanev1alpha1.Alert{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(alert *snowplanev1alpha1.Alert) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(alert.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(alert.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, alert.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.AlertObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterAlertOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.AlertObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Alert, obs *reconciler.Observation[*snowflake.AlertObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Alert, obs *reconciler.Observation[*snowflake.AlertObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, alert *snowplanev1alpha1.Alert) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, alert,
				alert.Namespace, alert.Spec.DatabaseRef, alert.Spec.DatabaseName, alert.Status.DatabaseName)
			if err != nil {
				return err
			}

			alert.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, alert,
				alert.Namespace, alert.Spec.SchemaRef, alert.Spec.SchemaName, alert.Status.SchemaName)
			if err != nil {
				return err
			}

			alert.Status.SchemaName = schemaFQN

			// Resolve optional warehouse ref.
			if alert.Spec.WarehouseRef != nil || alert.Spec.WarehouseName != nil {
				whName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, alert,
					alert.Namespace, alert.Spec.WarehouseRef, alert.Spec.WarehouseName, alert.Status.WarehouseName,
					"Warehouse",
					func() *snowplanev1alpha1.Warehouse { return &snowplanev1alpha1.Warehouse{} },
					snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
					func(w *snowplanev1alpha1.Warehouse) string { return w.Spec.Name },
				)
				if err != nil {
					return err
				}

				alert.Status.WarehouseName = whName
			}

			refs := []refresolver.RefDescriptor{
				{KindLabel: "Database", Ref: alert.Spec.DatabaseRef, RawName: alert.Spec.DatabaseName},
				{KindLabel: "Schema", Ref: alert.Spec.SchemaRef, RawName: alert.Spec.SchemaName},
			}
			if alert.Spec.WarehouseRef != nil || alert.Spec.WarehouseName != nil {
				refs = append(refs, refresolver.RefDescriptor{KindLabel: "Warehouse", Ref: alert.Spec.WarehouseRef, RawName: alert.Spec.WarehouseName})
			}

			refresolver.SetAllReferencesResolvedCondition(alert, refs...)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.AlertList{} }, ".spec.databaseRef.name", "listing alerts for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.AlertList{} }, ".spec.schemaRef.name", "listing alerts for schema watch")),
			)

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Alert{},
				".spec.warehouseRef.name",
				func(o sigs.Object) []string {
					al, ok := o.(*snowplanev1alpha1.Alert)
					if !ok || al.Spec.WarehouseRef == nil {
						return nil
					}

					return []string{al.Spec.WarehouseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.warehouseRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Warehouse{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.AlertList{} }, ".spec.warehouseRef.name", "listing alerts for warehouse watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, alert *snowplanev1alpha1.Alert) error {
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

func applyObservation(alert *snowplanev1alpha1.Alert, obs *snowflake.AlertObservation) {
	if obs.ShowOutput != nil {
		alert.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		alert.Status.DatabaseName = obs.ShowOutput.DatabaseName
		alert.Status.SchemaName = obs.ShowOutput.SchemaName

		alert.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(alert *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier) snowflake.CreateAlertOptions {
	opts := snowflake.CreateAlertOptions{
		Name:      id,
		Schedule:  alert.Spec.Schedule,
		Comment:   alert.Spec.Comment,
		Condition: alert.Spec.Condition,
		Action:    alert.Spec.Action,
	}

	if alert.Status.WarehouseName != "" {
		wh := alert.Status.WarehouseName
		opts.Warehouse = &wh
	}

	return opts
}

func buildAlterOptions(alert *snowplanev1alpha1.Alert, id snowflake.SchemaObjectIdentifier, obs *snowflake.AlertObservation) snowflake.AlterAlertOptions {
	opts := snowflake.AlterAlertOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&alert.Spec, alert.Status.TrackedParameters)

	if alert.Spec.Comment != nil {
		if obs.ShowOutput == nil || *alert.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = alert.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Pass current state so buildAlterAlertStatements can auto-suspend
		// before modifying condition/action/schedule/warehouse.
		opts.CurrentState = obs.ShowOutput.State

		// Schedule changes.
		if alert.Spec.Schedule != nil && *alert.Spec.Schedule != obs.ShowOutput.Schedule {
			opts.Schedule = alert.Spec.Schedule
		}

		// Warehouse changes.
		if alert.Status.WarehouseName != "" && alert.Status.WarehouseName != obs.ShowOutput.Warehouse {
			wh := alert.Status.WarehouseName
			opts.Warehouse = &wh
		}

		// Condition changes.
		if alert.Spec.Condition != obs.ShowOutput.Condition {
			cond := alert.Spec.Condition
			opts.Condition = &cond
		}

		// Action changes.
		if alert.Spec.Action != obs.ShowOutput.Action {
			action := alert.Spec.Action
			opts.Action = &action
		}

		// Suspend/resume state changes.
		if alert.Spec.Suspend != nil {
			isSuspended := obs.ShowOutput.State == "suspended"
			if *alert.Spec.Suspend != isSuspended {
				opts.Suspend = alert.Spec.Suspend
			}
		}
	}

	return opts
}

func detectDrift(alert *snowplanev1alpha1.Alert, obs *snowflake.AlertObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", alert.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(alert.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(alert.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields from SHOW output.
		d.CompareString("COMMENT", alert.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("SCHEDULE", alert.Spec.Schedule, obs.ShowOutput.Schedule, false)

		var warehousePtr *string
		if alert.Status.WarehouseName != "" {
			wh := alert.Status.WarehouseName
			warehousePtr = &wh
		}

		d.CompareString("WAREHOUSE", warehousePtr, obs.ShowOutput.Warehouse, false)
		d.CompareStringValue("CONDITION", alert.Spec.Condition, obs.ShowOutput.Condition, false)
		d.CompareStringValue("ACTION", alert.Spec.Action, obs.ShowOutput.Action, false)
	}

	return d.Result()
}
