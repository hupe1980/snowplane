// Package task implements the reconciler for Task resources.
package task

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
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
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewTaskClient(sfC), cleanup, nil
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

		task.Status.ShowOutput = &snowplanev1alpha1.TaskShowOutput{
			CreatedOn:                 obs.ShowOutput.CreatedOn,
			Name:                      obs.ShowOutput.Name,
			DatabaseName:              obs.ShowOutput.DatabaseName,
			SchemaName:                obs.ShowOutput.SchemaName,
			Owner:                     obs.ShowOutput.Owner,
			Comment:                   obs.ShowOutput.Comment,
			Warehouse:                 obs.ShowOutput.Warehouse,
			Schedule:                  obs.ShowOutput.Schedule,
			State:                     obs.ShowOutput.State,
			Definition:                obs.ShowOutput.Definition,
			Condition:                 obs.ShowOutput.Condition,
			Predecessors:              obs.ShowOutput.Predecessors,
			ErrorIntegration:          obs.ShowOutput.ErrorIntegration,
			AllowOverlappingExecution: obs.ShowOutput.AllowOverlappingExecution,
			Config:                    obs.ShowOutput.Config,
		}
	}

	if obs.Parameters != nil {
		task.Status.Parameters = &snowplanev1alpha1.TaskParameters{
			UserTaskTimeoutMs:                       obs.Parameters.UserTaskTimeoutMs,
			SuspendTaskAfterNumFailures:             obs.Parameters.SuspendTaskAfterNumFailures,
			TaskAutoRetryAttempts:                   obs.Parameters.TaskAutoRetryAttempts,
			LogLevel:                                obs.Parameters.LogLevel,
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
		LogLevel:                                task.Spec.LogLevel,
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
		if obs.Parameters == nil || *task.Spec.LogLevel != obs.Parameters.LogLevel {
			opts.LogLevel = task.Spec.LogLevel
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
		opts.ServerlessTaskMinStatementSize = task.Spec.ServerlessTaskMinStatementSize
	}

	if task.Spec.ServerlessTaskMaxStatementSize != nil {
		opts.ServerlessTaskMaxStatementSize = task.Spec.ServerlessTaskMaxStatementSize
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
		d.CompareString("LOG_LEVEL", task.Spec.LogLevel, obs.Parameters.LogLevel, false)
		d.CompareInt32("USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS", task.Spec.UserTaskMinimumTriggerIntervalInSeconds, obs.Parameters.UserTaskMinimumTriggerIntervalInSeconds, false)
	}

	return d.Result()
}
