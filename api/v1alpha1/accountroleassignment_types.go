package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountRoleAssignmentSpec defines the desired state of an AccountRoleAssignment.
// Assigns an account role to another role or user:
//
//	GRANT ROLE <role> TO ROLE <parent_role>
//	GRANT ROLE <role> TO USER <user>
//
// All fields are immutable after creation — changing any field requires
// deleting and recreating the resource.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.roleName) == has(self.roleName) && (!has(self.roleName) || self.roleName == oldSelf.roleName)",message="spec.roleName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.roleRef) == has(self.roleRef) && (!has(self.roleRef) || self.roleRef == oldSelf.roleRef)",message="spec.roleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toRole) == has(self.toRole) && (!has(self.toRole) || self.toRole == oldSelf.toRole)",message="spec.toRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toRoleRef) == has(self.toRoleRef) && (!has(self.toRoleRef) || self.toRoleRef == oldSelf.toRoleRef)",message="spec.toRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toUser) == has(self.toUser) && (!has(self.toUser) || self.toUser == oldSelf.toUser)",message="spec.toUser is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.toUserRef) == has(self.toUserRef) && (!has(self.toUserRef) || self.toUserRef == oldSelf.toUserRef)",message="spec.toUserRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.roleName) && !has(self.roleRef)) || (!has(self.roleName) && has(self.roleRef))",message="exactly one of spec.roleName or spec.roleRef must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.toRole) ? 1 : 0) + (has(self.toRoleRef) ? 1 : 0) + (has(self.toUser) ? 1 : 0) + (has(self.toUserRef) ? 1 : 0) == 1",message="exactly one of spec.toRole, spec.toRoleRef, spec.toUser, or spec.toUserRef must be set"
type AccountRoleAssignmentSpec struct {
	CommonSpec `json:",inline"`

	// RoleName is the name of the account role to assign.
	// Mutually exclusive with RoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	RoleName *string `json:"roleName,omitempty"`

	// RoleRef references an AccountRole CR in the same namespace.
	// When set, the role name is resolved from the CR's spec.name.
	// Mutually exclusive with RoleName.
	// +optional
	RoleRef *ObjectReference `json:"roleRef,omitempty"`

	// ToRole is the name of the parent account role to assign the role to.
	// Mutually exclusive with ToRoleRef, ToUser, ToUserRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ToRole *string `json:"toRole,omitempty"`

	// ToRoleRef references an AccountRole CR in the same namespace.
	// When set, the target role name is resolved from the CR's spec.name.
	// Mutually exclusive with ToRole, ToUser, ToUserRef.
	// +optional
	ToRoleRef *ObjectReference `json:"toRoleRef,omitempty"`

	// ToUser is the name of the user to assign the role to.
	// Mutually exclusive with ToUserRef, ToRole, ToRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ToUser *string `json:"toUser,omitempty"`

	// ToUserRef references a User CR in the same namespace.
	// When set, the user name is resolved from the CR's spec.name.
	// Mutually exclusive with ToUser, ToRole, ToRoleRef.
	// +optional
	ToUserRef *ObjectReference `json:"toUserRef,omitempty"`
}

// AccountRoleAssignmentStatus defines the observed state of an AccountRoleAssignment.
type AccountRoleAssignmentStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW GRANTS OF ROLE output entry for this assignment.
	ShowOutput *RoleAssignmentShowOutput `json:"showOutput,omitempty"`
}

// AccountRoleAssignment is the Schema for the accountroleassignments API.
// It assigns an account role to another role or user in Snowflake.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ara,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="ROLE",type=string,JSONPath=`.spec.roleName`,priority=0
// +kubebuilder:printcolumn:name="TO-ROLE",type=string,JSONPath=`.spec.toRole`,priority=0
// +kubebuilder:printcolumn:name="TO-USER",type=string,JSONPath=`.spec.toUser`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type AccountRoleAssignment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountRoleAssignmentSpec   `json:"spec,omitempty"`
	Status AccountRoleAssignmentStatus `json:"status,omitempty"`
}

// AccountRoleAssignmentList contains a list of AccountRoleAssignment.
// +kubebuilder:object:root=true
type AccountRoleAssignmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountRoleAssignment `json:"items"`
}

// GetSpecName returns a human-readable composite name for the role assignment.
func (r *AccountRoleAssignment) GetSpecName() string {
	var role string
	if r.Spec.RoleName != nil {
		role = *r.Spec.RoleName
	}

	if role == "" && r.Spec.RoleRef != nil {
		role = "(ref: " + r.Spec.RoleRef.Name + ")"
	}

	target := r.resolvedTarget()

	return fmt.Sprintf("ROLE %s -> %s", role, target)
}

// resolvedTarget returns a string describing who the role is assigned to.
func (r *AccountRoleAssignment) resolvedTarget() string {
	if r.Spec.ToRole != nil {
		return "ROLE " + *r.Spec.ToRole
	}

	if r.Spec.ToRoleRef != nil {
		return "ROLE (ref: " + r.Spec.ToRoleRef.Name + ")"
	}

	if r.Spec.ToUser != nil {
		return "USER " + *r.Spec.ToUser
	}

	if r.Spec.ToUserRef != nil {
		return "USER (ref: " + r.Spec.ToUserRef.Name + ")"
	}

	return "<unset>"
}

func init() {
	SchemeBuilder.Register(&AccountRoleAssignment{}, &AccountRoleAssignmentList{})
}
