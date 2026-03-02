package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIAuthenticationIntegrationWithClientCredentialsSpec defines the desired state
// of a Snowflake API Authentication Security Integration using the OAuth2 client
// credentials grant flow.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate)"
type APIAuthenticationIntegrationWithClientCredentialsSpec struct {
	CommonSpec `json:",inline"`

	// Snowflake integration identifier (e.g. 'MY_API_AUTH_CC').
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Specifies whether this security integration is enabled or disabled.
	Enabled bool `json:"enabled" snowflake:"ENABLED"`

	// Specifies the client ID for the OAuth application in the external service.
	OAuthClientID string `json:"oauthClientId" snowflake:"OAUTH_CLIENT_ID"`

	// Specifies the client secret for the OAuth application in the external service.
	OAuthClientSecret string `json:"oauthClientSecret" snowflake:"OAUTH_CLIENT_SECRET"`

	// Specifies the token endpoint used by the client to obtain an access token.
	// +optional
	OAuthTokenEndpoint *string `json:"oauthTokenEndpoint,omitempty" snowflake:"OAUTH_TOKEN_ENDPOINT"`

	// Specifies how client credentials are sent to the external service.
	// +optional
	OAuthClientAuthMethod *OAuthClientAuthMethod `json:"oauthClientAuthMethod,omitempty" snowflake:"OAUTH_CLIENT_AUTH_METHOD"`

	// Specifies the default lifetime of the OAuth access token (in seconds).
	// +optional
	OAuthAccessTokenValidity *int `json:"oauthAccessTokenValidity,omitempty" snowflake:"OAUTH_ACCESS_TOKEN_VALIDITY"`

	// Specifies the list of scopes to use during the OAuth client credentials flow.
	// +optional
	OAuthAllowedScopes []string `json:"oauthAllowedScopes,omitempty" snowflake:"OAUTH_ALLOWED_SCOPES"`

	// Comment for the integration.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// APIAuthenticationIntegrationWithClientCredentialsStatus defines the observed state.
type APIAuthenticationIntegrationWithClientCredentialsStatus struct {
	CommonStatus `json:",inline"`

	// Output from SHOW SECURITY INTEGRATIONS.
	// +optional
	ShowOutput *APIAuthenticationIntegrationShowOutput `json:"showOutput,omitempty"`

	// Output from DESCRIBE INTEGRATION.
	// +optional
	DescribeOutput *APIAuthenticationIntegrationDescribeOutput `json:"describeOutput,omitempty"`

	// Tracked parameters for UNSET operations.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=aaiwcc
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// APIAuthenticationIntegrationWithClientCredentials is the Schema for the Snowflake
// API Authentication Security Integration with Client Credentials API.
type APIAuthenticationIntegrationWithClientCredentials struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIAuthenticationIntegrationWithClientCredentialsSpec   `json:"spec,omitempty"`
	Status APIAuthenticationIntegrationWithClientCredentialsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIAuthenticationIntegrationWithClientCredentialsList contains a list of
// APIAuthenticationIntegrationWithClientCredentials.
type APIAuthenticationIntegrationWithClientCredentialsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIAuthenticationIntegrationWithClientCredentials `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIAuthenticationIntegrationWithClientCredentials{}, &APIAuthenticationIntegrationWithClientCredentialsList{})
}
