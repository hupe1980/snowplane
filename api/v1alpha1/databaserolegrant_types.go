package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseRoleGrantSpec defines the desired state of a DatabaseRoleGrant.
// All grant fields are immutable after creation — changing any field requires
// a REVOKE + re-GRANT (delete and recreate the resource to change).
//
// +kubebuilder:validation:XValidation:rule="self.privilege == oldSelf.privilege",message="spec.privilege is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.on == oldSelf.on",message="spec.on is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRole) == has(self.databaseRole) && (!has(self.databaseRole) || self.databaseRole == oldSelf.databaseRole)",message="spec.databaseRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRoleRef) == has(self.databaseRoleRef) && (!has(self.databaseRoleRef) || self.databaseRoleRef == oldSelf.databaseRoleRef)",message="spec.databaseRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.withGrantOption == oldSelf.withGrantOption",message="spec.withGrantOption is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRole) && !has(self.databaseRoleRef)) || (!has(self.databaseRole) && has(self.databaseRoleRef))",message="exactly one of spec.databaseRole or spec.databaseRoleRef must be set"
type DatabaseRoleGrantSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant (e.g. USAGE, SELECT, CREATE TABLE).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Privilege string `json:"privilege"`

	// On defines what the privilege is granted on.
	// Exactly one variant must be set.
	On GrantOn `json:"on"`

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
	DatabaseRoleRef *LocalObjectReference `json:"databaseRoleRef,omitempty"`

	// WithGrantOption allows the grantee to re-grant the privilege to other roles.
	// +optional
	// +kubebuilder:default=false
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// DatabaseRoleGrantStatus defines the observed state of a DatabaseRoleGrant.
type DatabaseRoleGrantStatus struct {
	CommonStatus `json:",inline"`

	// Kind indicates the resolved grant kind (Regular, Future, All).
	Kind GrantKind `json:"kind,omitempty"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// DatabaseRoleGrant is the Schema for the databaserolegrants API.
// It grants a Snowflake privilege to a database role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="PRIVILEGE",type=string,JSONPath=`.spec.privilege`,priority=0
// +kubebuilder:printcolumn:name="ROLE",type=string,JSONPath=`.spec.databaseRole`,priority=0
// +kubebuilder:printcolumn:name="KIND",type=string,JSONPath=`.status.kind`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type DatabaseRoleGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleGrantSpec   `json:"spec,omitempty"`
	Status DatabaseRoleGrantStatus `json:"status,omitempty"`
}

// DatabaseRoleGrantList contains a list of DatabaseRoleGrant.
// +kubebuilder:object:root=true
type DatabaseRoleGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRoleGrant `json:"items"`
}

// GetSpecName returns a human-readable composite name for the grant.
func (r *DatabaseRoleGrant) GetSpecName() string {
	var role string
	if r.Spec.DatabaseRole != nil {
		role = *r.Spec.DatabaseRole
	}

	if role == "" && r.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + r.Spec.DatabaseRoleRef.Name + ")"
	}

	return fmt.Sprintf("%s %s -> DATABASE ROLE %s", r.Spec.Privilege, r.Spec.On.Description(), role)
}

// ResolvedDatabaseRole returns the resolved database role name (either direct or from ref).
func (r *DatabaseRoleGrant) ResolvedDatabaseRole() string {
	if r.Spec.DatabaseRole != nil {
		return *r.Spec.DatabaseRole
	}

	return ""
}

// ResolveKind determines the GrantKind from the spec.
func (s *DatabaseRoleGrantSpec) ResolveKind() GrantKind {
	return resolveGrantKind(&s.On)
}

func init() {
	SchemeBuilder.Register(&DatabaseRoleGrant{}, &DatabaseRoleGrantList{})
}
