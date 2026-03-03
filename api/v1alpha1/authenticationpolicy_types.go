package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthenticationPolicyMfaPolicy defines the MFA sub-policy for an authentication policy.
type AuthenticationPolicyMfaPolicy struct {
	// AllowedMethods is the list of MFA methods allowed (e.g. "TOTP").
	// +optional
	// +kubebuilder:validation:items:Enum=TOTP
	AllowedMethods []string `json:"allowedMethods,omitempty"`

	// EnforceMfaOnExternalAuthentication controls MFA for external auth.
	// Valid values: "OPTIONAL", "REQUIRED".
	// +optional
	// +kubebuilder:validation:Enum=OPTIONAL;REQUIRED
	EnforceMfaOnExternalAuthentication *string `json:"enforceMfaOnExternalAuthentication,omitempty"`
}

// AuthenticationPolicyPatPolicy defines the PAT (Programmatic Access Token) sub-policy.
type AuthenticationPolicyPatPolicy struct {
	// DefaultExpiryInDays is the default token expiry in days.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DefaultExpiryInDays *int32 `json:"defaultExpiryInDays,omitempty"`

	// MaxExpiryInDays is the maximum token expiry in days.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxExpiryInDays *int32 `json:"maxExpiryInDays,omitempty"`

	// NetworkPolicyEvaluation controls whether the network policy is evaluated
	// for PAT-based authentication. Valid values: "OPTIONAL", "REQUIRED".
	// +optional
	// +kubebuilder:validation:Enum=OPTIONAL;REQUIRED
	NetworkPolicyEvaluation *string `json:"networkPolicyEvaluation,omitempty"`

	// RequireRoleRestrictionForServiceUsers requires a role restriction for service users.
	// +optional
	RequireRoleRestrictionForServiceUsers *bool `json:"requireRoleRestrictionForServiceUsers,omitempty"`
}

// AuthenticationPolicyWorkloadIdentityPolicy defines the workload identity sub-policy.
type AuthenticationPolicyWorkloadIdentityPolicy struct {
	// AllowedProviders is the list of identity providers allowed (e.g. "AWS", "AZURE", "GCP").
	// +optional
	// +kubebuilder:validation:items:Enum=AWS;AZURE;GCP
	AllowedProviders []string `json:"allowedProviders,omitempty"`

	// AllowedAwsAccounts is the list of AWS account IDs allowed for workload identity.
	// +optional
	AllowedAwsAccounts []string `json:"allowedAwsAccounts,omitempty"`

	// AllowedAzureIssuers is the list of Azure issuers allowed for workload identity.
	// +optional
	AllowedAzureIssuers []string `json:"allowedAzureIssuers,omitempty"`

	// AllowedOidcIssuers is the list of OIDC issuers allowed for workload identity.
	// +optional
	AllowedOidcIssuers []string `json:"allowedOidcIssuers,omitempty"`
}

// AuthenticationPolicySpec defines the desired state of a Snowflake Authentication Policy.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type AuthenticationPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake authentication policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a managed Database resource for the parent database.
	// Mutually exclusive with DatabaseName.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a managed Schema resource for the parent schema.
	// Mutually exclusive with SchemaName.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable"
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// The controller constructs the FQN from databaseName + schemaName + name.
	// Mutually exclusive with SchemaRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable"
	SchemaName *string `json:"schemaName,omitempty"`

	// AuthenticationMethods is the list of authentication methods allowed.
	// Valid values include: ALL, SAML, PASSWORD, OAUTH, KEYPAIR, PROGRAMMATIC_ACCESS_TOKEN, WORKLOAD_IDENTITY.
	// +optional
	// +kubebuilder:validation:items:Enum=ALL;SAML;PASSWORD;OAUTH;KEYPAIR;PROGRAMMATIC_ACCESS_TOKEN;WORKLOAD_IDENTITY
	AuthenticationMethods []string `json:"authenticationMethods,omitempty" snowflake:"AUTHENTICATION_METHODS"`

	// ClientTypes is the list of client types allowed.
	// Valid values include: ALL, SNOWFLAKE_UI, DRIVERS, SNOWFLAKE_CLI, SNOWSQL.
	// +optional
	// +kubebuilder:validation:items:Enum=ALL;SNOWFLAKE_UI;DRIVERS;SNOWFLAKE_CLI;SNOWSQL
	ClientTypes []string `json:"clientTypes,omitempty" snowflake:"CLIENT_TYPES"`

	// SecurityIntegrations is the list of security integration names
	// that can be used with this authentication policy.
	// +optional
	SecurityIntegrations []string `json:"securityIntegrations,omitempty" snowflake:"SECURITY_INTEGRATIONS"`

	// MfaEnrollment controls MFA enrollment behavior.
	// Valid values: REQUIRED, REQUIRED_PASSWORD_ONLY, OPTIONAL.
	// +optional
	// +kubebuilder:validation:Enum=REQUIRED;REQUIRED_PASSWORD_ONLY;OPTIONAL
	MfaEnrollment *string `json:"mfaEnrollment,omitempty" snowflake:"MFA_ENROLLMENT"`

	// MfaPolicy configures multi-factor authentication sub-policy.
	// +optional
	MfaPolicy *AuthenticationPolicyMfaPolicy `json:"mfaPolicy,omitempty" snowflake:"MFA_POLICY"`

	// PatPolicy configures programmatic access token sub-policy.
	// +optional
	PatPolicy *AuthenticationPolicyPatPolicy `json:"patPolicy,omitempty" snowflake:"PAT_POLICY"`

	// WorkloadIdentityPolicy configures the workload identity sub-policy.
	// +optional
	WorkloadIdentityPolicy *AuthenticationPolicyWorkloadIdentityPolicy `json:"workloadIdentityPolicy,omitempty" snowflake:"WORKLOAD_IDENTITY_POLICY"`

	// Comment is an optional description for the authentication policy.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// AuthenticationPolicyShowOutput mirrors the SHOW AUTHENTICATION POLICIES output stored in status.
type AuthenticationPolicyShowOutput struct {
	// CreatedOn is the timestamp when the policy was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the policy name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the database containing the policy.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the schema containing the policy.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the policy.
	Owner string `json:"owner,omitempty"`

	// Comment is the policy description.
	Comment string `json:"comment,omitempty"`
}

// AuthenticationPolicyStatus defines the observed state of an AuthenticationPolicy.
type AuthenticationPolicyStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved database fully-qualified name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema fully-qualified name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW AUTHENTICATION POLICIES output.
	ShowOutput *AuthenticationPolicyShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the DESCRIBE AUTHENTICATION POLICY key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// AuthenticationPolicy is the Schema for the authenticationpolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type AuthenticationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuthenticationPolicySpec   `json:"spec,omitempty"`
	Status AuthenticationPolicyStatus `json:"status,omitempty"`
}

// AuthenticationPolicyList contains a list of AuthenticationPolicy.
// +kubebuilder:object:root=true
type AuthenticationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuthenticationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthenticationPolicy{}, &AuthenticationPolicyList{})
}
