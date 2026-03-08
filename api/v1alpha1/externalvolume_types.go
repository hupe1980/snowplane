package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalVolumeStorageLocation defines a single storage location within an external volume.
type ExternalVolumeStorageLocation struct {
	// Name is the unique name of this storage location within the external volume.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// StorageProvider identifies the cloud storage provider.
	// +kubebuilder:validation:Enum=S3;S3GOV;GCS;AZURE
	StorageProvider string `json:"storageProvider"`

	// StorageBaseURL is the base URL for the storage location
	// (e.g. "s3://bucket/path/", "gcs://bucket/", "azure://account.blob.core.windows.net/container/").
	// +kubebuilder:validation:MinLength=1
	StorageBaseURL string `json:"storageBaseURL"`

	// StorageAWSRoleARN is the IAM role ARN Snowflake assumes for S3/S3GOV access.
	// Required when storageProvider is S3 or S3GOV.
	// +optional
	StorageAWSRoleARN *string `json:"storageAWSRoleARN,omitempty"`

	// StorageAWSExternalID specifies an external ID for the S3/S3GOV trust policy.
	// +optional
	StorageAWSExternalID *string `json:"storageAWSExternalID,omitempty"`

	// EncryptionType specifies the encryption type for the storage location.
	// For S3/S3GOV: AWS_SSE_S3, AWS_SSE_KMS, or NONE.
	// For GCS: GCS_SSE_KMS or NONE.
	// +optional
	// +kubebuilder:validation:Enum=AWS_SSE_S3;AWS_SSE_KMS;GCS_SSE_KMS;NONE
	EncryptionType *string `json:"encryptionType,omitempty"`

	// EncryptionKMSKeyID specifies the KMS key ID for encryption
	// (required when encryptionType is AWS_SSE_KMS or GCS_SSE_KMS).
	// +optional
	EncryptionKMSKeyID *string `json:"encryptionKMSKeyID,omitempty"`

	// AzureTenantID is the Azure tenant ID. Required when storageProvider is AZURE.
	// +optional
	AzureTenantID *string `json:"azureTenantID,omitempty"`
}

// ExternalVolumeSpec defines the desired state of a Snowflake External Volume.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type ExternalVolumeSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake external volume name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// StorageLocations is the list of cloud storage locations that make up
	// this external volume. At least one location is required.
	// +kubebuilder:validation:MinItems=1
	StorageLocations []ExternalVolumeStorageLocation `json:"storageLocations"`

	// AllowWrites controls whether Snowflake can write to the external volume.
	// +optional
	// +kubebuilder:default=true
	AllowWrites *bool `json:"allowWrites,omitempty" snowflake:"ALLOW_WRITES"`

	// Comment is an optional description for the external volume.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ExternalVolumeShowOutput mirrors the SHOW EXTERNAL VOLUMES output stored in status.
type ExternalVolumeShowOutput struct {
	// Name is the external volume name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// AllowWrites indicates whether write access is enabled.
	AllowWrites bool `json:"allowWrites,omitempty" snowflake:"ALLOW_WRITES"`

	// Comment is the external volume description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ExternalVolumeStatus defines the observed state of an ExternalVolume.
type ExternalVolumeStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW EXTERNAL VOLUMES output.
	ShowOutput *ExternalVolumeShowOutput `json:"showOutput,omitempty"`

	// StorageLocationNames tracks the names of storage locations currently
	// present on the Snowflake external volume (from DESCRIBE).
	StorageLocationNames []string `json:"storageLocationNames,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// ExternalVolume is the Schema for the externalvolumes API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=extvol
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type ExternalVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalVolumeSpec   `json:"spec,omitempty"`
	Status ExternalVolumeStatus `json:"status,omitempty"`
}

// ExternalVolumeList contains a list of ExternalVolume.
// +kubebuilder:object:root=true
type ExternalVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalVolume{}, &ExternalVolumeList{})
}
