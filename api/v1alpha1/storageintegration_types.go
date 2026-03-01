package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageIntegrationType specifies the cloud provider type for a storage integration.
// +kubebuilder:validation:Enum=EXTERNAL_STAGE
type StorageIntegrationType string

// Valid StorageIntegrationType values.
const (
	StorageIntegrationTypeExternalStage StorageIntegrationType = "EXTERNAL_STAGE"
)

// StorageIntegrationSpec defines the desired state of a Snowflake Storage Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.storageProvider == oldSelf.storageProvider",message="spec.storageProvider is immutable (delete and recreate the resource to change)"
type StorageIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake storage integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type specifies the integration type (currently only EXTERNAL_STAGE).
	// Immutable after creation.
	// +kubebuilder:default=EXTERNAL_STAGE
	Type StorageIntegrationType `json:"type"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// StorageAllowedLocations lists cloud storage locations (URIs) that the
	// integration can access.
	// +kubebuilder:validation:MinItems=1
	StorageAllowedLocations []string `json:"storageAllowedLocations" snowflake:"STORAGE_ALLOWED_LOCATIONS,nounset"`

	// StorageBlockedLocations lists cloud storage locations (URIs) that the
	// integration is explicitly denied access to.
	// +optional
	StorageBlockedLocations []string `json:"storageBlockedLocations,omitempty" snowflake:"STORAGE_BLOCKED_LOCATIONS"`

	// StorageProvider specifies the cloud provider (S3, GCS, AZURE).
	// Immutable after creation because the provider-specific config changes shape.
	// +kubebuilder:validation:Enum=S3;GCS;AZURE
	StorageProvider string `json:"storageProvider"`

	// StorageAWSRoleARN is the IAM role ARN that Snowflake assumes for S3 access.
	// Required when storageProvider=S3.
	// +optional
	StorageAWSRoleARN *string `json:"storageAWSRoleARN,omitempty" snowflake:"STORAGE_AWS_ROLE_ARN,nounset"`

	// StorageAWSExternalID optionally specifies an external ID that Snowflake
	// uses to establish a trust relationship with AWS.  If omitted, Snowflake
	// auto-generates one during CREATE.
	// +optional
	StorageAWSExternalID *string `json:"storageAWSExternalID,omitempty" snowflake:"STORAGE_AWS_EXTERNAL_ID,nounset"`

	// AzureTenantID is the Azure AD tenant ID for AZURE integrations.
	// +optional
	AzureTenantID *string `json:"azureTenantID,omitempty" snowflake:"AZURE_TENANT_ID,nounset"`

	// Comment is an optional description for the storage integration.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// StorageIntegrationShowOutput mirrors the SHOW STORAGE INTEGRATIONS output stored in status.
type StorageIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the integration type.
	Type string `json:"type,omitempty"`

	// Category is the integration category (STORAGE).
	Category string `json:"category,omitempty"`

	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Comment is the integration description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// StorageIntegrationStatus defines the observed state of a StorageIntegration.
type StorageIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW STORAGE INTEGRATIONS output.
	ShowOutput *StorageIntegrationShowOutput `json:"showOutput,omitempty"`

	// StorageAWSIAMUserARN is the Snowflake IAM user ARN (from DESCRIBE).
	// Users need this to configure the IAM trust policy.
	StorageAWSIAMUserARN string `json:"storageAWSIAMUserARN,omitempty"`

	// StorageAWSExternalID is the external ID from DESCRIBE.
	StorageAWSExternalID string `json:"storageAWSExternalID,omitempty" snowflake:"STORAGE_AWS_EXTERNAL_ID,nounset"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// StorageIntegration is the Schema for the storageintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type StorageIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageIntegrationSpec   `json:"spec,omitempty"`
	Status StorageIntegrationStatus `json:"status,omitempty"`
}

// StorageIntegrationList contains a list of StorageIntegration.
// +kubebuilder:object:root=true
type StorageIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageIntegration{}, &StorageIntegrationList{})
}
