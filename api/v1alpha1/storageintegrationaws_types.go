package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageIntegrationAWSSpec defines the desired state of a Snowflake Storage Integration for AWS S3.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type StorageIntegrationAWSSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake storage integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// StorageAllowedLocations lists S3 storage locations (URIs) that the
	// integration can access (e.g. "s3://bucket/path/").
	// +kubebuilder:validation:MinItems=1
	StorageAllowedLocations []string `json:"storageAllowedLocations" snowflake:"STORAGE_ALLOWED_LOCATIONS,nounset"`

	// StorageBlockedLocations lists S3 storage locations (URIs) that the
	// integration is explicitly denied access to.
	// +optional
	StorageBlockedLocations []string `json:"storageBlockedLocations,omitempty" snowflake:"STORAGE_BLOCKED_LOCATIONS"`

	// StorageAWSRoleARN is the IAM role ARN that Snowflake assumes for S3 access.
	// Required.
	// +kubebuilder:validation:MinLength=1
	StorageAWSRoleARN string `json:"storageAWSRoleARN" snowflake:"STORAGE_AWS_ROLE_ARN,nounset"`

	// StorageAWSExternalID optionally specifies an external ID that Snowflake
	// uses to establish a trust relationship with AWS. If omitted, Snowflake
	// auto-generates one during CREATE.
	// +optional
	StorageAWSExternalID *string `json:"storageAWSExternalID,omitempty" snowflake:"STORAGE_AWS_EXTERNAL_ID,nounset"`

	// Comment is an optional description for the storage integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// StorageIntegrationAWSShowOutput mirrors the SHOW STORAGE INTEGRATIONS output stored in status.
type StorageIntegrationAWSShowOutput struct {
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

// StorageIntegrationAWSStatus defines the observed state of a StorageIntegrationAWS.
type StorageIntegrationAWSStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW STORAGE INTEGRATIONS output.
	ShowOutput *StorageIntegrationAWSShowOutput `json:"showOutput,omitempty"`

	// StorageAWSIAMUserARN is the Snowflake IAM user ARN (from DESCRIBE).
	// Users need this to configure the IAM trust policy.
	StorageAWSIAMUserARN string `json:"storageAWSIAMUserARN,omitempty"`

	// StorageAWSExternalID is the external ID from DESCRIBE.
	StorageAWSExternalID string `json:"storageAWSExternalID,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// StorageIntegrationAWS is the Schema for the storageintegrationaws API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=staws
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type StorageIntegrationAWS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StorageIntegrationAWSSpec   `json:"spec,omitempty"`
	Status StorageIntegrationAWSStatus `json:"status,omitempty"`
}

// StorageIntegrationAWSList contains a list of StorageIntegrationAWS.
// +kubebuilder:object:root=true
type StorageIntegrationAWSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StorageIntegrationAWS `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StorageIntegrationAWS{}, &StorageIntegrationAWSList{})
}
