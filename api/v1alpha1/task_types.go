package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TaskSpec defines the desired state of a Snowflake Task.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.warehouseRef) && has(self.userTaskManagedInitialWarehouseSize)) && !(has(self.warehouseName) && has(self.userTaskManagedInitialWarehouseSize))",message="spec.warehouseRef/warehouseName and spec.userTaskManagedInitialWarehouseSize are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!(has(self.warehouseRef) && has(self.warehouseName))",message="spec.warehouseRef and spec.warehouseName are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.warehouseName) || !self.warehouseName.contains('.')",message="spec.warehouseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!(has(self.errorIntegrationRef) && has(self.errorIntegrationName))",message="spec.errorIntegrationRef and spec.errorIntegrationName are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!(has(self.successIntegrationRef) && has(self.successIntegrationName))",message="spec.successIntegrationRef and spec.successIntegrationName are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!(has(self.finalizeRef) && has(self.finalizeName))",message="spec.finalizeRef and spec.finalizeName are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.finalizeName) || !has(self.schedule)",message="spec.finalizeName and spec.schedule are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.finalizeRef) || !has(self.schedule)",message="spec.finalizeRef and spec.schedule are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.finalizeName) || !has(self.after) || size(self.after) == 0",message="spec.finalizeName and spec.after are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.finalizeRef) || !has(self.after) || size(self.after) == 0",message="spec.finalizeRef and spec.after are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.after) || self.after.all(p, (has(p.ref) && !has(p.name)) || (!has(p.ref) && has(p.name)))",message="each entry in spec.after must set exactly one of ref or name"
// +kubebuilder:validation:XValidation:rule="!has(self.after) || self.after.all(p, !has(p.name) || !p.name.contains('.'))",message="spec.after[].name must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type TaskSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake task name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SchemaName *string `json:"schemaName,omitempty"`

	// WarehouseRef references a Warehouse CR in the same namespace.
	// Mutually exclusive with WarehouseName and UserTaskManagedInitialWarehouseSize.
	// +optional
	WarehouseRef *LocalObjectReference `json:"warehouseRef,omitempty"`

	// WarehouseName is the Snowflake warehouse identifier (e.g. "COMPUTE_WH").
	// Mutually exclusive with WarehouseRef and UserTaskManagedInitialWarehouseSize.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	WarehouseName *string `json:"warehouseName,omitempty"`

	// UserTaskManagedInitialWarehouseSize sets the initial size for serverless tasks.
	// Mutually exclusive with WarehouseRef and WarehouseName.
	// +optional
	// +kubebuilder:validation:Enum=XSMALL;SMALL;MEDIUM;LARGE;XLARGE;XXLARGE
	UserTaskManagedInitialWarehouseSize *string `json:"userTaskManagedInitialWarehouseSize,omitempty" snowflake:"USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE"`

	// Schedule defines when the task runs. Required for standalone and root tasks.
	// Examples: "5 MINUTES", "USING CRON 0 9-17 * * SUN America/Los_Angeles"
	// +optional
	Schedule *string `json:"schedule,omitempty" snowflake:"SCHEDULE"`

	// SQLStatement is the SQL code executed when the task runs.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SQLStatement string `json:"sqlStatement"`

	// After specifies predecessor tasks for DAG scheduling.
	// Each entry references either a Task CR or a raw Snowflake task name.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	After []TaskPredecessor `json:"after,omitempty"`

	// When specifies a boolean SQL expression that determines whether the task runs.
	// +optional
	When *string `json:"when,omitempty"`

	// Comment is an optional description for the task.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// AllowOverlappingExecution allows multiple instances of the task graph to run concurrently.
	// +optional
	AllowOverlappingExecution *bool `json:"allowOverlappingExecution,omitempty" snowflake:"ALLOW_OVERLAPPING_EXECUTION"`

	// UserTaskTimeoutMs specifies the time limit on a single run in milliseconds (0-604800000).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=604800000
	UserTaskTimeoutMs *int32 `json:"userTaskTimeoutMs,omitempty" snowflake:"USER_TASK_TIMEOUT_MS"`

	// SuspendTaskAfterNumFailures specifies the number of consecutive failed runs
	// before the task is automatically suspended.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SuspendTaskAfterNumFailures *int32 `json:"suspendTaskAfterNumFailures,omitempty" snowflake:"SUSPEND_TASK_AFTER_NUM_FAILURES"`

	// ErrorIntegrationRef references a NotificationIntegration CR for error notifications.
	// Mutually exclusive with ErrorIntegrationName.
	// +optional
	ErrorIntegrationRef *LocalObjectReference `json:"errorIntegrationRef,omitempty"`

	// ErrorIntegrationName is the Snowflake notification integration identifier for error notifications.
	// Mutually exclusive with ErrorIntegrationRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ErrorIntegrationName *string `json:"errorIntegrationName,omitempty" snowflake:"ERROR_INTEGRATION"`

	// SuccessIntegrationRef references a NotificationIntegration CR for success notifications.
	// Mutually exclusive with SuccessIntegrationName.
	// +optional
	SuccessIntegrationRef *LocalObjectReference `json:"successIntegrationRef,omitempty"`

	// SuccessIntegrationName is the Snowflake notification integration identifier for success notifications.
	// Mutually exclusive with SuccessIntegrationRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SuccessIntegrationName *string `json:"successIntegrationName,omitempty" snowflake:"SUCCESS_INTEGRATION"`

	// TaskAutoRetryAttempts specifies the number of automatic retry attempts (0-30).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=30
	TaskAutoRetryAttempts *int32 `json:"taskAutoRetryAttempts,omitempty" snowflake:"TASK_AUTO_RETRY_ATTEMPTS"`

	// Suspend indicates whether the task should be suspended. Default is true
	// (tasks are created in suspended state).
	// +optional
	// +kubebuilder:default=true
	Suspend *bool `json:"suspend,omitempty"`

	// Config specifies the default configuration string in valid JSON format
	// that all tasks in a task graph can access via SYSTEM$GET_TASK_GRAPH_CONFIG.
	// +optional
	Config *string `json:"config,omitempty" snowflake:"CONFIG"`

	// FinalizeRef references a Task CR that this finalizer task is associated with.
	// Finalizer tasks run after all other tasks in the task graph complete.
	// Mutually exclusive with FinalizeName, Schedule, and After.
	// +optional
	FinalizeRef *LocalObjectReference `json:"finalizeRef,omitempty"`

	// FinalizeName is the name of a root task that this finalizer task is
	// associated with. Finalizer tasks run after all other tasks in the task
	// graph complete. Mutually exclusive with FinalizeRef, Schedule, and After.
	// +optional
	FinalizeName *string `json:"finalizeName,omitempty" snowflake:"FINALIZE,nounset"`

	// LogLevel specifies the severity level of events for the task.
	// +optional
	// +kubebuilder:validation:Enum=TRACE;DEBUG;INFO;WARN;ERROR;FATAL;OFF
	LogLevel *LogLevel `json:"logLevel,omitempty" snowflake:"LOG_LEVEL"`

	// UserTaskMinimumTriggerIntervalInSeconds defines how frequently a triggered task
	// can execute, in seconds. Changes within this interval are batched together.
	// +optional
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=604800
	UserTaskMinimumTriggerIntervalInSeconds *int32 `json:"userTaskMinimumTriggerIntervalInSeconds,omitempty" snowflake:"USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS"`

	// TargetCompletionInterval specifies the desired task completion time.
	// Only applies to serverless tasks. Required for serverless triggered tasks.
	// Examples: "10 MINUTES", "1 HOURS"
	// +optional
	TargetCompletionInterval *string `json:"targetCompletionInterval,omitempty" snowflake:"TARGET_COMPLETION_INTERVAL"`

	// ServerlessTaskMinStatementSize specifies the minimum warehouse size for
	// the serverless task. Only applies to serverless tasks.
	// +optional
	// +kubebuilder:validation:Enum=XSMALL;SMALL;MEDIUM;LARGE;XLARGE;XXLARGE
	ServerlessTaskMinStatementSize *string `json:"serverlessTaskMinStatementSize,omitempty" snowflake:"SERVERLESS_TASK_MIN_STATEMENT_SIZE"`

	// ServerlessTaskMaxStatementSize specifies the maximum warehouse size for
	// the serverless task. Only applies to serverless tasks.
	// +optional
	// +kubebuilder:validation:Enum=XSMALL;SMALL;MEDIUM;LARGE;XLARGE;XXLARGE
	ServerlessTaskMaxStatementSize *string `json:"serverlessTaskMaxStatementSize,omitempty" snowflake:"SERVERLESS_TASK_MAX_STATEMENT_SIZE"`
}

// TaskPredecessor specifies a predecessor task for DAG scheduling.
// Exactly one of Ref or Name must be set.
type TaskPredecessor struct {
	// Ref references a Task CR in the same namespace.
	// +optional
	Ref *LocalObjectReference `json:"ref,omitempty"`

	// Name is the Snowflake task identifier.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name *string `json:"name,omitempty"`
}

// TaskShowOutput mirrors the SHOW TASKS output stored in status.
type TaskShowOutput struct {
	// CreatedOn is the timestamp when the task was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the task name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the task.
	Owner string `json:"owner,omitempty"`

	// Comment is the task description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Warehouse is the warehouse used by the task.
	Warehouse string `json:"warehouse,omitempty"`

	// Schedule is the task schedule expression.
	Schedule string `json:"schedule,omitempty" snowflake:"SCHEDULE"`

	// State is the task state (started or suspended).
	State string `json:"state,omitempty"`

	// Definition is the SQL statement body.
	Definition string `json:"definition,omitempty"`

	// Condition is the WHEN clause.
	Condition string `json:"condition,omitempty"`

	// Predecessors is a comma-separated list of predecessor task names.
	Predecessors string `json:"predecessors,omitempty"`

	// ErrorIntegration is the error notification integration.
	ErrorIntegration string `json:"errorIntegration,omitempty" snowflake:"ERROR_INTEGRATION"`

	// AllowOverlappingExecution indicates whether concurrent graph runs are allowed.
	AllowOverlappingExecution bool `json:"allowOverlappingExecution,omitempty" snowflake:"ALLOW_OVERLAPPING_EXECUTION"`

	// Config is the task configuration JSON string.
	Config string `json:"config,omitempty" snowflake:"CONFIG"`
}

// TaskParameters contains relevant session-level task parameters from SHOW PARAMETERS IN TASK.
type TaskParameters struct {
	// UserTaskTimeoutMs is the task timeout in milliseconds.
	UserTaskTimeoutMs *int32 `json:"userTaskTimeoutMs,omitempty" snowflake:"USER_TASK_TIMEOUT_MS"`

	// SuspendTaskAfterNumFailures is the number of consecutive failures before suspension.
	SuspendTaskAfterNumFailures *int32 `json:"suspendTaskAfterNumFailures,omitempty" snowflake:"SUSPEND_TASK_AFTER_NUM_FAILURES"`

	// TaskAutoRetryAttempts is the number of automatic retry attempts.
	TaskAutoRetryAttempts *int32 `json:"taskAutoRetryAttempts,omitempty" snowflake:"TASK_AUTO_RETRY_ATTEMPTS"`

	// LogLevel is the severity level of events for the task.
	LogLevel LogLevel `json:"logLevel,omitempty" snowflake:"LOG_LEVEL"`

	// UserTaskMinimumTriggerIntervalInSeconds defines how frequently a triggered task can execute.
	UserTaskMinimumTriggerIntervalInSeconds *int32 `json:"userTaskMinimumTriggerIntervalInSeconds,omitempty" snowflake:"USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS"`

	// TargetCompletionInterval is the target completion interval for the task.
	TargetCompletionInterval *string `json:"targetCompletionInterval,omitempty" snowflake:"TARGET_COMPLETION_INTERVAL"`

	// UserTaskManagedInitialWarehouseSize is the initial warehouse size for serverless tasks.
	UserTaskManagedInitialWarehouseSize *string `json:"userTaskManagedInitialWarehouseSize,omitempty" snowflake:"USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE"`
}

// TaskStatus defines the observed state of a Task.
type TaskStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// WarehouseName is the resolved warehouse name.
	WarehouseName string `json:"warehouseName,omitempty"`

	// ErrorIntegrationName is the resolved error integration name.
	ErrorIntegrationName string `json:"errorIntegrationName,omitempty"`

	// SuccessIntegrationName is the resolved success integration name.
	SuccessIntegrationName string `json:"successIntegrationName,omitempty"`

	// FinalizeName is the resolved finalize task name.
	FinalizeName string `json:"finalizeName,omitempty"`

	// AfterNames is the resolved list of predecessor task names.
	AfterNames []string `json:"afterNames,omitempty"`

	// ShowOutput contains the raw SHOW TASKS output for this task.
	ShowOutput *TaskShowOutput `json:"showOutput,omitempty"`

	// Parameters contains the session-level task parameters from SHOW PARAMETERS IN TASK.
	Parameters *TaskParameters `json:"parameters,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Task is the Schema for the tasks API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

// TaskList contains a list of Task.
// +kubebuilder:object:root=true
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}
