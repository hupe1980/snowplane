package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageSerializationPolicy specifies how Snowflake serializes data for storage.
type StorageSerializationPolicy string

// Valid StorageSerializationPolicy values.
const (
	StorageSerializationPolicyCompatible StorageSerializationPolicy = "COMPATIBLE"
	StorageSerializationPolicyOptimized  StorageSerializationPolicy = "OPTIMIZED"
)

// LogLevel specifies the logging verbosity for a Snowflake object.
type LogLevel string

// Valid LogLevel values.
const (
	LogLevelTrace LogLevel = "TRACE"
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
	LogLevelOff   LogLevel = "OFF"
)

// MetricLevel specifies the metric collection level.
type MetricLevel string

// Valid MetricLevel values.
const (
	MetricLevelNone MetricLevel = "NONE"
	MetricLevelAll  MetricLevel = "ALL"
)

// TraceLevel specifies the trace collection level.
type TraceLevel string

// Valid TraceLevel values.
const (
	TraceLevelAlways  TraceLevel = "ALWAYS"
	TraceLevelOnEvent TraceLevel = "ON_EVENT"
	TraceLevelOff     TraceLevel = "OFF"
)

// DatabaseSpec defines the desired state of a Database.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.transient == oldSelf.transient",message="spec.transient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type DatabaseSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake database name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Comment is an optional description for the database.
	Comment *string `json:"comment,omitempty"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty"`

	// Transient indicates this is a transient database (no Fail-safe). Immutable after creation.
	// +kubebuilder:default=false
	Transient bool `json:"transient,omitempty"`

	// Catalog specifies an Apache Iceberg catalog integration name.
	Catalog *string `json:"catalog,omitempty"`

	// ExternalVolume specifies the external volume for Iceberg table storage.
	ExternalVolume *string `json:"externalVolume,omitempty"`

	// ReplaceInvalidCharacters controls whether to replace invalid UTF-8 characters.
	ReplaceInvalidCharacters *bool `json:"replaceInvalidCharacters,omitempty"`

	// DefaultDDLCollation sets the default collation for string columns.
	DefaultDDLCollation *string `json:"defaultDdlCollation,omitempty"`

	// StorageSerializationPolicy controls storage serialization format.
	// +kubebuilder:validation:Enum=COMPATIBLE;OPTIMIZED
	StorageSerializationPolicy *StorageSerializationPolicy `json:"storageSerializationPolicy,omitempty"`

	// LogLevel controls the logging verbosity.
	// +kubebuilder:validation:Enum=TRACE;DEBUG;INFO;WARN;ERROR;FATAL;OFF
	LogLevel *LogLevel `json:"logLevel,omitempty"`

	// MetricLevel controls the metric collection level.
	// +kubebuilder:validation:Enum=NONE;ALL
	MetricLevel *MetricLevel `json:"metricLevel,omitempty"`

	// TraceLevel controls the trace collection level.
	// +kubebuilder:validation:Enum=ALWAYS;ON_EVENT;OFF
	TraceLevel *TraceLevel `json:"traceLevel,omitempty"`
}

// DatabaseShowOutput mirrors the SHOW DATABASES output stored in status.
type DatabaseShowOutput struct {
	// CreatedOn is the timestamp when the database was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the database name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Kind is the database kind (STANDARD or TRANSIENT).
	Kind string `json:"kind,omitempty"`

	// Comment is the database description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns the database.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`
}

// DatabaseStatus defines the observed state of a Database.
type DatabaseStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW DATABASES output for this database.
	ShowOutput *DatabaseShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Database is the Schema for the databases API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// DatabaseList contains a list of Database.
// +kubebuilder:object:root=true
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
