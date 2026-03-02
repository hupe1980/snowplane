package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseRoleAssignmentSpec defines the desired state of a DatabaseRoleAssignment.
// Assigns a database role to an account role or another database role:
//
//	GRANT DATABASE ROLE <db>.<role> TO ROLE <parent_role>
//	GRANT DATABASE ROLE <db>.<role> TO DATABASE ROLE <db>.<other_role>
//
// All fields are immutable after creation — changing any field requires
// deleting and recreating the resource.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRoleName) == has(self.databaseRoleName) && (!has(self.databaseRoleName) || self.databaseRoleName == oldSelf.databaseRoleName)",message="spec.databaseRoleName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRoleRef) == has(self.databaseRoleRef) && (!has(self.databaseRoleRef) || self.databaseRoleRef == oldSelf.databaseRoleRef)",message="spec.databaseRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toRole) == has(self.toRole) && (!has(self.toRole) || self.toRole == oldSelf.toRole)",message="spec.toRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toRoleRef) == has(self.toRoleRef) && (!has(self.toRoleRef) || self.toRoleRef == oldSelf.toRoleRef)",message="spec.toRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toDatabaseRole) == has(self.toDatabaseRole) && (!has(self.toDatabaseRole) || self.toDatabaseRole == oldSelf.toDatabaseRole)",message="spec.toDatabaseRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toDatabaseRoleRef) == has(self.toDatabaseRoleRef) && (!has(self.toDatabaseRoleRef) || self.toDatabaseRoleRef == oldSelf.toDatabaseRoleRef)",message="spec.toDatabaseRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRoleName) && !has(self.databaseRoleRef)) || (!has(self.databaseRoleName) && has(self.databaseRoleRef))",message="exactly one of spec.databaseRoleName or spec.databaseRoleRef must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.toRole) ? 1 : 0) + (has(self.toRoleRef) ? 1 : 0) + (has(self.toDatabaseRole) ? 1 : 0) + (has(self.toDatabaseRoleRef) ? 1 : 0) == 1",message="exactly one of spec.toRole, spec.toRoleRef, spec.toDatabaseRole, or spec.toDatabaseRoleRef must be set"
type DatabaseRoleAssignmentSpec struct {
	CommonSpec `json:",inline"`

	// DatabaseRoleName is the fully qualified name of the database role to assign (e.g. MY_DB.MY_ROLE).
	// Mutually exclusive with DatabaseRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseRoleName *string `json:"databaseRoleName,omitempty"`

	// DatabaseRoleRef references a DatabaseRole CR in the same namespace.
	// When set, the database role name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with DatabaseRoleName.
	// +optional
	DatabaseRoleRef *LocalObjectReference `json:"databaseRoleRef,omitempty"`

	// ToRole is the name of the account role to assign the database role to.
	// Mutually exclusive with ToRoleRef, ToDatabaseRole, ToDatabaseRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ToRole *string `json:"toRole,omitempty"`

	// ToRoleRef references an AccountRole CR in the same namespace.
	// When set, the target role name is resolved from the CR's spec.name.
	// Mutually exclusive with ToRole, ToDatabaseRole, ToDatabaseRoleRef.
	// +optional
	ToRoleRef *LocalObjectReference `json:"toRoleRef,omitempty"`

	// ToDatabaseRole is the fully qualified name of the database role to assign to (e.g. MY_DB.MY_ROLE).
	// Mutually exclusive with ToDatabaseRoleRef, ToRole, ToRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ToDatabaseRole *string `json:"toDatabaseRole,omitempty"`

	// ToDatabaseRoleRef references a DatabaseRole CR in the same namespace.
	// When set, the target database role name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with ToDatabaseRole, ToRole, ToRoleRef.
	// +optional
	ToDatabaseRoleRef *LocalObjectReference `json:"toDatabaseRoleRef,omitempty"`
}

// DatabaseRoleAssignmentStatus defines the observed state of a DatabaseRoleAssignment.
type DatabaseRoleAssignmentStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW GRANTS OF DATABASE ROLE output entry for this assignment.
	ShowOutput *RoleAssignmentShowOutput `json:"showOutput,omitempty"`
}

// DatabaseRoleAssignment is the Schema for the databaseroleassignments API.
// It assigns a database role to an account role or another database role in Snowflake.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dra,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="DB-ROLE",type=string,JSONPath=`.spec.databaseRoleName`,priority=0
// +kubebuilder:printcolumn:name="TO-ROLE",type=string,JSONPath=`.spec.toRole`,priority=0
// +kubebuilder:printcolumn:name="TO-DB-ROLE",type=string,JSONPath=`.spec.toDatabaseRole`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type DatabaseRoleAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleAssignmentSpec   `json:"spec,omitempty"`
	Status DatabaseRoleAssignmentStatus `json:"status,omitempty"`
}

// DatabaseRoleAssignmentList contains a list of DatabaseRoleAssignment.
// +kubebuilder:object:root=true
type DatabaseRoleAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRoleAssignment `json:"items"`
}

// GetSpecName returns a human-readable composite name for the role assignment.
func (r *DatabaseRoleAssignment) GetSpecName() string {
	var role string
	if r.Spec.DatabaseRoleName != nil {
		role = *r.Spec.DatabaseRoleName
	}

	if role == "" && r.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + r.Spec.DatabaseRoleRef.Name + ")"
	}

	target := r.resolvedTarget()

	return fmt.Sprintf("DATABASE ROLE %s -> %s", role, target)
}

// resolvedTarget returns a string describing who the database role is assigned to.
func (r *DatabaseRoleAssignment) resolvedTarget() string {
	if r.Spec.ToRole != nil {
		return "ROLE " + *r.Spec.ToRole
	}

	if r.Spec.ToRoleRef != nil {
		return "ROLE (ref: " + r.Spec.ToRoleRef.Name + ")"
	}

	if r.Spec.ToDatabaseRole != nil {
		return "DATABASE ROLE " + *r.Spec.ToDatabaseRole
	}

	if r.Spec.ToDatabaseRoleRef != nil {
		return "DATABASE ROLE (ref: " + r.Spec.ToDatabaseRoleRef.Name + ")"
	}

	return "<unset>"
}

func init() {
	SchemeBuilder.Register(&DatabaseRoleAssignment{}, &DatabaseRoleAssignmentList{})
}
