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
			CreatedOn:        obs.ShowOutput.CreatedOn,
			Name:             obs.ShowOutput.Name,
			DatabaseName:     obs.ShowOutput.DatabaseName,
			SchemaName:       obs.ShowOutput.SchemaName,
			Owner:            obs.ShowOutput.Owner,
			Comment:          obs.ShowOutput.Comment,
			Warehouse:        obs.ShowOutput.Warehouse,
			Schedule:         obs.ShowOutput.Schedule,
			State:            obs.ShowOutput.State,
			Definition:       obs.ShowOutput.Definition,
			Condition:        obs.ShowOutput.Condition,
			Predecessors:     obs.ShowOutput.Predecessors,
			ErrorIntegration: obs.ShowOutput.ErrorIntegration,
		}
	}
}

func buildCreateOptions(task *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier) snowflake.CreateTaskOptions {
	return snowflake.CreateTaskOptions{
		Name:                                    id,
		Warehouse:                               task.Spec.Warehouse,
		UserTaskManagedInitialWarehouseSize:     task.Spec.UserTaskManagedInitialWarehouseSize,
		Schedule:                                task.Spec.Schedule,
		SQLStatement:                            task.Spec.SQLStatement,
		After:                                   task.Spec.After,
		When:                                    task.Spec.When,
		Comment:                                 task.Spec.Comment,
		AllowOverlappingExecution:               task.Spec.AllowOverlappingExecution,
		UserTaskTimeoutMs:                       task.Spec.UserTaskTimeoutMs,
		SuspendTaskAfterNumFailures:             task.Spec.SuspendTaskAfterNumFailures,
		ErrorIntegration:                        task.Spec.ErrorIntegration,
		SuccessIntegration:                      task.Spec.SuccessIntegration,
		TaskAutoRetryAttempts:                   task.Spec.TaskAutoRetryAttempts,
		Config:                                  task.Spec.Config,
		Finalize:                                task.Spec.Finalize,
		LogLevel:                                task.Spec.LogLevel,
		UserTaskMinimumTriggerIntervalInSeconds: task.Spec.UserTaskMinimumTriggerIntervalInSeconds,
		TargetCompletionInterval:                task.Spec.TargetCompletionInterval,
		ServerlessTaskMinStatementSize:          task.Spec.ServerlessTaskMinStatementSize,
		ServerlessTaskMaxStatementSize:          task.Spec.ServerlessTaskMaxStatementSize,
	}
}

func buildAlterOptions(task *snowplanev1alpha1.Task, id snowflake.SchemaObjectIdentifier, obs *snowflake.TaskObservation) snowflake.AlterTaskOptions {
	opts := snowflake.AlterTaskOptions{Name: id}
	opts.UnsetFields = computeUnsetFields(task)

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
		if task.Spec.Warehouse != nil && *task.Spec.Warehouse != obs.ShowOutput.Warehouse {
			opts.Warehouse = task.Spec.Warehouse
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
		opts.UserTaskTimeoutMs = task.Spec.UserTaskTimeoutMs
	}

	if task.Spec.SuspendTaskAfterNumFailures != nil {
		opts.SuspendTaskAfterNumFailures = task.Spec.SuspendTaskAfterNumFailures
	}

	if task.Spec.ErrorIntegration != nil {
		if obs.ShowOutput == nil || *task.Spec.ErrorIntegration != obs.ShowOutput.ErrorIntegration {
			opts.ErrorIntegration = task.Spec.ErrorIntegration
		}
	}

	if task.Spec.SuccessIntegration != nil {
		opts.SuccessIntegration = task.Spec.SuccessIntegration
	}

	if task.Spec.AllowOverlappingExecution != nil {
		opts.AllowOverlappingExecution = task.Spec.AllowOverlappingExecution
	}

	if task.Spec.TaskAutoRetryAttempts != nil {
		opts.TaskAutoRetryAttempts = task.Spec.TaskAutoRetryAttempts
	}

	if task.Spec.Config != nil {
		opts.Config = task.Spec.Config
	}

	// Finalize — uses dedicated SET/UNSET FINALIZE.
	if task.Spec.Finalize != nil {
		opts.SetFinalize = task.Spec.Finalize
	} else {
		for _, p := range task.Status.TrackedParameters {
			if p == "FINALIZE" {
				opts.UnsetFinalize = true
				break
			}
		}
	}

	if task.Spec.LogLevel != nil {
		opts.LogLevel = task.Spec.LogLevel
	}

	if task.Spec.UserTaskMinimumTriggerIntervalInSeconds != nil {
		opts.UserTaskMinimumTriggerIntervalInSeconds = task.Spec.UserTaskMinimumTriggerIntervalInSeconds
	}

	if task.Spec.TargetCompletionInterval != nil {
		opts.TargetCompletionInterval = task.Spec.TargetCompletionInterval
	}

	if task.Spec.ServerlessTaskMinStatementSize != nil {
		opts.ServerlessTaskMinStatementSize = task.Spec.ServerlessTaskMinStatementSize
	}

	if task.Spec.ServerlessTaskMaxStatementSize != nil {
		opts.ServerlessTaskMaxStatementSize = task.Spec.ServerlessTaskMaxStatementSize
	}

	if task.Spec.UserTaskManagedInitialWarehouseSize != nil {
		opts.UserTaskManagedInitialWarehouseSize = task.Spec.UserTaskManagedInitialWarehouseSize
	}

	return opts
}

func computeUnsetFields(task *snowplanev1alpha1.Task) []string {
	if len(task.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(task.Status.TrackedParameters))
	for _, f := range task.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if task.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	if task.Spec.Schedule == nil && managed["SCHEDULE"] {
		unset = append(unset, "SCHEDULE")
	}

	if task.Spec.UserTaskTimeoutMs == nil && managed["USER_TASK_TIMEOUT_MS"] {
		unset = append(unset, "USER_TASK_TIMEOUT_MS")
	}

	if task.Spec.SuspendTaskAfterNumFailures == nil && managed["SUSPEND_TASK_AFTER_NUM_FAILURES"] {
		unset = append(unset, "SUSPEND_TASK_AFTER_NUM_FAILURES")
	}

	if task.Spec.ErrorIntegration == nil && managed["ERROR_INTEGRATION"] {
		unset = append(unset, "ERROR_INTEGRATION")
	}

	if task.Spec.SuccessIntegration == nil && managed["SUCCESS_INTEGRATION"] {
		unset = append(unset, "SUCCESS_INTEGRATION")
	}

	if task.Spec.AllowOverlappingExecution == nil && managed["ALLOW_OVERLAPPING_EXECUTION"] {
		unset = append(unset, "ALLOW_OVERLAPPING_EXECUTION")
	}

	if task.Spec.TaskAutoRetryAttempts == nil && managed["TASK_AUTO_RETRY_ATTEMPTS"] {
		unset = append(unset, "TASK_AUTO_RETRY_ATTEMPTS")
	}

	if task.Spec.Config == nil && managed["CONFIG"] {
		unset = append(unset, "CONFIG")
	}

	// FINALIZE is handled separately via UnsetFinalize, not via generic UnsetFields.

	if task.Spec.LogLevel == nil && managed["LOG_LEVEL"] {
		unset = append(unset, "LOG_LEVEL")
	}

	if task.Spec.TargetCompletionInterval == nil && managed["TARGET_COMPLETION_INTERVAL"] {
		unset = append(unset, "TARGET_COMPLETION_INTERVAL")
	}

	if task.Spec.ServerlessTaskMinStatementSize == nil && managed["SERVERLESS_TASK_MIN_STATEMENT_SIZE"] {
		unset = append(unset, "SERVERLESS_TASK_MIN_STATEMENT_SIZE")
	}

	if task.Spec.ServerlessTaskMaxStatementSize == nil && managed["SERVERLESS_TASK_MAX_STATEMENT_SIZE"] {
		unset = append(unset, "SERVERLESS_TASK_MAX_STATEMENT_SIZE")
	}

	if task.Spec.UserTaskMinimumTriggerIntervalInSeconds == nil && managed["USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS"] {
		unset = append(unset, "USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS")
	}

	if task.Spec.UserTaskManagedInitialWarehouseSize == nil && managed["USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE"] {
		unset = append(unset, "USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE")
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.TaskSpec) []string {
	var fields []string

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	if spec.Schedule != nil {
		fields = append(fields, "SCHEDULE")
	}

	if spec.UserTaskTimeoutMs != nil {
		fields = append(fields, "USER_TASK_TIMEOUT_MS")
	}

	if spec.SuspendTaskAfterNumFailures != nil {
		fields = append(fields, "SUSPEND_TASK_AFTER_NUM_FAILURES")
	}

	if spec.ErrorIntegration != nil {
		fields = append(fields, "ERROR_INTEGRATION")
	}

	if spec.SuccessIntegration != nil {
		fields = append(fields, "SUCCESS_INTEGRATION")
	}

	if spec.AllowOverlappingExecution != nil {
		fields = append(fields, "ALLOW_OVERLAPPING_EXECUTION")
	}

	if spec.TaskAutoRetryAttempts != nil {
		fields = append(fields, "TASK_AUTO_RETRY_ATTEMPTS")
	}

	if spec.Config != nil {
		fields = append(fields, "CONFIG")
	}

	if spec.Finalize != nil {
		fields = append(fields, "FINALIZE")
	}

	if spec.LogLevel != nil {
		fields = append(fields, "LOG_LEVEL")
	}

	if spec.UserTaskMinimumTriggerIntervalInSeconds != nil {
		fields = append(fields, "USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS")
	}

	if spec.TargetCompletionInterval != nil {
		fields = append(fields, "TARGET_COMPLETION_INTERVAL")
	}

	if spec.ServerlessTaskMinStatementSize != nil {
		fields = append(fields, "SERVERLESS_TASK_MIN_STATEMENT_SIZE")
	}

	if spec.ServerlessTaskMaxStatementSize != nil {
		fields = append(fields, "SERVERLESS_TASK_MAX_STATEMENT_SIZE")
	}

	if spec.UserTaskManagedInitialWarehouseSize != nil {
		fields = append(fields, "USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE")
	}

	return fields
}

func detectDrift(task *snowplanev1alpha1.Task, obs *snowflake.TaskObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", task.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(task.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(task.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareString("COMMENT", task.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("SCHEDULE", task.Spec.Schedule, obs.ShowOutput.Schedule, false)
		d.CompareString("WAREHOUSE", task.Spec.Warehouse, obs.ShowOutput.Warehouse, false)

		d.CompareStringValue("SQL_STATEMENT", task.Spec.SQLStatement, obs.ShowOutput.Definition, false)
		d.CompareString("WHEN", task.Spec.When, obs.ShowOutput.Condition, false)
		d.CompareString("ERROR_INTEGRATION", task.Spec.ErrorIntegration, obs.ShowOutput.ErrorIntegration, false)
	}

	return d.Result()
}
