package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchemaSpec defines the desired state of a Schema.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.transient == oldSelf.transient",message="spec.transient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
type SchemaSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake schema name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Use this when the database is NOT managed by Snowplane.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseName *string `json:"databaseName,omitempty"`

	// Comment is an optional description for the schema.
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty" snowflake:"DATA_RETENTION_TIME_IN_DAYS"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty" snowflake:"MAX_DATA_EXTENSION_TIME_IN_DAYS"`

	// Transient indicates this is a transient schema (no Fail-safe). Immutable after creation.
	// +kubebuilder:default=false
	Transient bool `json:"transient,omitempty"`

	// ManagedAccess enables managed access mode for the schema.
	ManagedAccess bool `json:"managedAccess,omitempty"`

	// DefaultDDLCollation sets the default collation for string columns.
	DefaultDDLCollation *string `json:"defaultDDLCollation,omitempty" snowflake:"DEFAULT_DDL_COLLATION"`

	// ReplaceInvalidCharacters controls whether to replace invalid UTF-8 characters.
	ReplaceInvalidCharacters *bool `json:"replaceInvalidCharacters,omitempty" snowflake:"REPLACE_INVALID_CHARACTERS"`

	// StorageSerializationPolicy controls storage serialization format.
	StorageSerializationPolicy *StorageSerializationPolicy `json:"storageSerializationPolicy,omitempty" snowflake:"STORAGE_SERIALIZATION_POLICY"`

	// LogLevel controls the logging verbosity.
	LogLevel *LogLevel `json:"logLevel,omitempty" snowflake:"LOG_LEVEL"`

	// MetricLevel controls the metric collection level.
	MetricLevel *MetricLevel `json:"metricLevel,omitempty" snowflake:"METRIC_LEVEL"`

	// TraceLevel controls the trace collection level.
	TraceLevel *TraceLevel `json:"traceLevel,omitempty" snowflake:"TRACE_LEVEL"`
}

// SchemaShowOutput mirrors the SHOW SCHEMAS output stored in status.
type SchemaShowOutput struct {
	// CreatedOn is the timestamp when the schema was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the schema name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// Kind is the schema kind (STANDARD or TRANSIENT).
	Kind string `json:"kind,omitempty"`

	// Comment is the schema description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Owner is the role that owns the schema.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`

	// Options contains schema options (e.g. "MANAGED ACCESS").
	Options string `json:"options,omitempty"`
}

// SchemaStatus defines the observed state of a Schema.
type SchemaStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// ShowOutput contains the raw SHOW SCHEMAS output for this schema.
	ShowOutput *SchemaShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Schema is the Schema for the schemas API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Schema struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SchemaSpec   `json:"spec,omitempty"`
	Status SchemaStatus `json:"status,omitempty"`
}

// SchemaList contains a list of Schema.
// +kubebuilder:object:root=true
type SchemaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Schema `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Schema{}, &SchemaList{})
}
