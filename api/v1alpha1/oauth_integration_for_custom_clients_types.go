package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OAuthIntegrationForCustomClientsSpec defines the desired state of a Snowflake OAuth Integration for Custom Clients.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.oauthClientType == oldSelf.oauthClientType",message="spec.oauthClientType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type OAuthIntegrationForCustomClientsSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// OAuthClientType specifies the type of client being registered.
	// +kubebuilder:validation:Enum=PUBLIC;CONFIDENTIAL
	OAuthClientType string `json:"oauthClientType" snowflake:"OAUTH_CLIENT_TYPE,nounset"`

	// OAuthRedirectURI is the client redirect URI.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	OAuthRedirectURI string `json:"oauthRedirectURI" snowflake:"OAUTH_REDIRECT_URI,nounset"`

	// OAuthAllowNonTLSRedirectURI allows non-TLS redirect URIs.
	// +optional
	OAuthAllowNonTLSRedirectURI *bool `json:"oauthAllowNonTLSRedirectURI,omitempty" snowflake:"OAUTH_ALLOW_NON_TLS_REDIRECT_URI"`

	// OAuthEnforcePKCE enforces PKCE for the integration.
	// +optional
	OAuthEnforcePKCE *bool `json:"oauthEnforcePKCE,omitempty" snowflake:"OAUTH_ENFORCE_PKCE"`

	// OAuthUseSecondaryRoles specifies secondary role behavior.
	// +optional
	// +kubebuilder:validation:Enum=IMPLICIT;NONE
	OAuthUseSecondaryRoles *string `json:"oauthUseSecondaryRoles,omitempty" snowflake:"OAUTH_USE_SECONDARY_ROLES"`

	// PreAuthorizedRolesList is the list of Snowflake roles a user does not need to consent to.
	// +optional
	PreAuthorizedRolesList []string `json:"preAuthorizedRolesList,omitempty" snowflake:"PRE_AUTHORIZED_ROLES_LIST"`

	// BlockedRolesList is the list of Snowflake roles that cannot be used with this integration.
	// +optional
	BlockedRolesList []string `json:"blockedRolesList,omitempty" snowflake:"BLOCKED_ROLES_LIST"`

	// OAuthIssueRefreshTokens controls whether refresh tokens are issued.
	// +optional
	OAuthIssueRefreshTokens *bool `json:"oauthIssueRefreshTokens,omitempty" snowflake:"OAUTH_ISSUE_REFRESH_TOKENS"`

	// OAuthRefreshTokenValidity is the validity period for refresh tokens in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	OAuthRefreshTokenValidity *int64 `json:"oauthRefreshTokenValidity,omitempty" snowflake:"OAUTH_REFRESH_TOKEN_VALIDITY"`

	// NetworkPolicy is the optional network policy attached to the integration.
	// +optional
	NetworkPolicy *string `json:"networkPolicy,omitempty" snowflake:"NETWORK_POLICY"`

	// OAuthClientRSAPublicKey is the RSA public key for the client (PEM without headers).
	// +optional
	OAuthClientRSAPublicKey *string `json:"oauthClientRSAPublicKey,omitempty" snowflake:"OAUTH_CLIENT_RSA_PUBLIC_KEY"`

	// OAuthClientRSAPublicKey2 is the second RSA public key for key rotation.
	// +optional
	OAuthClientRSAPublicKey2 *string `json:"oauthClientRSAPublicKey2,omitempty" snowflake:"OAUTH_CLIENT_RSA_PUBLIC_KEY_2"`

	// Comment is an optional comment for the integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// OAuthIntegrationForCustomClientsShowOutput holds the values from SHOW SECURITY INTEGRATIONS.
type OAuthIntegrationForCustomClientsShowOutput struct {
	CreatedOn string `json:"createdOn,omitempty"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Category  string `json:"category,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// OAuthIntegrationForCustomClientsStatus defines the observed state of OAuthIntegrationForCustomClients.
type OAuthIntegrationForCustomClientsStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *OAuthIntegrationForCustomClientsShowOutput `json:"showOutput,omitempty"`
	DescribeOutput    map[string]string                           `json:"describeOutput,omitempty"`
	TrackedParameters []string                                    `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=oauthcc,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=`.status.conditions[?(@.type=='Synced')].status`
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".status.fullyQualifiedName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// OAuthIntegrationForCustomClients is the Schema for the Snowflake OAuth Integration for Custom Clients API.
type OAuthIntegrationForCustomClients struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthIntegrationForCustomClientsSpec   `json:"spec,omitempty"`
	Status            OAuthIntegrationForCustomClientsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OAuthIntegrationForCustomClientsList contains a list of OAuthIntegrationForCustomClients.
type OAuthIntegrationForCustomClientsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthIntegrationForCustomClients `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OAuthIntegrationForCustomClients{}, &OAuthIntegrationForCustomClientsList{})
}
