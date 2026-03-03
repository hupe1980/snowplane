package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIAuthenticationIntegrationWithJWTBearerSpec defines the desired state
// of a Snowflake API Authentication Security Integration using the OAuth2
// JWT bearer grant flow.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate)"
type APIAuthenticationIntegrationWithJWTBearerSpec struct {
	CommonSpec `json:",inline"`

	// Snowflake integration identifier (e.g. 'MY_API_AUTH_JWT').
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Specifies whether this security integration is enabled or disabled.
	Enabled bool `json:"enabled" snowflake:"ENABLED"`

	// Specifies the client ID for the OAuth application in the external service.
	OAuthClientID string `json:"oauthClientId" snowflake:"OAUTH_CLIENT_ID"`

	// Specifies the client secret for the OAuth application in the external service.
	// References a Kubernetes Secret to avoid storing credentials in the CRD spec.
	OAuthClientSecretRef SecretKeyReference `json:"oauthClientSecretRef"`

	// Specifies the assertion issuer for the JWT bearer flow.
	OAuthAssertionIssuer string `json:"oauthAssertionIssuer" snowflake:"OAUTH_ASSERTION_ISSUER"`

	// Specifies the token endpoint used by the client to obtain an access token.
	// +optional
	OAuthTokenEndpoint *string `json:"oauthTokenEndpoint,omitempty" snowflake:"OAUTH_TOKEN_ENDPOINT"`

	// Specifies the URL for authenticating to the external service.
	// +optional
	OAuthAuthorizationEndpoint *string `json:"oauthAuthorizationEndpoint,omitempty" snowflake:"OAUTH_AUTHORIZATION_ENDPOINT"`

	// Specifies how client credentials are sent to the external service.
	// +optional
	OAuthClientAuthMethod *OAuthClientAuthMethod `json:"oauthClientAuthMethod,omitempty" snowflake:"OAUTH_CLIENT_AUTH_METHOD"`

	// Specifies the default lifetime of the OAuth access token (in seconds).
	// +optional
	OAuthAccessTokenValidity *int `json:"oauthAccessTokenValidity,omitempty" snowflake:"OAUTH_ACCESS_TOKEN_VALIDITY"`

	// Specifies the validity of the refresh token obtained from the OAuth server.
	// +optional
	OAuthRefreshTokenValidity *int `json:"oauthRefreshTokenValidity,omitempty" snowflake:"OAUTH_REFRESH_TOKEN_VALIDITY"`

	// Comment for the integration.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// APIAuthenticationIntegrationWithJWTBearerStatus defines the observed state.
type APIAuthenticationIntegrationWithJWTBearerStatus struct {
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
// +kubebuilder:resource:categories=snowplane,shortName=aaiwjb
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// APIAuthenticationIntegrationWithJWTBearer is the Schema for the Snowflake
// API Authentication Security Integration with JWT Bearer API.
type APIAuthenticationIntegrationWithJWTBearer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIAuthenticationIntegrationWithJWTBearerSpec   `json:"spec,omitempty"`
	Status APIAuthenticationIntegrationWithJWTBearerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIAuthenticationIntegrationWithJWTBearerList contains a list of
// APIAuthenticationIntegrationWithJWTBearer.
type APIAuthenticationIntegrationWithJWTBearerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIAuthenticationIntegrationWithJWTBearer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIAuthenticationIntegrationWithJWTBearer{}, &APIAuthenticationIntegrationWithJWTBearerList{})
}
