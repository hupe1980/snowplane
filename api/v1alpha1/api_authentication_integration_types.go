package v1alpha1

// OAuthClientAuthMethod specifies how client credentials are sent to the external service.
// +kubebuilder:validation:Enum=CLIENT_SECRET_BASIC;CLIENT_SECRET_POST
type OAuthClientAuthMethod string

// OAuth client authentication methods.
const (
	OAuthClientAuthMethodBasic OAuthClientAuthMethod = "CLIENT_SECRET_BASIC"
	OAuthClientAuthMethodPost  OAuthClientAuthMethod = "CLIENT_SECRET_POST"
)

// APIAuthenticationIntegrationDescribeOutput holds the output from DESCRIBE INTEGRATION.
type APIAuthenticationIntegrationDescribeOutput struct {
	// +optional
	AuthType *string `json:"authType,omitempty"`
	// +optional
	OAuthClientID *string `json:"oauthClientId,omitempty"`
	// +optional
	OAuthClientAuthMethod *string `json:"oauthClientAuthMethod,omitempty"`
	// +optional
	OAuthTokenEndpoint *string `json:"oauthTokenEndpoint,omitempty"`
	// +optional
	OAuthAuthorizationEndpoint *string `json:"oauthAuthorizationEndpoint,omitempty"`
	// +optional
	OAuthGrant *string `json:"oauthGrant,omitempty"`
	// +optional
	OAuthAccessTokenValidity *string `json:"oauthAccessTokenValidity,omitempty"`
	// +optional
	OAuthRefreshTokenValidity *string `json:"oauthRefreshTokenValidity,omitempty"`
	// +optional
	OAuthAllowedScopes *string `json:"oauthAllowedScopes,omitempty"`
	// +optional
	Enabled *string `json:"enabled,omitempty"`
	// +optional
	Comment *string `json:"comment,omitempty"`
	// +optional
	ParentIntegration *string `json:"parentIntegration,omitempty"`
}
