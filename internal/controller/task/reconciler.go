// Package task implements the reconciler for Task resources.
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
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/task"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake tasks.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error)
	Create(ctx context.Context, opts snowflake.CreateTaskOptions) error
	Alter(ctx context.Context, opts snowflake.AlterTaskOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Task reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewTaskClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Task resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation]{
		ResourceNameVal:  "task",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Task { return &snowplanev1alpha1.Task{} },
		ServiceFactoryFn: sf,
		SupportsCoA:      true,
		BuildIdentifierFn: func(task *snowplanev1alpha1.Task) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(task.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(task.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, task.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.TaskObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterTaskOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.TaskObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Task, obs *reconciler.Observation[*snowflake.TaskObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Task, obs *reconciler.Observation[*snowflake.TaskObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, task *snowplanev1alpha1.Task) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, task,
				task.Namespace, task.Spec.DatabaseRef, task.Spec.DatabaseName, task.Status.DatabaseName)
			if err != nil {
				return err
			}

			task.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, task,
				task.Namespace, task.Spec.SchemaRef, task.Spec.SchemaName, task.Status.SchemaName)
			if err != nil {
				return err
			}

			task.Status.SchemaName = schemaFQN

			// Resolve optional warehouse ref.
			if task.Spec.WarehouseRef != nil || task.Spec.WarehouseName != nil {
				whName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, task,
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
				eiName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, task,
					task.Namespace, task.Spec.ErrorIntegrationRef, task.Spec.ErrorIntegrationName, task.Status.ErrorIntegrationName,
					"ErrorIntegration",
					func() *snowplanev1alpha1.QueueNotificationIntegration {
						return &snowplanev1alpha1.QueueNotificationIntegration{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("QueueNotificationIntegration"),
					func(ni *snowplanev1alpha1.QueueNotificationIntegration) string { return ni.Spec.Name },
				)
				if err != nil {
					return err
				}

				task.Status.ErrorIntegrationName = eiName
			}

			// Resolve optional success integration ref.
			if task.Spec.SuccessIntegrationRef != nil || task.Spec.SuccessIntegrationName != nil {
				siName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, task,
					task.Namespace, task.Spec.SuccessIntegrationRef, task.Spec.SuccessIntegrationName, task.Status.SuccessIntegrationName,
					"SuccessIntegration",
					func() *snowplanev1alpha1.QueueNotificationIntegration {
						return &snowplanev1alpha1.QueueNotificationIntegration{}
					},
					snowplanev1alpha1.GroupVersion.WithKind("QueueNotificationIntegration"),
					func(ni *snowplanev1alpha1.QueueNotificationIntegration) string { return ni.Spec.Name },
				)
				if err != nil {
					return err
				}

				task.Status.SuccessIntegrationName = siName
			}

			// Resolve optional finalize task ref.
			if task.Spec.FinalizeRef != nil || task.Spec.FinalizeName != nil {
				fnName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, task,
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
					predName, afterErr := refresolver.PreReconcileSourceRef(ctx, c, recorder, task,
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
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.databaseRef.name", "listing tasks for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.schemaRef.name", "listing tasks for schema watch")),
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.warehouseRef.name", "listing tasks for warehouse watch")),
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
				&snowplanev1alpha1.QueueNotificationIntegration{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.errorIntegrationRef.name", "listing tasks for notification integration watch")),
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.finalizeRef.name", "listing tasks for finalize task watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Task{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.TaskList{} }, ".spec.after.ref.name", "listing tasks for after predecessor watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, task *snowplanev1alpha1.Task) error {
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

// cachedAfterName safely returns the cached after name at index i, or "" if out of bounds.
func cachedAfterName(cached []string, i int) string {
	if i < len(cached) {
		return cached[i]
	}

	return ""
}

func applyObservation(task *snowplanev1alpha1.Task, obs *snowflake.TaskObservation) {
	if obs.ShowOutput != nil {
		task.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		task.Status.DatabaseName = obs.ShowOutput.DatabaseName
		task.Status.SchemaName = obs.ShowOutput.SchemaName

		task.Status.ShowOutput = obs.ShowOutput
	}

	if obs.Parameters != nil {
		task.Status.Parameters = &snowplanev1alpha1.TaskParameters{
			UserTaskTimeoutMs:                       obs.Parameters.UserTaskTimeoutMs,
			SuspendTaskAfterNumFailures:             obs.Parameters.SuspendTaskAfterNumFailures,
			TaskAutoRetryAttempts:                   obs.Parameters.TaskAutoRetryAttempts,
			LogLevel:                                snowplanev1alpha1.LogLevel(obs.Parameters.LogLevel),
			UserTaskMinimumTriggerIntervalInSeconds: obs.Parameters.UserTaskMinimumTriggerIntervalInSeconds,
			TargetCompletionInterval:                obs.Parameters.TargetCompletionInterval,
			UserTaskManagedInitialWarehouseSize:     obs.Parameters.UserTaskManagedInitialWarehouseSize,
		}
	}
}

func buildCreateOptions(task *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier) snowflake.CreateTaskOptions {
	opts := snowflake.CreateTaskOptions{
		Name:                                    id,
		UserTaskManagedInitialWarehouseSize:     task.Spec.UserTaskManagedInitialWarehouseSize,
		Schedule:                                task.Spec.Schedule,
		SQLStatement:                            task.Spec.SQLStatement,
		After:                                   task.Status.AfterNames,
		When:                                    task.Spec.When,
		Comment:                                 task.Spec.Comment,
		AllowOverlappingExecution:               task.Spec.AllowOverlappingExecution,
		UserTaskTimeoutMs:                       task.Spec.UserTaskTimeoutMs,
		SuspendTaskAfterNumFailures:             task.Spec.SuspendTaskAfterNumFailures,
		TaskAutoRetryAttempts:                   task.Spec.TaskAutoRetryAttempts,
		Config:                                  task.Spec.Config,
		LogLevel:                                (*string)(task.Spec.LogLevel),
		UserTaskMinimumTriggerIntervalInSeconds: task.Spec.UserTaskMinimumTriggerIntervalInSeconds,
		TargetCompletionInterval:                task.Spec.TargetCompletionInterval,
		ServerlessTaskMinStatementSize:          task.Spec.ServerlessTaskMinStatementSize,
		ServerlessTaskMaxStatementSize:          task.Spec.ServerlessTaskMaxStatementSize,
	}

	if task.Status.WarehouseName != "" {
		wh := task.Status.WarehouseName
		opts.Warehouse = &wh
	}

	if task.Status.ErrorIntegrationName != "" {
		ei := task.Status.ErrorIntegrationName
		opts.ErrorIntegration = &ei
	}

	if task.Status.SuccessIntegrationName != "" {
		si := task.Status.SuccessIntegrationName
		opts.SuccessIntegration = &si
	}

	if task.Status.FinalizeName != "" {
		fn := task.Status.FinalizeName
		opts.Finalize = &fn
	}

	return opts
}

func buildAlterOptions(task *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier, obs *snowflake.TaskObservation) snowflake.AlterTaskOptions {
	opts := snowflake.AlterTaskOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&task.Spec, task.Status.TrackedParameters)

	if task.Spec.Comment != nil {
		if obs.ShowOutput == nil || *task.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = task.Spec.Comment
		}
	}

	if obs.ShowOutput != nil {
		// Schedule changes.
		if task.Spec.Schedule != nil && *task.Spec.Schedule != obs.ShowOutput.Schedule {
			opts.Schedule = task.Spec.Schedule
		}

		// Warehouse changes.
		if task.Status.WarehouseName != "" && task.Status.WarehouseName != obs.ShowOutput.Warehouse {
			wh := task.Status.WarehouseName
			opts.Warehouse = &wh
		}

		// SQL statement changes.
		if task.Spec.SQLStatement != obs.ShowOutput.Definition {
			stmt := task.Spec.SQLStatement
			opts.SQLStatement = &stmt
		}

		// When condition changes.
		if task.Spec.When != nil && *task.Spec.When != obs.ShowOutput.Condition {
			opts.When = task.Spec.When
		}

		// Suspend/resume state changes.
		if task.Spec.Suspend != nil {
			isSuspended := obs.ShowOutput.State == "suspended"
			if *task.Spec.Suspend != isSuspended {
				opts.Suspend = task.Spec.Suspend
			}
		}
	}

	if task.Spec.UserTaskTimeoutMs != nil {
		if obs.Parameters == nil || obs.Parameters.UserTaskTimeoutMs == nil || *task.Spec.UserTaskTimeoutMs != *obs.Parameters.UserTaskTimeoutMs {
			opts.UserTaskTimeoutMs = task.Spec.UserTaskTimeoutMs
		}
	}

	if task.Spec.SuspendTaskAfterNumFailures != nil {
		if obs.Parameters == nil || obs.Parameters.SuspendTaskAfterNumFailures == nil || *task.Spec.SuspendTaskAfterNumFailures != *obs.Parameters.SuspendTaskAfterNumFailures {
			opts.SuspendTaskAfterNumFailures = task.Spec.SuspendTaskAfterNumFailures
		}
	}

	if task.Status.ErrorIntegrationName != "" {
		if obs.ShowOutput == nil || task.Status.ErrorIntegrationName != obs.ShowOutput.ErrorIntegration {
			ei := task.Status.ErrorIntegrationName
			opts.ErrorIntegration = &ei
		}
	}

	if task.Status.SuccessIntegrationName != "" {
		// SuccessIntegration is not exposed in SHOW TASKS output, so always include when set.
		si := task.Status.SuccessIntegrationName
		opts.SuccessIntegration = &si
	}

	if task.Spec.AllowOverlappingExecution != nil {
		if obs.ShowOutput == nil || *task.Spec.AllowOverlappingExecution != obs.ShowOutput.AllowOverlappingExecution {
			opts.AllowOverlappingExecution = task.Spec.AllowOverlappingExecution
		}
	}

	if task.Spec.TaskAutoRetryAttempts != nil {
		if obs.Parameters == nil || obs.Parameters.TaskAutoRetryAttempts == nil || *task.Spec.TaskAutoRetryAttempts != *obs.Parameters.TaskAutoRetryAttempts {
			opts.TaskAutoRetryAttempts = task.Spec.TaskAutoRetryAttempts
		}
	}

	if task.Spec.Config != nil {
		if obs.ShowOutput == nil || *task.Spec.Config != obs.ShowOutput.Config {
			opts.Config = task.Spec.Config
		}
	}

	// Finalize — uses dedicated SET/UNSET FINALIZE.
	if task.Status.FinalizeName != "" {
		fn := task.Status.FinalizeName
		opts.SetFinalize = &fn
	} else {
		for _, p := range task.Status.TrackedParameters {
			if p == "FINALIZE" {
				opts.UnsetFinalize = true
				break
			}
		}
	}

	if task.Spec.LogLevel != nil {
		if obs.Parameters == nil || string(*task.Spec.LogLevel) != obs.Parameters.LogLevel {
			opts.LogLevel = (*string)(task.Spec.LogLevel)
		}
	}

	if task.Spec.UserTaskMinimumTriggerIntervalInSeconds != nil {
		if obs.Parameters == nil || obs.Parameters.UserTaskMinimumTriggerIntervalInSeconds == nil || *task.Spec.UserTaskMinimumTriggerIntervalInSeconds != *obs.Parameters.UserTaskMinimumTriggerIntervalInSeconds {
			opts.UserTaskMinimumTriggerIntervalInSeconds = task.Spec.UserTaskMinimumTriggerIntervalInSeconds
		}
	}

	if task.Spec.TargetCompletionInterval != nil {
		if obs.Parameters == nil || obs.Parameters.TargetCompletionInterval == nil || *task.Spec.TargetCompletionInterval != *obs.Parameters.TargetCompletionInterval {
			opts.TargetCompletionInterval = task.Spec.TargetCompletionInterval
		}
	}

	if task.Spec.ServerlessTaskMinStatementSize != nil {
		if obs.Parameters == nil || obs.Parameters.ServerlessTaskMinStatementSize == nil || *task.Spec.ServerlessTaskMinStatementSize != *obs.Parameters.ServerlessTaskMinStatementSize {
			opts.ServerlessTaskMinStatementSize = task.Spec.ServerlessTaskMinStatementSize
		}
	}

	if task.Spec.ServerlessTaskMaxStatementSize != nil {
		if obs.Parameters == nil || obs.Parameters.ServerlessTaskMaxStatementSize == nil || *task.Spec.ServerlessTaskMaxStatementSize != *obs.Parameters.ServerlessTaskMaxStatementSize {
			opts.ServerlessTaskMaxStatementSize = task.Spec.ServerlessTaskMaxStatementSize
		}
	}

	if task.Spec.UserTaskManagedInitialWarehouseSize != nil {
		if obs.Parameters == nil || obs.Parameters.UserTaskManagedInitialWarehouseSize == nil || *task.Spec.UserTaskManagedInitialWarehouseSize != *obs.Parameters.UserTaskManagedInitialWarehouseSize {
			opts.UserTaskManagedInitialWarehouseSize = task.Spec.UserTaskManagedInitialWarehouseSize
		}
	}

	return opts
}

func detectDrift(task *snowplanev1alpha1.Task, obs *snowflake.TaskObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", task.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(task.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(task.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields from SHOW output.
		d.CompareString("COMMENT", task.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("SCHEDULE", task.Spec.Schedule, obs.ShowOutput.Schedule, false)

		var warehousePtr *string
		if task.Status.WarehouseName != "" {
			wh := task.Status.WarehouseName
			warehousePtr = &wh
		}

		d.CompareString("WAREHOUSE", warehousePtr, obs.ShowOutput.Warehouse, false)

		d.CompareStringValue("SQL_STATEMENT", task.Spec.SQLStatement, obs.ShowOutput.Definition, false)
		d.CompareString("WHEN", task.Spec.When, obs.ShowOutput.Condition, false)
		var errorIntPtr *string
		if task.Status.ErrorIntegrationName != "" {
			ei := task.Status.ErrorIntegrationName
			errorIntPtr = &ei
		}

		d.CompareString("ERROR_INTEGRATION", errorIntPtr, obs.ShowOutput.ErrorIntegration, false)

		// AllowOverlappingExecution: spec is *bool, observed is plain bool.
		if task.Spec.AllowOverlappingExecution != nil {
			d.CompareBoolValue("ALLOW_OVERLAPPING_EXECUTION", *task.Spec.AllowOverlappingExecution, obs.ShowOutput.AllowOverlappingExecution, false)
		}

		d.CompareString("CONFIG", task.Spec.Config, obs.ShowOutput.Config, false)
	}

	if obs.Parameters != nil {
		// Mutable fields from SHOW PARAMETERS.
		d.CompareInt32("USER_TASK_TIMEOUT_MS", task.Spec.UserTaskTimeoutMs, obs.Parameters.UserTaskTimeoutMs, false)
		d.CompareInt32("SUSPEND_TASK_AFTER_NUM_FAILURES", task.Spec.SuspendTaskAfterNumFailures, obs.Parameters.SuspendTaskAfterNumFailures, false)
		d.CompareInt32("TASK_AUTO_RETRY_ATTEMPTS", task.Spec.TaskAutoRetryAttempts, obs.Parameters.TaskAutoRetryAttempts, false)
		d.CompareString("LOG_LEVEL", (*string)(task.Spec.LogLevel), obs.Parameters.LogLevel, false)
		d.CompareInt32("USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS", task.Spec.UserTaskMinimumTriggerIntervalInSeconds, obs.Parameters.UserTaskMinimumTriggerIntervalInSeconds, false)
	}

	return d.Result()
}
