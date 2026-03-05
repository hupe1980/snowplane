package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GrantPrivilegesToDatabaseRoleSpec defines the desired state of a GrantPrivilegesToDatabaseRole.
// All grant fields are immutable after creation — changing any field requires
// a REVOKE + re-GRANT (delete and recreate the resource to change).
//
// Database roles can only grant on: the parent DATABASE, schemas, or schema
// objects. Unlike account role grants, they cannot grant on ACCOUNT or on
// account-level objects (WAREHOUSE, USER, etc.).
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.privilege) == has(self.privilege) && (!has(self.privilege) || self.privilege == oldSelf.privilege)",message="spec.privilege is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.allPrivileges) == has(self.allPrivileges) && (!has(self.allPrivileges) || self.allPrivileges == oldSelf.allPrivileges)",message="spec.allPrivileges is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.on == oldSelf.on",message="spec.on is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRole) == has(self.databaseRole) && (!has(self.databaseRole) || self.databaseRole == oldSelf.databaseRole)",message="spec.databaseRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRoleRef) == has(self.databaseRoleRef) && (!has(self.databaseRoleRef) || self.databaseRoleRef == oldSelf.databaseRoleRef)",message="spec.databaseRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.withGrantOption == oldSelf.withGrantOption",message="spec.withGrantOption is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRole) && !has(self.databaseRoleRef)) || (!has(self.databaseRole) && has(self.databaseRoleRef))",message="exactly one of spec.databaseRole or spec.databaseRoleRef must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.privilege) && size(self.privilege) > 0 ? 1 : 0) + (has(self.allPrivileges) && self.allPrivileges ? 1 : 0) == 1",message="exactly one of spec.privilege or spec.allPrivileges must be set"
type GrantPrivilegesToDatabaseRoleSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant (e.g. USAGE, SELECT, CREATE TABLE).
	// Mutually exclusive with AllPrivileges.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Privilege string `json:"privilege,omitempty"`

	// AllPrivileges grants all privileges on the target.
	// Translates to GRANT ALL PRIVILEGES ON ... TO DATABASE ROLE ...
	// Mutually exclusive with Privilege.
	// +optional
	AllPrivileges bool `json:"allPrivileges,omitempty"`

	// On defines what the privilege is granted on.
	// Database roles can grant on the parent DATABASE, on schemas, or on schema objects.
	// Exactly one variant must be set.
	On GrantPrivilegesToDatabaseRoleOn `json:"on"`

	// DatabaseRole is the fully qualified database role name (e.g. MY_DB.MY_ROLE).
	// Mutually exclusive with DatabaseRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseRole *string `json:"databaseRole,omitempty"`

	// DatabaseRoleRef references a DatabaseRole CR in the same namespace.
	// When set, the database role name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with DatabaseRole.
	// +optional
	DatabaseRoleRef *ObjectReference `json:"databaseRoleRef,omitempty"`

	// WithGrantOption allows the grantee to re-grant the privilege to other roles.
	// +optional
	// +kubebuilder:default=false
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// GrantPrivilegesToDatabaseRoleStatus defines the observed state of a GrantPrivilegesToDatabaseRole.
type GrantPrivilegesToDatabaseRoleStatus struct {
	CommonStatus `json:",inline"`

	// Kind indicates the resolved grant kind (Regular, Future, All).
	Kind GrantKind `json:"kind,omitempty"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// GrantPrivilegesToDatabaseRole is the Schema for the grantprivilegestodatabaseroles API.
// It grants a Snowflake privilege to a database role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="PRIVILEGE",type=string,JSONPath=`.spec.privilege`,priority=0
// +kubebuilder:printcolumn:name="ALL_PRIV",type=boolean,JSONPath=`.spec.allPrivileges`,priority=0
// +kubebuilder:printcolumn:name="ROLE",type=string,JSONPath=`.spec.databaseRole`,priority=0
// +kubebuilder:printcolumn:name="KIND",type=string,JSONPath=`.status.kind`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type GrantPrivilegesToDatabaseRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrantPrivilegesToDatabaseRoleSpec   `json:"spec,omitempty"`
	Status GrantPrivilegesToDatabaseRoleStatus `json:"status,omitempty"`
}

// GrantPrivilegesToDatabaseRoleList contains a list of GrantPrivilegesToDatabaseRole.
// +kubebuilder:object:root=true
type GrantPrivilegesToDatabaseRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrantPrivilegesToDatabaseRole `json:"items"`
}

// GetSpecName returns a human-readable composite name for the grant.
func (r *GrantPrivilegesToDatabaseRole) GetSpecName() string {
	var role string
	if r.Spec.DatabaseRole != nil {
		role = *r.Spec.DatabaseRole
	}

	if role == "" && r.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + r.Spec.DatabaseRoleRef.Name + ")"
	}

	return fmt.Sprintf("%s %s -> DATABASE ROLE %s", r.Spec.ResolvedPrivilege(), r.Spec.On.Description(), role)
}

// ResolvedDatabaseRole returns the resolved database role name (either direct or from ref).
func (r *GrantPrivilegesToDatabaseRole) ResolvedDatabaseRole() string {
	if r.Spec.DatabaseRole != nil {
		return *r.Spec.DatabaseRole
	}

	return ""
}

// ResolvedPrivilege returns the effective privilege string.
// Returns "ALL PRIVILEGES" when allPrivileges is true.
func (s *GrantPrivilegesToDatabaseRoleSpec) ResolvedPrivilege() string {
	if s.AllPrivileges {
		return "ALL PRIVILEGES"
	}

	return s.Privilege
}

// ResolveKind determines the GrantKind from the spec.
func (s *GrantPrivilegesToDatabaseRoleSpec) ResolveKind() GrantKind {
	return s.On.ResolveKind()
}

func init() {
	SchemeBuilder.Register(&GrantPrivilegesToDatabaseRole{}, &GrantPrivilegesToDatabaseRoleList{})
}
