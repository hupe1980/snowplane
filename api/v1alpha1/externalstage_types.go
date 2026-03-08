package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalStageEncryption specifies the encryption type for an external stage.
type ExternalStageEncryption struct {
	// Type is the encryption type for external stages.
	// +kubebuilder:validation:Enum=AWS_CSE;AWS_SSE_S3;AWS_SSE_KMS;GCS_SSE_KMS;AZURE_CSE;NONE
	Type string `json:"type"`
}

// ExternalStageDirectoryOptions configures the directory table for an external stage.
type ExternalStageDirectoryOptions struct {
	// Enable enables or disables the directory table.
	Enable bool `json:"enable"`

	// AutoRefresh enables automatic refresh for external stage directories.
	// +optional
	AutoRefresh *bool `json:"autoRefresh,omitempty"`

	// RefreshOnCreate triggers an automatic refresh of the directory table
	// metadata when the stage is created. Default is true.
	// +optional
	RefreshOnCreate *bool `json:"refreshOnCreate,omitempty"`

	// NotificationIntegration specifies the notification integration for
	// automatic directory table metadata refreshes.
	// +optional
	NotificationIntegration *string `json:"notificationIntegration,omitempty"`
}

// ExternalStageSpec defines the desired state of an ExternalStage.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.url == oldSelf.url",message="spec.url is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type ExternalStageSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake stage name. Immutable after creation.
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

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SchemaName *string `json:"schemaName,omitempty"`

	// URL is the external stage URL (e.g. "s3://bucket/path/"). Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	URL string `json:"url"`

	// StorageIntegration is the name of the storage integration for the external stage.
	// +optional
	StorageIntegration *string `json:"storageIntegration,omitempty" snowflake:"STORAGE_INTEGRATION"`

	// Encryption specifies the encryption settings for the external stage.
	// +optional
	Encryption *ExternalStageEncryption `json:"encryption,omitempty"`

	// Directory configures the directory table settings.
	// +optional
	Directory *ExternalStageDirectoryOptions `json:"directory,omitempty" snowflake:"DIRECTORY"`

	// FileFormat is the named file format (FORMAT_NAME) or inline options.
	// +optional
	FileFormat *string `json:"fileFormat,omitempty" snowflake:"FILE_FORMAT"`

	// Comment is an optional description for the stage.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ExternalStageShowOutput mirrors the SHOW STAGES output stored in status.
type ExternalStageShowOutput struct {
	// CreatedOn is the timestamp when the stage was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the stage name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// URL is the external stage URL.
	URL string `json:"url,omitempty" snowflake:"URL"`

	// Owner is the role that owns the stage.
	Owner string `json:"owner,omitempty"`

	// Comment is the stage description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// StorageIntegration is the associated storage integration name.
	StorageIntegration string `json:"storageIntegration,omitempty" snowflake:"STORAGE_INTEGRATION"`

	// DirectoryEnabled indicates whether the directory table is enabled.
	DirectoryEnabled bool `json:"directoryEnabled,omitempty"`
}

// ExternalStageStatus defines the observed state of an ExternalStage.
type ExternalStageStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW STAGES output for this stage.
	ShowOutput *ExternalStageShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// ExternalStage is the Schema for the externalstages API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=estg
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type ExternalStage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalStageSpec   `json:"spec,omitempty"`
	Status ExternalStageStatus `json:"status,omitempty"`
}

// ExternalStageList contains a list of ExternalStage.
// +kubebuilder:object:root=true
type ExternalStageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalStage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalStage{}, &ExternalStageList{})
}
