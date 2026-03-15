package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SCIMIntegrationSpec defines the desired state of a Snowflake SCIM Security Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.scimClient == oldSelf.scimClient",message="spec.scimClient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.runAsRole == oldSelf.runAsRole",message="spec.runAsRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type SCIMIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// SCIMClient is the identity provider type.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=OKTA;AZURE;GENERIC
	SCIMClient string `json:"scimClient"`

	// RunAsRole is the Snowflake role used to create users and groups via SCIM provisioning.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	RunAsRole string `json:"runAsRole"`

	// NetworkPolicy is the optional network policy attached to the SCIM integration.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	NetworkPolicy *string `json:"networkPolicy,omitempty" snowflake:"NETWORK_POLICY"`

	// SyncPassword controls whether user passwords are synced from the identity provider.
	// +optional
	SyncPassword *bool `json:"syncPassword,omitempty" snowflake:"SYNC_PASSWORD"`

	// Comment is an optional description for the integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SCIMIntegrationShowOutput mirrors the SHOW SECURITY INTEGRATIONS output for a SCIM integration.
type SCIMIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the integration type (SCIM).
	Type string `json:"type,omitempty"`

	// Category is the integration category (SECURITY).
	Category string `json:"category,omitempty"`

	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Comment is the integration description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SCIMIntegrationStatus defines the observed state of a SCIMIntegration.
type SCIMIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW SECURITY INTEGRATIONS output.
	ShowOutput *SCIMIntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput is a map of DESCRIBE INTEGRATION key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// SCIMIntegration is the Schema for the scimintegrations API.
// It manages a Snowflake SCIM security integration with dedicated, type-safe fields.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=scim
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="SCIM-CLIENT",type=string,JSONPath=`.spec.scimClient`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type SCIMIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SCIMIntegrationSpec   `json:"spec,omitempty"`
	Status SCIMIntegrationStatus `json:"status,omitempty"`
}

// SCIMIntegrationList contains a list of SCIMIntegration.
// +kubebuilder:object:root=true
type SCIMIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCIMIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SCIMIntegration{}, &SCIMIntegrationList{})
}
