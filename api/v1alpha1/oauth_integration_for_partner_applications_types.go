package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OAuthIntegrationForPartnerApplicationsSpec defines the desired state of a Snowflake OAuth Integration for Partner Applications.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.oauthClient == oldSelf.oauthClient",message="spec.oauthClient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type OAuthIntegrationForPartnerApplicationsSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// OAuthClient specifies the partner application type. Immutable after creation.
	// +kubebuilder:validation:Enum=TABLEAU_DESKTOP;TABLEAU_SERVER;LOOKER
	OAuthClient string `json:"oauthClient" snowflake:"OAUTH_CLIENT,nounset"`

	// OAuthRedirectURI is the client redirect URI (required for some partner apps).
	// +optional
	OAuthRedirectURI *string `json:"oauthRedirectURI,omitempty" snowflake:"OAUTH_REDIRECT_URI"`

	// OAuthUseSecondaryRoles specifies secondary role behavior.
	// +optional
	// +kubebuilder:validation:Enum=IMPLICIT;NONE
	OAuthUseSecondaryRoles *string `json:"oauthUseSecondaryRoles,omitempty" snowflake:"OAUTH_USE_SECONDARY_ROLES"`

	// OAuthIssueRefreshTokens controls whether refresh tokens are issued.
	// +optional
	OAuthIssueRefreshTokens *bool `json:"oauthIssueRefreshTokens,omitempty" snowflake:"OAUTH_ISSUE_REFRESH_TOKENS"`

	// OAuthRefreshTokenValidity is the validity period for refresh tokens in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=0
	OAuthRefreshTokenValidity *int64 `json:"oauthRefreshTokenValidity,omitempty" snowflake:"OAUTH_REFRESH_TOKEN_VALIDITY"`

	// BlockedRolesList is the list of Snowflake roles that cannot be used with this integration.
	// +optional
	BlockedRolesList []string `json:"blockedRolesList,omitempty" snowflake:"BLOCKED_ROLES_LIST"`

	// Comment is an optional comment for the integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// OAuthIntegrationForPartnerApplicationsShowOutput holds the values from SHOW SECURITY INTEGRATIONS.
type OAuthIntegrationForPartnerApplicationsShowOutput struct {
	CreatedOn string `json:"createdOn,omitempty"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Category  string `json:"category,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// OAuthIntegrationForPartnerApplicationsStatus defines the observed state.
type OAuthIntegrationForPartnerApplicationsStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *OAuthIntegrationForPartnerApplicationsShowOutput `json:"showOutput,omitempty"`
	DescribeOutput    map[string]string                                 `json:"describeOutput,omitempty"`
	TrackedParameters []string                                          `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=oauthpa,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=`.status.conditions[?(@.type=='Synced')].status`
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".status.fullyQualifiedName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// OAuthIntegrationForPartnerApplications is the Schema for the Snowflake OAuth Integration for Partner Applications API.
type OAuthIntegrationForPartnerApplications struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OAuthIntegrationForPartnerApplicationsSpec   `json:"spec,omitempty"`
	Status            OAuthIntegrationForPartnerApplicationsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OAuthIntegrationForPartnerApplicationsList contains a list of OAuthIntegrationForPartnerApplications.
type OAuthIntegrationForPartnerApplicationsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuthIntegrationForPartnerApplications `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OAuthIntegrationForPartnerApplications{}, &OAuthIntegrationForPartnerApplicationsList{})
}
