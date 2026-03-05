package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIIntegrationSpec defines the desired state of a Snowflake API Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.apiProvider == oldSelf.apiProvider",message="spec.apiProvider is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type APIIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake API integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// APIProvider specifies the cloud provider for the API integration. Immutable after creation.
	// +kubebuilder:validation:Enum=aws_api_gateway;aws_private_api_gateway;aws_gov_api_gateway;aws_gov_private_api_gateway;azure_api_management;google_api_gateway;git_https_api
	APIProvider string `json:"apiProvider"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED"`

	// APIAllowedPrefixes is the list of HTTPS endpoints allowed by this integration.
	// +kubebuilder:validation:MinItems=1
	APIAllowedPrefixes []string `json:"apiAllowedPrefixes" snowflake:"API_ALLOWED_PREFIXES,nounset"`

	// APIBlockedPrefixes is the list of HTTPS endpoints blocked by this integration.
	// +optional
	APIBlockedPrefixes []string `json:"apiBlockedPrefixes,omitempty" snowflake:"API_BLOCKED_PREFIXES"`

	// APIAWSRoleARN is the IAM role ARN for AWS API Gateway integrations.
	// Required when apiProvider is aws_api_gateway, aws_private_api_gateway,
	// aws_gov_api_gateway, or aws_gov_private_api_gateway.
	// +optional
	APIAWSRoleARN *string `json:"apiAwsRoleArn,omitempty" snowflake:"API_AWS_ROLE_ARN"`

	// AzureTenantID is the Azure AD tenant ID. Required when apiProvider is azure_api_management.
	// +optional
	AzureTenantID *string `json:"azureTenantId,omitempty" snowflake:"AZURE_TENANT_ID,nounset"`

	// AzureADApplicationID is the Azure AD application (client) ID.
	// Required when apiProvider is azure_api_management.
	// +optional
	AzureADApplicationID *string `json:"azureAdApplicationId,omitempty" snowflake:"AZURE_AD_APPLICATION_ID"`

	// GoogleAudience is the audience claim for Google API Gateway integrations.
	// Required when apiProvider is google_api_gateway.
	// +optional
	GoogleAudience *string `json:"googleAudience,omitempty" snowflake:"GOOGLE_AUDIENCE,nounset"`

	// APIKey is an optional API key for the integration.
	// +optional
	APIKey *string `json:"apiKey,omitempty" snowflake:"API_KEY"` //nolint:gosec // G117: API key, not a secret credential

	// Comment is an optional comment for the integration.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// APIIntegrationShowOutput holds the values from SHOW API INTEGRATIONS.
type APIIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`
	// Name is the Snowflake integration name.
	Name string `json:"name,omitempty"`
	// Type is the integration sub-type (e.g. EXTERNAL_API).
	Type string `json:"type,omitempty"`
	// Category is the integration category (API).
	Category string `json:"category,omitempty"`
	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty"`
	// Comment is the integration comment.
	Comment string `json:"comment,omitempty"`
}

// APIIntegrationStatus defines the observed state of APIIntegration.
type APIIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the most recently observed SHOW API INTEGRATIONS output.
	// +optional
	ShowOutput *APIIntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the most recently observed DESCRIBE INTEGRATION output.
	// +optional
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters records which Snowflake parameters were last applied by the controller.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=apii
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// APIIntegration is the Schema for the apiintegrations API.
type APIIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIIntegrationSpec   `json:"spec,omitempty"`
	Status APIIntegrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIIntegrationList contains a list of APIIntegration.
type APIIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIIntegration{}, &APIIntegrationList{})
}
