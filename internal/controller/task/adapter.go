package task

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

// adapter implements reconciler.ResourceAdapter for Task.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "task" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Task {
	return &snowplanev1alpha1.Task{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, task *snowplanev1alpha1.Task) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, task,
		task.Namespace, task.Spec.DatabaseRef, task.Spec.DatabaseName, task.Status.DatabaseName)
	if err != nil {
		return err
	}

	task.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, task,
		task.Namespace, task.Spec.SchemaRef, task.Spec.SchemaName, task.Status.SchemaName)
	if err != nil {
		return err
	}

	task.Status.SchemaName = schemaFQN

	// Resolve optional warehouse ref.
	if task.Spec.WarehouseRef != nil || task.Spec.WarehouseName != nil {
		whName, err := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, task,
			task.Namespace, task.Spec.WarehouseRef, task.Spec.WarehouseName, task.Status.WarehouseName,
			"Warehouse",
			func() *snowplanev1alpha1.Warehouse { return &snowplanev1alpha1.Warehouse{} },
			snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
			func(w *snowplanev1alpha1.Warehouse) string { return w.Spec.Name },
		)
		if err != nil {
			return err
		}

		task.Status.WarehouseName = whName
	}

	// Resolve optional error integration ref.
	if task.Spec.ErrorIntegrationRef != nil || task.Spec.ErrorIntegrationName != nil {
		eiName, err := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, task,
			task.Namespace, task.Spec.ErrorIntegrationRef, task.Spec.ErrorIntegrationName, task.Status.ErrorIntegrationName,
			"ErrorIntegration",
			func() *snowplanev1alpha1.NotificationIntegration {
				return &snowplanev1alpha1.NotificationIntegration{}
			},
			snowplanev1alpha1.GroupVersion.WithKind("NotificationIntegration"),
			func(ni *snowplanev1alpha1.NotificationIntegration) string { return ni.Spec.Name },
		)
		if err != nil {
			return err
		}

		task.Status.ErrorIntegrationName = eiName
	}

	// Resolve optional success integration ref.
	if task.Spec.SuccessIntegrationRef != nil || task.Spec.SuccessIntegrationName != nil {
		siName, err := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, task,
			task.Namespace, task.Spec.SuccessIntegrationRef, task.Spec.SuccessIntegrationName, task.Status.SuccessIntegrationName,
			"SuccessIntegration",
			func() *snowplanev1alpha1.NotificationIntegration {
				return &snowplanev1alpha1.NotificationIntegration{}
			},
			snowplanev1alpha1.GroupVersion.WithKind("NotificationIntegration"),
			func(ni *snowplanev1alpha1.NotificationIntegration) string { return ni.Spec.Name },
		)
		if err != nil {
			return err
		}

		task.Status.SuccessIntegrationName = siName
	}

	// Resolve optional finalize task ref.
	if task.Spec.FinalizeRef != nil || task.Spec.FinalizeName != nil {
		fnName, err := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, task,
			task.Namespace, task.Spec.FinalizeRef, task.Spec.FinalizeName, task.Status.FinalizeName,
			"Finalize",
			func() *snowplanev1alpha1.Task { return &snowplanev1alpha1.Task{} },
			snowplanev1alpha1.GroupVersion.WithKind("Task"),
			func(t *snowplanev1alpha1.Task) string { return t.Spec.Name },
		)
		if err != nil {
			return err
		}

		task.Status.FinalizeName = fnName
	}

	// Resolve After predecessor refs.
	if len(task.Spec.After) > 0 {
		afterNames := make([]string, 0, len(task.Spec.After))

		for i, pred := range task.Spec.After {
			predName, afterErr := refresolver.PreReconcileSourceRef(ctx, a.client, a.recorder, task,
				task.Namespace, pred.Ref, pred.Name, cachedAfterName(task.Status.AfterNames, i),
				fmt.Sprintf("After[%d]", i),
				func() *snowplanev1alpha1.Task { return &snowplanev1alpha1.Task{} },
				snowplanev1alpha1.GroupVersion.WithKind("Task"),
				func(t *snowplanev1alpha1.Task) string { return t.Spec.Name },
			)
			if afterErr != nil {
				return afterErr
			}

			afterNames = append(afterNames, predName)
		}

		task.Status.AfterNames = afterNames
	} else {
		task.Status.AfterNames = nil
	}

	refs := []refresolver.RefDescriptor{
		{KindLabel: "Database", Ref: task.Spec.DatabaseRef, RawName: task.Spec.DatabaseName},
		{KindLabel: "Schema", Ref: task.Spec.SchemaRef, RawName: task.Spec.SchemaName},
	}
	if task.Spec.WarehouseRef != nil || task.Spec.WarehouseName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "Warehouse", Ref: task.Spec.WarehouseRef, RawName: task.Spec.WarehouseName})
	}
	if task.Spec.ErrorIntegrationRef != nil || task.Spec.ErrorIntegrationName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "ErrorIntegration", Ref: task.Spec.ErrorIntegrationRef, RawName: task.Spec.ErrorIntegrationName})
	}
	if task.Spec.SuccessIntegrationRef != nil || task.Spec.SuccessIntegrationName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "SuccessIntegration", Ref: task.Spec.SuccessIntegrationRef, RawName: task.Spec.SuccessIntegrationName})
	}
	if task.Spec.FinalizeRef != nil || task.Spec.FinalizeName != nil {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: "Finalize", Ref: task.Spec.FinalizeRef, RawName: task.Spec.FinalizeName})
	}
	for i, pred := range task.Spec.After {
		refs = append(refs, refresolver.RefDescriptor{KindLabel: fmt.Sprintf("After[%d]", i), Ref: pred.Ref, RawName: pred.Name})
	}

	refresolver.SetAllReferencesResolvedCondition(task, refs...)

	return nil
}

func (a *adapter) BuildIdentifier(task *snowplanev1alpha1.Task) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(task.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(task.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, task.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
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
			&snowplanev1alpha1.Task{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
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
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.databaseRef.name", "listing tasks for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.schemaRef.name", "listing tasks for schema watch")),
		)

		// Warehouse ref indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.warehouseRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
				if !ok || t.Spec.WarehouseRef == nil {
					return nil
				}

				return []string{t.Spec.WarehouseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.warehouseRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Warehouse{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.warehouseRef.name", "listing tasks for warehouse watch")),
		)

		// ErrorIntegration ref indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.errorIntegrationRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
				if !ok || t.Spec.ErrorIntegrationRef == nil {
					return nil
				}

				return []string{t.Spec.ErrorIntegrationRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.errorIntegrationRef.name: %w", err)
		}

		// SuccessIntegration ref indexer.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.successIntegrationRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
				if !ok || t.Spec.SuccessIntegrationRef == nil {
					return nil
				}

				return []string{t.Spec.SuccessIntegrationRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.successIntegrationRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.NotificationIntegration{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.errorIntegrationRef.name", "listing tasks for notification integration watch")),
		)

		// FinalizeRef indexer + watch.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.finalizeRef.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
				if !ok || t.Spec.FinalizeRef == nil {
					return nil
				}

				return []string{t.Spec.FinalizeRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.finalizeRef.name: %w", err)
		}

		// After ref indexer — collects all ref names from the predecessor list.
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.Task{},
			".spec.after.ref.name",
			func(o sigs.Object) []string {
				t, ok := o.(*snowplanev1alpha1.Task)
				if !ok {
					return nil
				}

				var names []string
				for _, pred := range t.Spec.After {
					if pred.Ref != nil {
						names = append(names, pred.Ref.Name)
					}
				}

				return names
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.after.ref.name: %w", err)
		}

		// Watch Task CRs for finalize and after predecessor refs.
		bldr.Watches(
			&snowplanev1alpha1.Task{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.finalizeRef.name", "listing tasks for finalize task watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Task{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.after.ref.name", "listing tasks for after predecessor watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TaskObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.TaskObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Task, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterTaskOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, task *snowplanev1alpha1.Task) error {
	if reconciler.ShouldSkipImmutableValidation(task) {
		return nil
	}

	if task.Status.ShowOutput != nil {
		if task.Status.ShowOutput.Name != "" && !strings.EqualFold(task.Spec.Name, task.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", task.Status.ShowOutput.Name, task.Spec.Name)
		}

		if task.Status.ShowOutput.DatabaseName != "" && task.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(task.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, task.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", task.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if task.Status.ShowOutput.SchemaName != "" && task.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(task.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, task.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", task.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Task, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.TaskObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Task, obs *reconciler.Observation[*snowflake.TaskObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Task) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Task, obs *reconciler.Observation[*snowflake.TaskObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

// cachedAfterName safely returns the cached after name at index i, or "" if out of bounds.
func cachedAfterName(cached []string, i int) string {
	if i < len(cached) {
		return cached[i]
	}

	return ""
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] = (*adapter)(nil)
