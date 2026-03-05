package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GrantPrivilegesToAccountRoleSpec defines the desired state of an GrantPrivilegesToAccountRole.
// All grant fields are immutable after creation — changing any field requires
// a REVOKE + re-GRANT (delete and recreate the resource to change).
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.privilege) == has(self.privilege) && (!has(self.privilege) || self.privilege == oldSelf.privilege)",message="spec.privilege is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.allPrivileges) == has(self.allPrivileges) && (!has(self.allPrivileges) || self.allPrivileges == oldSelf.allPrivileges)",message="spec.allPrivileges is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.on == oldSelf.on",message="spec.on is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.accountRole) == has(self.accountRole) && (!has(self.accountRole) || self.accountRole == oldSelf.accountRole)",message="spec.accountRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.accountRoleRef) == has(self.accountRoleRef) && (!has(self.accountRoleRef) || self.accountRoleRef == oldSelf.accountRoleRef)",message="spec.accountRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.withGrantOption == oldSelf.withGrantOption",message="spec.withGrantOption is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.accountRole) && !has(self.accountRoleRef)) || (!has(self.accountRole) && has(self.accountRoleRef))",message="exactly one of spec.accountRole or spec.accountRoleRef must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.privilege) && size(self.privilege) > 0 ? 1 : 0) + (has(self.allPrivileges) && self.allPrivileges ? 1 : 0) == 1",message="exactly one of spec.privilege or spec.allPrivileges must be set"
type GrantPrivilegesToAccountRoleSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant (e.g. USAGE, SELECT, CREATE SCHEMA).
	// Mutually exclusive with AllPrivileges.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Privilege string `json:"privilege,omitempty"`

	// AllPrivileges grants all privileges on the target.
	// Translates to GRANT ALL PRIVILEGES ON ... TO ROLE ...
	// Mutually exclusive with Privilege.
	// +optional
	AllPrivileges bool `json:"allPrivileges,omitempty"`

	// On defines what the privilege is granted on.
	// Exactly one variant must be set.
	On GrantOn `json:"on"`

	// AccountRole is the name of the account role to grant the privilege to.
	// Mutually exclusive with AccountRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	AccountRole *string `json:"accountRole,omitempty"`

	// AccountRoleRef references an AccountRole CR in the same namespace.
	// When set, the role name is resolved from the CR's spec.name.
	// Mutually exclusive with AccountRole.
	// +optional
	AccountRoleRef *ObjectReference `json:"accountRoleRef,omitempty"`

	// WithGrantOption allows the grantee to re-grant the privilege to other roles.
	//
	// SECURITY NOTE: When set to true, the grantee can re-grant this privilege
	// to other roles, creating delegation chains. Revoking this CR does
	// NOT cascade-revoke grants made by the grantee. Use with caution.
	// +optional
	// +kubebuilder:default=false
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// GrantPrivilegesToAccountRoleStatus defines the observed state of an GrantPrivilegesToAccountRole.
type GrantPrivilegesToAccountRoleStatus struct {
	CommonStatus `json:",inline"`

	// Kind indicates the resolved grant kind (Regular, Future, All).
	Kind GrantKind `json:"kind,omitempty"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// GrantPrivilegesToAccountRole is the Schema for the grantprivilegestoaccountroles API.
// It grants a Snowflake privilege to an account role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="PRIVILEGE",type=string,JSONPath=`.spec.privilege`,priority=0
// +kubebuilder:printcolumn:name="ALL_PRIV",type=boolean,JSONPath=`.spec.allPrivileges`,priority=0
// +kubebuilder:printcolumn:name="ROLE",type=string,JSONPath=`.spec.accountRole`,priority=0
// +kubebuilder:printcolumn:name="KIND",type=string,JSONPath=`.status.kind`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type GrantPrivilegesToAccountRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrantPrivilegesToAccountRoleSpec   `json:"spec,omitempty"`
	Status GrantPrivilegesToAccountRoleStatus `json:"status,omitempty"`
}

// GrantPrivilegesToAccountRoleList contains a list of GrantPrivilegesToAccountRole.
// +kubebuilder:object:root=true
type GrantPrivilegesToAccountRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrantPrivilegesToAccountRole `json:"items"`
}

// GetSpecName returns a human-readable composite name for the grant.
func (r *GrantPrivilegesToAccountRole) GetSpecName() string {
	var role string
	if r.Spec.AccountRole != nil {
		role = *r.Spec.AccountRole
	}

	if role == "" && r.Spec.AccountRoleRef != nil {
		role = "(ref: " + r.Spec.AccountRoleRef.Name + ")"
	}

	return fmt.Sprintf("%s %s -> ROLE %s", r.Spec.ResolvedPrivilege(), r.Spec.On.Description(), role)
}

// ResolvedAccountRole returns the resolved account role name (either direct or from ref).
func (r *GrantPrivilegesToAccountRole) ResolvedAccountRole() string {
	if r.Spec.AccountRole != nil {
		return *r.Spec.AccountRole
	}

	return ""
}

// ResolvedPrivilege returns the effective privilege string.
// Returns "ALL PRIVILEGES" when allPrivileges is true.
func (s *GrantPrivilegesToAccountRoleSpec) ResolvedPrivilege() string {
	if s.AllPrivileges {
		return "ALL PRIVILEGES"
	}

	return s.Privilege
}

// ResolveKind determines the GrantKind from the spec.
func (s *GrantPrivilegesToAccountRoleSpec) ResolveKind() GrantKind {
	return resolveGrantKind(&s.On)
}

func init() {
	SchemeBuilder.Register(&GrantPrivilegesToAccountRole{}, &GrantPrivilegesToAccountRoleList{})
}
