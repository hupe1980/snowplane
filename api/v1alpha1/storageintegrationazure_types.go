package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageIntegrationAzureSpec defines the desired state of a Snowflake Storage Integration for Azure Blob Storage.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type StorageIntegrationAzureSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake storage integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// StorageAllowedLocations lists Azure storage locations (URIs) that the
	// integration can access (e.g. "azure://account.blob.core.windows.net/container/path/").
	// +kubebuilder:validation:MinItems=1
	StorageAllowedLocations []string `json:"storageAllowedLocations" snowflake:"STORAGE_ALLOWED_LOCATIONS,nounset"`

	// StorageBlockedLocations lists Azure storage locations (URIs) that the
	// integration is explicitly denied access to.
	// +optional
	StorageBlockedLocations []string `json:"storageBlockedLocations,omitempty" snowflake:"STORAGE_BLOCKED_LOCATIONS"`

	// AzureTenantID is the Azure AD tenant ID. Required.
	// +kubebuilder:validation:MinLength=1
	AzureTenantID string `json:"azureTenantID" snowflake:"AZURE_TENANT_ID,nounset"`

	// Comment is an optional description for the storage integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// StorageIntegrationAzureShowOutput mirrors the SHOW STORAGE INTEGRATIONS output stored in status.
type StorageIntegrationAzureShowOutput struct {
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

// StorageIntegrationAzureStatus defines the observed state of a StorageIntegrationAzure.
type StorageIntegrationAzureStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW STORAGE INTEGRATIONS output.
	ShowOutput *StorageIntegrationAzureShowOutput `json:"showOutput,omitempty"`

	// AzureConsentURL is the Azure consent URL (from DESCRIBE).
	// Users need this to grant Snowflake access.
	AzureConsentURL string `json:"azureConsentURL,omitempty"`

	// AzureMultiTenantAppName is the Azure multi-tenant app name (from DESCRIBE).
	AzureMultiTenantAppName string `json:"azureMultiTenantAppName,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// StorageIntegrationAzure is the Schema for the storageintegrationazures API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=stazure
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type StorageIntegrationAzure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageIntegrationAzureSpec   `json:"spec,omitempty"`
	Status StorageIntegrationAzureStatus `json:"status,omitempty"`
}

// StorageIntegrationAzureList contains a list of StorageIntegrationAzure.
// +kubebuilder:object:root=true
type StorageIntegrationAzureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageIntegrationAzure `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageIntegrationAzure{}, &StorageIntegrationAzureList{})
}
