package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountRoleSpec defines the desired state of an AccountRole.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type AccountRoleSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake role name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Comment is an optional description for the role.
	Comment *string `json:"comment,omitempty"`
}

// AccountRoleShowOutput mirrors the SHOW ROLES output stored in status.
type AccountRoleShowOutput struct {
	// CreatedOn is the timestamp when the role was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the role name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Comment is the role description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns this role.
	Owner string `json:"owner,omitempty"`

	// GrantedToRoles is the number of roles this role is granted to.
	GrantedToRoles int32 `json:"grantedToRoles,omitempty"`

	// GrantedRoles is the number of roles granted to this role.
	GrantedRoles int32 `json:"grantedRoles,omitempty"`
}

// AccountRoleStatus defines the observed state of an AccountRole.
type AccountRoleStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW ROLES output for this role.
	ShowOutput *AccountRoleShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// AccountRole is the Schema for the accountroles API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type AccountRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountRoleSpec   `json:"spec,omitempty"`
	Status AccountRoleStatus `json:"status,omitempty"`
}

// AccountRoleList contains a list of AccountRole.
// +kubebuilder:object:root=true
type AccountRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AccountRole{}, &AccountRoleList{})
}
