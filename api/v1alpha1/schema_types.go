package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchemaSpec defines the desired state of a Schema.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
type SchemaSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake schema name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier (e.g. "ANALYTICS").
	// Use this when the database is NOT managed by Snowplane.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// Comment is an optional description for the schema.
	Comment *string `json:"comment,omitempty"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty"`

	// Transient indicates this is a transient schema (no Fail-safe). Immutable after creation.
	Transient bool `json:"transient,omitempty"`

	// ManagedAccess enables managed access mode for the schema.
	ManagedAccess bool `json:"managedAccess,omitempty"`

	// DefaultDDLCollation sets the default collation for string columns.
	DefaultDDLCollation *string `json:"defaultDdlCollation,omitempty"`

	// ReplaceInvalidCharacters controls whether to replace invalid UTF-8 characters.
	ReplaceInvalidCharacters *bool `json:"replaceInvalidCharacters,omitempty"`

	// StorageSerializationPolicy controls storage serialization format.
	StorageSerializationPolicy *StorageSerializationPolicy `json:"storageSerializationPolicy,omitempty"`

	// LogLevel controls the logging verbosity.
	LogLevel *LogLevel `json:"logLevel,omitempty"`

	// MetricLevel controls the metric collection level.
	MetricLevel *MetricLevel `json:"metricLevel,omitempty"`

	// TraceLevel controls the trace collection level.
	TraceLevel *TraceLevel `json:"traceLevel,omitempty"`
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
	Comment string `json:"comment,omitempty"`

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

// GetConditions returns the conditions of the Schema.
func (s *Schema) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

// SetConditions sets the conditions of the Schema.
func (s *Schema) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (s *Schema) GetDeletionPolicy() DeletionPolicy {
	if s.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return s.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (s *Schema) GetFullyQualifiedName() string {
	return s.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (s *Schema) GetSpecName() string { return s.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (s *Schema) GetProviderRef() ProviderReference { return s.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (s *Schema) GetUseRole() *string { return s.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (s *Schema) GetObservedGeneration() int64 { return s.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (s *Schema) SetObservedGeneration(v int64) { s.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (s *Schema) GetLastAppliedSpecHash() string { return s.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (s *Schema) SetLastAppliedSpecHash(v string) { s.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (s *Schema) GetTrackedParametersList() []string { return s.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (s *Schema) SetTrackedParametersList(v []string) { s.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (s *Schema) GetOwner() string {
	if s.Status.ShowOutput != nil {
		return s.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (s *Schema) ValidateSpec() error { return s.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (s *Schema) ComputeSpecHash() (string, error) { return ComputeSpecHash(s.Spec) }

func init() {
	SchemeBuilder.Register(&Schema{}, &SchemaList{})
}
