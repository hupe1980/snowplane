package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SharedDatabaseSpec defines the desired state of a SharedDatabase.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.fromShare == oldSelf.fromShare",message="spec.fromShare is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type SharedDatabaseSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake database name for the shared database.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// FromShare is the fully-qualified share identifier to create the database from.
	// Format: "<provider_account>.<share_name>" or "<org>.<account>.<share_name>".
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:Pattern=`^[^.]+(\.[^.]+)+$`
	FromShare string `json:"fromShare"`

	// Comment is an optional description.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// ExternalVolume specifies the external volume for Iceberg table storage.
	// +optional
	ExternalVolume *string `json:"externalVolume,omitempty" snowflake:"EXTERNAL_VOLUME"`

	// Catalog specifies an Apache Iceberg catalog integration name.
	// +optional
	Catalog *string `json:"catalog,omitempty" snowflake:"CATALOG"`

	// DefaultDDLCollation sets the default collation for string columns.
	// +optional
	DefaultDDLCollation *string `json:"defaultDDLCollation,omitempty" snowflake:"DEFAULT_DDL_COLLATION"`

	// ReplaceInvalidCharacters controls whether to replace invalid UTF-8 characters.
	// +optional
	ReplaceInvalidCharacters *bool `json:"replaceInvalidCharacters,omitempty" snowflake:"REPLACE_INVALID_CHARACTERS"`

	// StorageSerializationPolicy controls storage serialization format.
	// +optional
	// +kubebuilder:validation:Enum=COMPATIBLE;OPTIMIZED
	StorageSerializationPolicy *StorageSerializationPolicy `json:"storageSerializationPolicy,omitempty" snowflake:"STORAGE_SERIALIZATION_POLICY"`

	// LogLevel controls the logging verbosity.
	// +optional
	// +kubebuilder:validation:Enum=TRACE;DEBUG;INFO;WARN;ERROR;FATAL;OFF
	LogLevel *LogLevel `json:"logLevel,omitempty" snowflake:"LOG_LEVEL"`

	// TraceLevel controls the trace collection level.
	// +optional
	// +kubebuilder:validation:Enum=ALWAYS;ON_EVENT;OFF
	TraceLevel *TraceLevel `json:"traceLevel,omitempty" snowflake:"TRACE_LEVEL"`
}

// SharedDatabaseShowOutput mirrors the SHOW DATABASES output for a shared database.
type SharedDatabaseShowOutput struct {
	// CreatedOn is the timestamp when the database was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the database name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Kind is the database kind.
	Kind string `json:"kind,omitempty"`

	// Comment is the database description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns the database.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`

	// Origin is the share identifier (<provider_account>.<share_name>).
	Origin string `json:"origin,omitempty"`
}

// SharedDatabaseStatus defines the observed state of a SharedDatabase.
type SharedDatabaseStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW DATABASES output for this database.
	ShowOutput *SharedDatabaseShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// SharedDatabase is the Schema for the shareddatabases API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=shdb
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="FROM-SHARE",type=string,JSONPath=`.spec.fromShare`,priority=1
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type SharedDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedDatabaseSpec   `json:"spec,omitempty"`
	Status SharedDatabaseStatus `json:"status,omitempty"`
}

// SharedDatabaseList contains a list of SharedDatabase.
// +kubebuilder:object:root=true
type SharedDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SharedDatabase{}, &SharedDatabaseList{})
}
