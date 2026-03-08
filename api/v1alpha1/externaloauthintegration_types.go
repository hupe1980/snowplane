package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalOAuthIntegrationSpec defines the desired state of a Snowflake External OAuth Security Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.externalOAuthType == oldSelf.externalOAuthType",message="spec.externalOAuthType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.issuer == oldSelf.issuer",message="spec.issuer is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.snowflakeUserMappingAttribute == oldSelf.snowflakeUserMappingAttribute",message="spec.snowflakeUserMappingAttribute is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type ExternalOAuthIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// ExternalOAuthType is the external OAuth provider type. Immutable after creation.
	// +kubebuilder:validation:Enum=CUSTOM;AZURE;OKTA;PING_FEDERATE
	ExternalOAuthType string `json:"externalOAuthType"`

	// Issuer is the OAuth 2.0 token issuer URL. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// TokenUserMappingClaim is the claim in the access token that identifies the user.
	// +kubebuilder:validation:MinLength=1
	TokenUserMappingClaim string `json:"tokenUserMappingClaim" snowflake:"EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM,always,nounset"`

	// SnowflakeUserMappingAttribute specifies how to map the claim to a Snowflake user.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=LOGIN_NAME;EMAIL_ADDRESS
	// +kubebuilder:default=LOGIN_NAME
	SnowflakeUserMappingAttribute string `json:"snowflakeUserMappingAttribute"`

	// JWSKeysURL is the endpoint to retrieve the JWKS for token validation.
	// +optional
	JWSKeysURL *string `json:"jwsKeysURL,omitempty" snowflake:"EXTERNAL_OAUTH_JWS_KEYS_URL"`

	// AudienceList is the list of allowed audience values in the access token.
	// +optional
	AudienceList []string `json:"audienceList,omitempty" snowflake:"EXTERNAL_OAUTH_AUDIENCE_LIST"`

	// AllowedRoles is the list of Snowflake roles that can be assumed via this integration.
	// +optional
	AllowedRoles []string `json:"allowedRoles,omitempty" snowflake:"EXTERNAL_OAUTH_ALLOWED_ROLES_LIST"`

	// BlockedRoles is the list of Snowflake roles that cannot be assumed via this integration.
	// +optional
	BlockedRoles []string `json:"blockedRoles,omitempty" snowflake:"EXTERNAL_OAUTH_BLOCKED_ROLES_LIST"`

	// AnyRoleMode controls whether the ANY role is allowed.
	// +optional
	// +kubebuilder:validation:Enum=DISABLE;ENABLE;ENABLE_FOR_PRIVILEGE
	AnyRoleMode *string `json:"anyRoleMode,omitempty" snowflake:"EXTERNAL_OAUTH_ANY_ROLE_MODE"`

	// ScopeDelimiter is the character used to separate scopes in the OAuth token.
	// +optional
	ScopeDelimiter *string `json:"scopeDelimiter,omitempty" snowflake:"EXTERNAL_OAUTH_SCOPE_DELIMITER"`

	// NetworkPolicy is the optional network policy attached to the integration.
	// +optional
	NetworkPolicy *string `json:"networkPolicy,omitempty" snowflake:"NETWORK_POLICY"`

	// Comment is an optional comment for the integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ExternalOAuthIntegrationShowOutput holds the values from SHOW SECURITY INTEGRATIONS.
type ExternalOAuthIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`
	// Name is the Snowflake integration name.
	Name string `json:"name,omitempty"`
	// Type is the integration sub-type (e.g. EXTERNAL_OAUTH).
	Type string `json:"type,omitempty"`
	// Category is the integration category (SECURITY).
	Category string `json:"category,omitempty"`
	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty"`
	// Comment is the integration comment.
	Comment string `json:"comment,omitempty"`
}

// ExternalOAuthIntegrationStatus defines the observed state of ExternalOAuthIntegration.
type ExternalOAuthIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the most recently observed SHOW SECURITY INTEGRATIONS output.
	// +optional
	ShowOutput *ExternalOAuthIntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the most recently observed DESCRIBE INTEGRATION output.
	// +optional
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters records which Snowflake parameters were last applied by the controller.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=eoai
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=`.status.conditions[?(@.type=='Synced')].status`
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".status.fullyQualifiedName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ExternalOAuthIntegration is the Schema for the externaloauthintegrations API.
type ExternalOAuthIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalOAuthIntegrationSpec   `json:"spec,omitempty"`
	Status ExternalOAuthIntegrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalOAuthIntegrationList contains a list of ExternalOAuthIntegration.
type ExternalOAuthIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalOAuthIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalOAuthIntegration{}, &ExternalOAuthIntegrationList{})
}
