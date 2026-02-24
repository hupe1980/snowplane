package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserType specifies the type of Snowflake user.
type UserType string

// Valid UserType values.
const (
	UserTypePerson        UserType = "PERSON"
	UserTypeService       UserType = "SERVICE"
	UserTypeLegacyService UserType = "LEGACY_SERVICE"
)

// UserSpec defines the desired state of a User.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.type) == has(self.type) && (!has(self.type) || self.type == oldSelf.type)",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type UserSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake user name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// LoginName is the name that the user enters to log into the system.
	// Defaults to the user Name if not set.
	LoginName *string `json:"loginName,omitempty"`

	// DisplayName is the user's display name in the Snowflake UI.
	DisplayName *string `json:"displayName,omitempty"`

	// Email is the user's email address.
	Email *string `json:"email,omitempty"`

	// FirstName is the user's first name.
	FirstName *string `json:"firstName,omitempty"`

	// LastName is the user's last name.
	LastName *string `json:"lastName,omitempty"`

	// Comment is an optional description for the user.
	Comment *string `json:"comment,omitempty"`

	// Password references a Kubernetes Secret containing the user's password.
	Password *SecretKeyReference `json:"password,omitempty"`

	// RSAPublicKey references a Kubernetes Secret containing the user's RSA public key
	// for key-pair authentication.
	RSAPublicKey *SecretKeyReference `json:"rsaPublicKey,omitempty"`

	// RSAPublicKey2 references a Kubernetes Secret containing the user's second RSA public key
	// for key rotation.
	RSAPublicKey2 *SecretKeyReference `json:"rsaPublicKey2,omitempty"`

	// Type specifies the user type: PERSON, SERVICE, or LEGACY_SERVICE.
	// Defaults to PERSON. Immutable after creation.
	// +kubebuilder:validation:Enum=PERSON;SERVICE;LEGACY_SERVICE
	// +kubebuilder:default=PERSON
	Type *UserType `json:"type,omitempty"`

	// DefaultRole is the default role assigned to the user on login.
	DefaultRole *string `json:"defaultRole,omitempty"`

	// DefaultSecondaryRoles specifies secondary roles activated on login.
	// Set to "ALL" to enable all granted roles as secondary.
	DefaultSecondaryRoles *string `json:"defaultSecondaryRoles,omitempty"`

	// DefaultWarehouse is the default virtual warehouse for the user.
	DefaultWarehouse *string `json:"defaultWarehouse,omitempty"`

	// DefaultNamespace is the default database.schema namespace for the user.
	DefaultNamespace *string `json:"defaultNamespace,omitempty"`

	// MustChangePassword forces a password change on next login.
	MustChangePassword *bool `json:"mustChangePassword,omitempty"`

	// Disabled controls whether the user is disabled.
	Disabled *bool `json:"disabled,omitempty"`
}

// UserShowOutput mirrors the SHOW USERS output stored in status.
type UserShowOutput struct {
	// CreatedOn is the timestamp when the user was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the user name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// LoginName is the login name.
	LoginName string `json:"loginName,omitempty"`

	// DisplayName is the display name.
	DisplayName string `json:"displayName,omitempty"`

	// Email is the email address.
	Email string `json:"email,omitempty"`

	// FirstName is the first name.
	FirstName string `json:"firstName,omitempty"`

	// LastName is the last name.
	LastName string `json:"lastName,omitempty"`

	// Comment is the user description.
	Comment string `json:"comment,omitempty"`

	// DefaultRole is the default role.
	DefaultRole string `json:"defaultRole,omitempty"`

	// DefaultSecondaryRoles is the default secondary roles setting.
	DefaultSecondaryRoles string `json:"defaultSecondaryRoles,omitempty"`

	// DefaultWarehouse is the default warehouse.
	DefaultWarehouse string `json:"defaultWarehouse,omitempty"`

	// DefaultNamespace is the default namespace.
	DefaultNamespace string `json:"defaultNamespace,omitempty"`

	// Owner is the role that owns the user.
	Owner string `json:"owner,omitempty"`

	// Disabled indicates whether the user is disabled.
	Disabled bool `json:"disabled,omitempty"`

	// MustChangePassword indicates whether the user must change password.
	MustChangePassword bool `json:"mustChangePassword,omitempty"`

	// HasRSAPublicKey indicates whether an RSA public key is set.
	HasRSAPublicKey bool `json:"hasRsaPublicKey,omitempty"`

	// Type is the user type.
	Type string `json:"type,omitempty"`
}

// UserDescribeOutput holds additional fields from DESCRIBE USER that
// are not available in SHOW USERS.
type UserDescribeOutput struct {
	// RSAPublicKeyFP is the fingerprint of the primary RSA public key.
	RSAPublicKeyFP string `json:"rsaPublicKeyFp,omitempty"`

	// RSAPublicKey2FP is the fingerprint of the secondary RSA public key.
	RSAPublicKey2FP string `json:"rsaPublicKey2Fp,omitempty"`
}

// UserStatus defines the observed state of a User.
type UserStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW USERS output for this user.
	ShowOutput *UserShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the DESCRIBE USER output.
	DescribeOutput *UserDescribeOutput `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`

	// LastAppliedPasswordHash is the HMAC-SHA256 hash of the last password
	// value applied to Snowflake, keyed by the resource UID. The reconciler
	// skips ALTER USER SET PASSWORD when the resolved password hash matches
	// this value, avoiding unnecessary Snowflake API traffic and audit log
	// noise. The plaintext password is never stored.
	LastAppliedPasswordHash string `json:"lastAppliedPasswordHash,omitempty"`

	// LastAppliedRSAPublicKeyHash is the HMAC-SHA256 hash of the last
	// RSA_PUBLIC_KEY value applied to Snowflake, keyed by the resource UID.
	// Skips ALTER USER SET RSA_PUBLIC_KEY when unchanged.
	LastAppliedRSAPublicKeyHash string `json:"lastAppliedRSAPublicKeyHash,omitempty"`

	// LastAppliedRSAPublicKey2Hash is the HMAC-SHA256 hash of the last
	// RSA_PUBLIC_KEY_2 value applied to Snowflake, keyed by the resource UID.
	// Skips ALTER USER SET RSA_PUBLIC_KEY_2 when unchanged.
	LastAppliedRSAPublicKey2Hash string `json:"lastAppliedRSAPublicKey2Hash,omitempty"`
}

// User is the Schema for the users API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserSpec   `json:"spec,omitempty"`
	Status UserStatus `json:"status,omitempty"`
}

// UserList contains a list of User.
// +kubebuilder:object:root=true
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []User `json:"items"`
}

// GetConditions returns the conditions of the User.
func (u *User) GetConditions() []metav1.Condition {
	return u.Status.Conditions
}

// SetConditions sets the conditions of the User.
func (u *User) SetConditions(conditions []metav1.Condition) {
	u.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (u *User) GetDeletionPolicy() DeletionPolicy {
	if u.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return u.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (u *User) GetFullyQualifiedName() string {
	return u.Status.FullyQualifiedName
}

func init() {
	SchemeBuilder.Register(&User{}, &UserList{})
}

// GetSpecName returns the Snowflake resource name from the spec.
func (u *User) GetSpecName() string { return u.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (u *User) GetProviderRef() ProviderReference { return u.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (u *User) GetUseRole() *string { return u.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (u *User) GetObservedGeneration() int64 { return u.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (u *User) SetObservedGeneration(v int64) { u.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (u *User) GetLastAppliedSpecHash() string { return u.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (u *User) SetLastAppliedSpecHash(v string) { u.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (u *User) GetTrackedParametersList() []string { return u.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (u *User) SetTrackedParametersList(v []string) { u.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (u *User) GetOwner() string {
	if u.Status.ShowOutput != nil {
		return u.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (u *User) ValidateSpec() error { return u.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (u *User) ComputeSpecHash() (string, error) { return ComputeSpecHash(u.Spec) }
