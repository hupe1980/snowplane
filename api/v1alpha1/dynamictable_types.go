package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DynamicTableRefreshMode specifies how a dynamic table refreshes its data.
// +kubebuilder:validation:Enum=AUTO;FULL;INCREMENTAL
type DynamicTableRefreshMode string

// Valid DynamicTableRefreshMode values.
const (
	DynamicTableRefreshModeAuto        DynamicTableRefreshMode = "AUTO"
	DynamicTableRefreshModeFull        DynamicTableRefreshMode = "FULL"
	DynamicTableRefreshModeIncremental DynamicTableRefreshMode = "INCREMENTAL"
)

// DynamicTableInitialize controls when initial data population occurs.
// +kubebuilder:validation:Enum=ON_CREATE;ON_SCHEDULE
type DynamicTableInitialize string

// Valid DynamicTableInitialize values.
const (
	DynamicTableInitializeOnCreate   DynamicTableInitialize = "ON_CREATE"
	DynamicTableInitializeOnSchedule DynamicTableInitialize = "ON_SCHEDULE"
)

// DynamicTableSpec defines the desired state of a Snowflake Dynamic Table.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.query == oldSelf.query",message="spec.query is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.refreshMode) == has(self.refreshMode) && (!has(self.refreshMode) || self.refreshMode == oldSelf.refreshMode)",message="spec.refreshMode is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.initialize) == has(self.initialize) && (!has(self.initialize) || self.initialize == oldSelf.initialize)",message="spec.initialize is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.transient == oldSelf.transient",message="spec.transient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type DynamicTableSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake dynamic table name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Query is the SQL query that defines the dynamic table content.
	// This is the AS clause of CREATE DYNAMIC TABLE. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Query string `json:"query"`

	// TargetLag specifies the maximum acceptable staleness of the dynamic table.
	// Examples: "1 minute", "5 minutes", "1 hour", "DOWNSTREAM".
	// +kubebuilder:validation:MinLength=1
	TargetLag string `json:"targetLag"`

	// Warehouse is the warehouse used for refreshing the dynamic table.
	// +kubebuilder:validation:MinLength=1
	Warehouse string `json:"warehouse"`

	// RefreshMode specifies the refresh strategy (AUTO, FULL, INCREMENTAL).
	// Immutable after creation.
	// +optional
	RefreshMode *DynamicTableRefreshMode `json:"refreshMode,omitempty"`

	// Initialize controls when initial data population occurs (ON_CREATE, ON_SCHEDULE).
	// Immutable after creation.
	// +optional
	Initialize *DynamicTableInitialize `json:"initialize,omitempty"`

	// Comment is an optional description for the dynamic table.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Transient indicates this is a transient dynamic table (no Fail-safe).
	// Immutable after creation.
	// +optional
	// +kubebuilder:default=false
	Transient bool `json:"transient,omitempty"`

	// ClusterBy specifies the clustering key expressions for the dynamic table.
	// +optional
	ClusterBy []string `json:"clusterBy,omitempty" snowflake:"CLUSTER_BY,nounset"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	// +optional
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty" snowflake:"DATA_RETENTION_TIME_IN_DAYS"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	// +optional
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty" snowflake:"MAX_DATA_EXTENSION_TIME_IN_DAYS"`
}

// DynamicTableShowOutput mirrors the SHOW DYNAMIC TABLES output stored in status.
type DynamicTableShowOutput struct {
	// CreatedOn is the timestamp when the dynamic table was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the dynamic table name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the dynamic table.
	Owner string `json:"owner,omitempty"`

	// Comment is the dynamic table description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// TargetLag is the configured target lag.
	TargetLag string `json:"targetLag,omitempty"`

	// Warehouse is the warehouse used for refreshes.
	Warehouse string `json:"warehouse,omitempty"`

	// RefreshMode is the current refresh mode.
	RefreshMode string `json:"refreshMode,omitempty"`

	// Text is the full DDL definition.
	Text string `json:"text,omitempty"`

	// SchedulingState indicates the scheduling state (RUNNING, SUSPENDED, etc.).
	SchedulingState string `json:"schedulingState,omitempty"`

	// ClusterBy is the clustering key expression.
	ClusterBy string `json:"clusterBy,omitempty" snowflake:"CLUSTER_BY,nounset"`

	// DataTimestamp is the timestamp of the latest data refresh.
	DataTimestamp string `json:"dataTimestamp,omitempty"`
}

// DynamicTableStatus defines the observed state of a DynamicTable.
type DynamicTableStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW DYNAMIC TABLES output for this dynamic table.
	ShowOutput *DynamicTableShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// DynamicTable is the Schema for the dynamictables API.
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
type DynamicTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DynamicTableSpec   `json:"spec,omitempty"`
	Status DynamicTableStatus `json:"status,omitempty"`
}

// DynamicTableList contains a list of DynamicTable.
// +kubebuilder:object:root=true
type DynamicTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DynamicTable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DynamicTable{}, &DynamicTableList{})
}
