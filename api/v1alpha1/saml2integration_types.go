package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SAML2IntegrationSpec defines the desired state of a Snowflake SAML2 Security Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.issuer == oldSelf.issuer",message="spec.issuer is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.ssoURL == oldSelf.ssoURL",message="spec.ssoURL is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.provider == oldSelf.provider",message="spec.provider is immutable (delete and recreate the resource to change)"
type SAML2IntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake security integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Issuer is the SAML2 IdP entity ID / issuer URL.
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// SSOURL is the IdP Single Sign-On endpoint URL.
	// +kubebuilder:validation:MinLength=1
	SSOURL string `json:"ssoURL"`

	// Provider is the SAML IdP provider name (e.g. CUSTOM, OKTA, ADFS).
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// X509Cert is the Base64-encoded X.509 certificate from the IdP for SAML signature validation.
	// +kubebuilder:validation:MinLength=1
	X509Cert string `json:"x509Cert" snowflake:"SAML2_X509_CERT,always,nounset"`

	// AllowedEmailPatterns is a list of email address patterns allowed for SAML auto-provisioning.
	// +optional
	AllowedEmailPatterns []string `json:"allowedEmailPatterns,omitempty" snowflake:"ALLOWED_EMAIL_PATTERNS"`

	// AllowedUserDomains is a list of email domains allowed for SAML authentication.
	// +optional
	AllowedUserDomains []string `json:"allowedUserDomains,omitempty" snowflake:"ALLOWED_USER_DOMAINS"`

	// SPInitiatedLoginPageLabel is the label for SP-initiated login on the Snowflake login page.
	// +optional
	SPInitiatedLoginPageLabel *string `json:"spInitiatedLoginPageLabel,omitempty" snowflake:"SAML2_SP_INITIATED_LOGIN_PAGE_LABEL"`

	// EnableSPInitiated controls whether Service Provider (SP) initiated SAML login is enabled.
	// +optional
	EnableSPInitiated *bool `json:"enableSPInitiated,omitempty" snowflake:"SAML2_ENABLE_SP_INITIATED"`

	// ForceAuthn controls whether the IdP should force re-authentication on every login.
	// +optional
	ForceAuthn *bool `json:"forceAuthn,omitempty" snowflake:"SAML2_FORCE_AUTHN"`

	// RequestedNameIDFormat specifies the NameID format to request from the IdP.
	// +optional
	// +kubebuilder:validation:Enum="urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified";"urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress";"urn:oasis:names:tc:SAML:2.0:nameid-format:persistent";"urn:oasis:names:tc:SAML:2.0:nameid-format:transient"
	RequestedNameIDFormat *string `json:"requestedNameIDFormat,omitempty" snowflake:"SAML2_REQUESTED_NAMEID_FORMAT"`

	// PostLogoutRedirectURL is the URL to redirect the user to after SAML logout.
	// +optional
	PostLogoutRedirectURL *string `json:"postLogoutRedirectURL,omitempty" snowflake:"SAML2_POST_LOGOUT_REDIRECT_URL"`

	// Comment is an optional description for the integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SAML2IntegrationShowOutput mirrors the SHOW SECURITY INTEGRATIONS output for a SAML2 integration.
type SAML2IntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the integration type (SAML2).
	Type string `json:"type,omitempty"`

	// Category is the integration category (SECURITY).
	Category string `json:"category,omitempty"`

	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Comment is the integration description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SAML2IntegrationStatus defines the observed state of a SAML2Integration.
type SAML2IntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW SECURITY INTEGRATIONS output.
	ShowOutput *SAML2IntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput is a map of DESCRIBE INTEGRATION key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// SAML2Integration is the Schema for the saml2integrations API.
// It manages a Snowflake SAML2 security integration with dedicated, type-safe fields.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=saml2
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type SAML2Integration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SAML2IntegrationSpec   `json:"spec,omitempty"`
	Status SAML2IntegrationStatus `json:"status,omitempty"`
}

// SAML2IntegrationList contains a list of SAML2Integration.
// +kubebuilder:object:root=true
type SAML2IntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SAML2Integration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SAML2Integration{}, &SAML2IntegrationList{})
}
