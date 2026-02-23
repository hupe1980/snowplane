package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StageType indicates whether the stage is internal or external.
type StageType string

// Valid StageType values.
const (
	StageTypeInternal StageType = "INTERNAL"
	StageTypeExternal StageType = "EXTERNAL"
)

// StageEncryption specifies the encryption type for a stage.
type StageEncryption struct {
	// Type is the encryption type (e.g. SNOWFLAKE_FULL, SNOWFLAKE_SSE, AWS_SSE_S3, AWS_SSE_KMS, etc.).
	// +kubebuilder:validation:Enum=SNOWFLAKE_FULL;SNOWFLAKE_SSE;AWS_CSE;AWS_SSE_S3;AWS_SSE_KMS;GCS_SSE_KMS;AZURE_CSE;NONE
	Type string `json:"type"`
}

// StageDirectoryOptions configures the directory table for a stage.
type StageDirectoryOptions struct {
	// Enable enables or disables the directory table.
	Enable bool `json:"enable"`

	// AutoRefresh enables automatic refresh for external stage directories.
	// +optional
	AutoRefresh *bool `json:"autoRefresh,omitempty"`
}

// StageSpec defines the desired state of a Stage.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type StageSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake stage name. Immutable after creation.
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

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN (e.g. '"ANALYTICS"."PUBLIC"').
	// Use this when the schema is NOT managed by Snowplane.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// URL is the external stage URL. Required for external stages, omitted for internal.
	// Immutable: changing from internal to external (or vice versa) is not supported.
	// +optional
	URL *string `json:"url,omitempty"`

	// StorageIntegration is the name of the storage integration for external stages.
	// +optional
	StorageIntegration *string `json:"storageIntegration,omitempty"`

	// Encryption specifies the encryption settings.
	// +optional
	Encryption *StageEncryption `json:"encryption,omitempty"`

	// Directory configures the directory table settings.
	// +optional
	Directory *StageDirectoryOptions `json:"directory,omitempty"`

	// FileFormat is the named file format (FORMAT_NAME) or inline options.
	// +optional
	FileFormat *string `json:"fileFormat,omitempty"`

	// Comment is an optional description for the stage.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// StageShowOutput mirrors the SHOW STAGES output stored in status.
type StageShowOutput struct {
	// CreatedOn is the timestamp when the stage was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the stage name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// URL is the external stage URL (blank for internal).
	URL string `json:"url,omitempty"`

	// Owner is the role that owns the stage.
	Owner string `json:"owner,omitempty"`

	// Comment is the stage description.
	Comment string `json:"comment,omitempty"`

	// Type is the stage type (INTERNAL or EXTERNAL).
	Type string `json:"type,omitempty"`

	// StorageIntegration is the associated storage integration name.
	StorageIntegration string `json:"storageIntegration,omitempty"`

	// DirectoryEnabled indicates whether the directory table is enabled.
	DirectoryEnabled bool `json:"directoryEnabled,omitempty"`
}

// StageStatus defines the observed state of a Stage.
type StageStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW STAGES output for this stage.
	ShowOutput *StageShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Stage is the Schema for the stages API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Stage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StageSpec   `json:"spec,omitempty"`
	Status StageStatus `json:"status,omitempty"`
}

// StageList contains a list of Stage.
// +kubebuilder:object:root=true
type StageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Stage `json:"items"`
}

// GetConditions returns the conditions of the Stage.
func (s *Stage) GetConditions() []metav1.Condition {
	return s.Status.Conditions
}

// SetConditions sets the conditions of the Stage.
func (s *Stage) SetConditions(conditions []metav1.Condition) {
	s.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (s *Stage) GetDeletionPolicy() DeletionPolicy {
	if s.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return s.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (s *Stage) GetFullyQualifiedName() string {
	return s.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (s *Stage) GetSpecName() string { return s.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (s *Stage) GetProviderRef() ProviderReference { return s.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (s *Stage) GetUseRole() *string { return s.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (s *Stage) GetObservedGeneration() int64 { return s.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (s *Stage) SetObservedGeneration(v int64) { s.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (s *Stage) GetLastAppliedSpecHash() string { return s.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (s *Stage) SetLastAppliedSpecHash(v string) { s.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (s *Stage) GetTrackedParametersList() []string { return s.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (s *Stage) SetTrackedParametersList(v []string) { s.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (s *Stage) GetOwner() string {
	if s.Status.ShowOutput != nil {
		return s.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (s *Stage) ValidateSpec() error { return s.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (s *Stage) ComputeSpecHash() (string, error) { return ComputeSpecHash(s.Spec) }

// IsExternal returns true if the stage is configured as an external stage.
func (s *Stage) IsExternal() bool { return s.Spec.URL != nil && *s.Spec.URL != "" }

func init() {
	SchemeBuilder.Register(&Stage{}, &StageList{})
}
