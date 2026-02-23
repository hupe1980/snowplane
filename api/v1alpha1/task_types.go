package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TaskSpec defines the desired state of a Snowflake Task.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.warehouse) && has(self.userTaskManagedInitialWarehouseSize))",message="spec.warehouse and spec.userTaskManagedInitialWarehouseSize are mutually exclusive"
type TaskSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake task name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Warehouse specifies the virtual warehouse for task runs.
	// Mutually exclusive with UserTaskManagedInitialWarehouseSize.
	// +optional
	Warehouse *string `json:"warehouse,omitempty"`

	// UserTaskManagedInitialWarehouseSize sets the initial size for serverless tasks.
	// Mutually exclusive with Warehouse.
	// +optional
	// +kubebuilder:validation:Enum=XSMALL;SMALL;MEDIUM;LARGE;XLARGE;XXLARGE
	UserTaskManagedInitialWarehouseSize *string `json:"userTaskManagedInitialWarehouseSize,omitempty"`

	// Schedule defines when the task runs. Required for standalone and root tasks.
	// Examples: "5 MINUTES", "USING CRON 0 9-17 * * SUN America/Los_Angeles"
	// +optional
	Schedule *string `json:"schedule,omitempty"`

	// SQLStatement is the SQL code executed when the task runs.
	// +kubebuilder:validation:MinLength=1
	SQLStatement string `json:"sqlStatement"`

	// After specifies predecessor task names for DAG scheduling.
	// +optional
	After []string `json:"after,omitempty"`

	// When specifies a boolean SQL expression that determines whether the task runs.
	// +optional
	When *string `json:"when,omitempty"`

	// Comment is an optional description for the task.
	// +optional
	Comment *string `json:"comment,omitempty"`

	// AllowOverlappingExecution allows multiple instances of the task graph to run concurrently.
	// +optional
	AllowOverlappingExecution *bool `json:"allowOverlappingExecution,omitempty"`

	// UserTaskTimeoutMs specifies the time limit on a single run in milliseconds (0-604800000).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=604800000
	UserTaskTimeoutMs *int32 `json:"userTaskTimeoutMs,omitempty"`

	// SuspendTaskAfterNumFailures specifies the number of consecutive failed runs
	// before the task is automatically suspended.
	// +optional
	// +kubebuilder:validation:Minimum=0
	SuspendTaskAfterNumFailures *int32 `json:"suspendTaskAfterNumFailures,omitempty"`

	// ErrorIntegration is the notification integration for error notifications.
	// +optional
	ErrorIntegration *string `json:"errorIntegration,omitempty"`

	// SuccessIntegration is the notification integration for success notifications.
	// +optional
	SuccessIntegration *string `json:"successIntegration,omitempty"`

	// TaskAutoRetryAttempts specifies the number of automatic retry attempts (0-30).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=30
	TaskAutoRetryAttempts *int32 `json:"taskAutoRetryAttempts,omitempty"`

	// Suspend indicates whether the task should be suspended. Default is true
	// (tasks are created in suspended state).
	// +optional
	// +kubebuilder:default=true
	Suspend *bool `json:"suspend,omitempty"`
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
	Comment string `json:"comment,omitempty"`

	// Warehouse is the warehouse used by the task.
	Warehouse string `json:"warehouse,omitempty"`

	// Schedule is the task schedule expression.
	Schedule string `json:"schedule,omitempty"`

	// State is the task state (started or suspended).
	State string `json:"state,omitempty"`

	// Definition is the SQL statement body.
	Definition string `json:"definition,omitempty"`

	// Condition is the WHEN clause.
	Condition string `json:"condition,omitempty"`

	// Predecessors is a comma-separated list of predecessor task names.
	Predecessors string `json:"predecessors,omitempty"`

	// ErrorIntegration is the error notification integration.
	ErrorIntegration string `json:"errorIntegration,omitempty"`
}

// TaskStatus defines the observed state of a Task.
type TaskStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW TASKS output for this task.
	ShowOutput *TaskShowOutput `json:"showOutput,omitempty"`

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

// GetConditions returns the conditions of the Task.
func (t *Task) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

// SetConditions sets the conditions of the Task.
func (t *Task) SetConditions(conditions []metav1.Condition) {
	t.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (t *Task) GetDeletionPolicy() DeletionPolicy {
	if t.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return t.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (t *Task) GetFullyQualifiedName() string {
	return t.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (t *Task) GetSpecName() string { return t.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (t *Task) GetProviderRef() ProviderReference { return t.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (t *Task) GetUseRole() *string { return t.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (t *Task) GetObservedGeneration() int64 { return t.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (t *Task) SetObservedGeneration(v int64) { t.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (t *Task) GetLastAppliedSpecHash() string { return t.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (t *Task) SetLastAppliedSpecHash(v string) { t.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (t *Task) GetTrackedParametersList() []string { return t.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (t *Task) SetTrackedParametersList(v []string) { t.Status.TrackedParameters = v }

// GetOwner returns the owner role from status.
func (t *Task) GetOwner() string {
	if t.Status.ShowOutput != nil {
		return t.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (t *Task) ValidateSpec() error { return t.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (t *Task) ComputeSpecHash() (string, error) { return ComputeSpecHash(t.Spec) }

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}
