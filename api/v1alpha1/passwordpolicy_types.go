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
// +kubebuilder:validation:XValidation:rule="!has(self.databaseRef) || !has(oldSelf.databaseRef) || self.databaseRef == oldSelf.databaseRef",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !has(oldSelf.databaseName) || self.databaseName == oldSelf.databaseName",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaRef) || !has(oldSelf.schemaRef) || self.schemaRef == oldSelf.schemaRef",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !has(oldSelf.schemaName) || self.schemaName == oldSelf.schemaName",message="spec.schemaName is immutable (delete and recreate the resource to change)"
type PasswordPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake password policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a managed Database resource for the parent database.
	// Mutually exclusive with DatabaseName.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the literal Snowflake database name.
	// Mutually exclusive with DatabaseRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a managed Schema resource for the parent schema.
	// Mutually exclusive with SchemaName.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable"
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the literal Snowflake fully-qualified schema name (DB.SCHEMA).
	// Mutually exclusive with SchemaRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable"
	SchemaName *string `json:"schemaName,omitempty"`

	// PasswordMinLength is the minimum number of characters the password must contain.
	// +optional
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=256
	PasswordMinLength *int32 `json:"passwordMinLength,omitempty"`

	// PasswordMaxLength is the maximum number of characters the password can contain.
	// +optional
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=256
	PasswordMaxLength *int32 `json:"passwordMaxLength,omitempty"`

	// PasswordMinUpperCaseChars is the minimum number of uppercase characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinUpperCaseChars *int32 `json:"passwordMinUpperCaseChars,omitempty"`

	// PasswordMinLowerCaseChars is the minimum number of lowercase characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinLowerCaseChars *int32 `json:"passwordMinLowerCaseChars,omitempty"`

	// PasswordMinNumericChars is the minimum number of numeric characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinNumericChars *int32 `json:"passwordMinNumericChars,omitempty"`

	// PasswordMinSpecialChars is the minimum number of special characters.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=256
	PasswordMinSpecialChars *int32 `json:"passwordMinSpecialChars,omitempty"`

	// PasswordMinAgeDays is the minimum number of days before a password can be changed.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	PasswordMinAgeDays *int32 `json:"passwordMinAgeDays,omitempty"`

	// PasswordMaxAgeDays is the maximum number of days a password is valid (0 = no expiry).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=999
	PasswordMaxAgeDays *int32 `json:"passwordMaxAgeDays,omitempty"`

	// PasswordMaxRetries is the maximum number of failed login attempts before lockout.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	PasswordMaxRetries *int32 `json:"passwordMaxRetries,omitempty"`

	// PasswordLockoutTimeMins is the lockout duration in minutes after exceeding max retries.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=999
	PasswordLockoutTimeMins *int32 `json:"passwordLockoutTimeMins,omitempty"`

	// PasswordHistory is the number of recent passwords that cannot be reused.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=24
	PasswordHistory *int32 `json:"passwordHistory,omitempty"`

	// Comment is an optional description for the password policy.
	// +optional
	Comment *string `json:"comment,omitempty"`
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
	Comment string `json:"comment,omitempty"`
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
