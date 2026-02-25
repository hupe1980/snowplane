package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecurityIntegrationType specifies the type of security integration.
// +kubebuilder:validation:Enum=API_AUTHENTICATION;EXTERNAL_OAUTH;SAML2;SCIM
type SecurityIntegrationType string

// Valid SecurityIntegrationType values.
const (
	SecurityIntegrationTypeAPIAuthentication SecurityIntegrationType = "API_AUTHENTICATION"
	SecurityIntegrationTypeExternalOAuth     SecurityIntegrationType = "EXTERNAL_OAUTH"
	SecurityIntegrationTypeSAML2             SecurityIntegrationType = "SAML2"
	SecurityIntegrationTypeSCIM              SecurityIntegrationType = "SCIM"
)

// ExternalOAuthConfig holds configuration for External OAuth security integrations.
type ExternalOAuthConfig struct {
	// Type is the external OAuth type (e.g. CUSTOM, AZURE, OKTA, PING_FEDERATE).
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// Issuer is the OAuth 2.0 token issuer URL.
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// TokenUserMappingClaim is the claim in the access token that identifies the user.
	// +kubebuilder:validation:MinLength=1
	TokenUserMappingClaim string `json:"tokenUserMappingClaim"`

	// SnowflakeUserMappingAttribute specifies how to map the claim to a Snowflake user
	// (LOGIN_NAME or EMAIL_ADDRESS).
	// +kubebuilder:validation:Enum=LOGIN_NAME;EMAIL_ADDRESS
	// +kubebuilder:default=LOGIN_NAME
	SnowflakeUserMappingAttribute string `json:"snowflakeUserMappingAttribute"`

	// JWSKeysURL is the endpoint to retrieve the JWKS (JSON Web Key Set) for token validation.
	// +optional
	JWSKeysURL *string `json:"jwsKeysURL,omitempty"`

	// AudienceList is the list of allowed audience values in the access token.
	// +optional
	AudienceList []string `json:"audienceList,omitempty"`

	// AllowedRoles is the list of Snowflake roles that can be assumed via this integration.
	// +optional
	AllowedRoles []string `json:"allowedRoles,omitempty"`

	// BlockedRoles is the list of Snowflake roles that cannot be assumed via this integration.
	// +optional
	BlockedRoles []string `json:"blockedRoles,omitempty"`

	// AnyRoleMode controls whether the ANY role is allowed.
	// +optional
	// +kubebuilder:validation:Enum=DISABLE;ENABLE;ENABLE_FOR_PRIVILEGE
	AnyRoleMode *string `json:"anyRoleMode,omitempty"`

	// ScopeDelimiter is the character used to separate scopes.
	// +optional
	ScopeDelimiter *string `json:"scopeDelimiter,omitempty"`

	// NetworkPolicy is the optional network policy attached to the integration.
	// +optional
	NetworkPolicy *string `json:"networkPolicy,omitempty"`
}

// SAML2Config holds configuration for SAML2 security integrations.
type SAML2Config struct {
	// Issuer is the SAML2 IdP entity ID / issuer.
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// SSOURL is the IdP SSO URL.
	// +kubebuilder:validation:MinLength=1
	SSOURL string `json:"ssoURL"`

	// Provider is the SAML IdP provider name (e.g. CUSTOM, OKTA, ADFS).
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// X509Cert is the Base64-encoded X.509 certificate from the IdP for signature validation.
	// +kubebuilder:validation:MinLength=1
	X509Cert string `json:"x509Cert"`

	// AllowedEmailPatterns is a list of email address patterns allowed for auto-provisioning.
	// +optional
	AllowedEmailPatterns []string `json:"allowedEmailPatterns,omitempty"`

	// AllowedUserDomains is a list of email domains allowed for authentication.
	// +optional
	AllowedUserDomains []string `json:"allowedUserDomains,omitempty"`

	// SPInitiatedLoginPageLabel is the label for the SP-initiated login page.
	// +optional
	SPInitiatedLoginPageLabel *string `json:"spInitiatedLoginPageLabel,omitempty"`

	// EnableSPInitiated controls whether SP-initiated login is enabled.
	// +optional
	EnableSPInitiated *bool `json:"enableSPInitiated,omitempty"`

	// ForceAuthn controls whether force authentication is requested.
	// +optional
	ForceAuthn *bool `json:"forceAuthn,omitempty"`

	// RequestedNameIDFormat specifies the NameID format to request from the IdP.
	// +optional
	// +kubebuilder:validation:Enum="urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified";"urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress";"urn:oasis:names:tc:SAML:2.0:nameid-format:persistent";"urn:oasis:names:tc:SAML:2.0:nameid-format:transient"
	RequestedNameIDFormat *string `json:"requestedNameIDFormat,omitempty"`

	// PostLogoutRedirectURL is the URL to redirect to after SAML logout.
	// +optional
	PostLogoutRedirectURL *string `json:"postLogoutRedirectURL,omitempty"`
}

// SCIMConfig holds configuration for SCIM security integrations.
type SCIMConfig struct {
	// SCIMClient is the identity provider type (OKTA, AZURE, GENERIC).
	// +kubebuilder:validation:Enum=OKTA;AZURE;GENERIC
	SCIMClient string `json:"scimClient"`

	// RunAsRole is the Snowflake role used to create users/groups via SCIM.
	// +kubebuilder:validation:MinLength=1
	RunAsRole string `json:"runAsRole"`

	// NetworkPolicy is the optional network policy attached to the SCIM integration.
	// +optional
	NetworkPolicy *string `json:"networkPolicy,omitempty"`

	// SyncPassword controls whether user passwords are synced from the IdP.
	// +optional
	SyncPassword *bool `json:"syncPassword,omitempty"`
}

// APIAuthenticationConfig holds configuration for API Authentication security integrations.
type APIAuthenticationConfig struct {
	// OAuthClientID is the OAuth 2.0 client ID.
	// +kubebuilder:validation:MinLength=1
	OAuthClientID string `json:"oauthClientID"`

	// OAuthClientSecret is the OAuth 2.0 client secret.
	// +kubebuilder:validation:MinLength=1
	OAuthClientSecret string `json:"oauthClientSecret"`

	// OAuthTokenEndpoint is the OAuth token endpoint.
	// +kubebuilder:validation:MinLength=1
	OAuthTokenEndpoint string `json:"oauthTokenEndpoint"`

	// OAuthAllowedScopes is the list of allowed OAuth scopes.
	// +optional
	OAuthAllowedScopes []string `json:"oauthAllowedScopes,omitempty"`

	// OAuthGrantType is the method for getting the access token.
	// +optional
	// +kubebuilder:validation:Enum=CLIENT_CREDENTIALS;AUTHORIZATION_CODE;JWT_BEARER
	OAuthGrantType *string `json:"oauthGrantType,omitempty"`
}

// SecurityIntegrationSpec defines the desired state of a Snowflake Security Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
type SecurityIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type specifies the integration type (API_AUTHENTICATION, EXTERNAL_OAUTH, SAML2, SCIM).
	// Immutable after creation.
	Type SecurityIntegrationType `json:"type"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// ExternalOAuth holds configuration for EXTERNAL_OAUTH integrations.
	// +optional
	ExternalOAuth *ExternalOAuthConfig `json:"externalOAuth,omitempty"`

	// SAML2 holds configuration for SAML2 integrations.
	// +optional
	SAML2 *SAML2Config `json:"saml2,omitempty"`

	// SCIM holds configuration for SCIM integrations.
	// +optional
	SCIM *SCIMConfig `json:"scim,omitempty"`

	// APIAuthentication holds configuration for API_AUTHENTICATION integrations.
	// +optional
	APIAuthentication *APIAuthenticationConfig `json:"apiAuthentication,omitempty"`

	// Comment is an optional description for the security integration.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// SecurityIntegrationShowOutput mirrors the SHOW SECURITY INTEGRATIONS output stored in status.
type SecurityIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the integration type.
	Type string `json:"type,omitempty"`

	// Category is the integration category (SECURITY).
	Category string `json:"category,omitempty"`

	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty"`

	// Comment is the integration description.
	Comment string `json:"comment,omitempty"`
}

// SecurityIntegrationStatus defines the observed state of a SecurityIntegration.
type SecurityIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW SECURITY INTEGRATIONS output.
	ShowOutput *SecurityIntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput is a map of DESCRIBE INTEGRATION key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// SecurityIntegration is the Schema for the securityintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type SecurityIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecurityIntegrationSpec   `json:"spec,omitempty"`
	Status SecurityIntegrationStatus `json:"status,omitempty"`
}

// SecurityIntegrationList contains a list of SecurityIntegration.
// +kubebuilder:object:root=true
type SecurityIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecurityIntegration{}, &SecurityIntegrationList{})
}
