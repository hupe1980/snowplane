package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PasswordPolicySpec defines the desired state of a Snowflake Password Policy.
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
type PasswordPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake password policy name. Immutable after creation.
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

	// PasswordMinLength is the minimum number of characters the password must contain.
	// +optional
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=256
	PasswordMinLength *int32 `json:"passwordMinLength,omitempty" snowflake:"PASSWORD_MIN_LENGTH"`

	// PasswordMaxLength is the maximum number of characters the password can contain.
	// +optional
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=256
	PasswordMaxLength *int32 `json:"passwordMaxLength,omitempty" snowflake:"PASSWORD_MAX_LENGTH"`

	// PasswordMinUpperCaseChars is the minimum number of uppercase characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinUpperCaseChars *int32 `json:"passwordMinUpperCaseChars,omitempty" snowflake:"PASSWORD_MIN_UPPER_CASE_CHARS"`

	// PasswordMinLowerCaseChars is the minimum number of lowercase characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinLowerCaseChars *int32 `json:"passwordMinLowerCaseChars,omitempty" snowflake:"PASSWORD_MIN_LOWER_CASE_CHARS"`

	// PasswordMinNumericChars is the minimum number of numeric characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinNumericChars *int32 `json:"passwordMinNumericChars,omitempty" snowflake:"PASSWORD_MIN_NUMERIC_CHARS"`

	// PasswordMinSpecialChars is the minimum number of special characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinSpecialChars *int32 `json:"passwordMinSpecialChars,omitempty" snowflake:"PASSWORD_MIN_SPECIAL_CHARS"`

	// PasswordMinAgeDays is the minimum number of days before a password can be changed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	PasswordMinAgeDays *int32 `json:"passwordMinAgeDays,omitempty" snowflake:"PASSWORD_MIN_AGE_DAYS"`

	// PasswordMaxAgeDays is the maximum number of days a password is valid (0 = no expiry).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	PasswordMaxAgeDays *int32 `json:"passwordMaxAgeDays,omitempty" snowflake:"PASSWORD_MAX_AGE_DAYS"`

	// PasswordMaxRetries is the maximum number of failed login attempts before lockout.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	PasswordMaxRetries *int32 `json:"passwordMaxRetries,omitempty" snowflake:"PASSWORD_MAX_RETRIES"`

	// PasswordLockoutTimeMins is the lockout duration in minutes after exceeding max retries.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=999
	PasswordLockoutTimeMins *int32 `json:"passwordLockoutTimeMins,omitempty" snowflake:"PASSWORD_LOCKOUT_TIME_MINS"`

	// PasswordHistory is the number of recent passwords that cannot be reused.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=24
	PasswordHistory *int32 `json:"passwordHistory,omitempty" snowflake:"PASSWORD_HISTORY"`

	// Comment is an optional description for the password policy.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// PasswordPolicyShowOutput mirrors the SHOW PASSWORD POLICIES output stored in status.
type PasswordPolicyShowOutput struct {
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
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// PasswordPolicyStatus defines the observed state of a PasswordPolicy.
type PasswordPolicyStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved database fully-qualified name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema fully-qualified name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW PASSWORD POLICIES output.
	ShowOutput *PasswordPolicyShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the DESCRIBE PASSWORD POLICY key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// PasswordPolicy is the Schema for the passwordpolicies API.
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
type PasswordPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PasswordPolicySpec   `json:"spec,omitempty"`
	Status PasswordPolicyStatus `json:"status,omitempty"`
}

// PasswordPolicyList contains a list of PasswordPolicy.
// +kubebuilder:object:root=true
type PasswordPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PasswordPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PasswordPolicy{}, &PasswordPolicyList{})
}
