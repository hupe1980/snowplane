package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GrantPrivilegesToShareSpec defines the desired state of a GrantPrivilegesToShare.
// Share grants are simpler than role grants: they only support specific named
// objects (no ALL/FUTURE bulk grants except allTablesInSchema) and do not
// support WITH GRANT OPTION.
//
// Modelled after the Terraform provider's snowflake_grant_privileges_to_share
// resource with flat on-target fields for explicit, type-safe declarations.
//
// +kubebuilder:validation:XValidation:rule="self.privilege == oldSelf.privilege",message="spec.privilege is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.on == oldSelf.on",message="spec.on is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.share == oldSelf.share",message="spec.share is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type GrantPrivilegesToShareSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant.
	// Shares only support a limited set: USAGE, SELECT, REFERENCE_USAGE, READ, EVOLVE SCHEMA.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Privilege string `json:"privilege"`

	// On defines what the privilege is granted on.
	// Exactly one field must be set.
	On GrantPrivilegesToShareOn `json:"on"`

	// Share is the name of the share to grant the privilege to.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Share string `json:"share"`
}

// GrantPrivilegesToShareStatus defines the observed state of a GrantPrivilegesToShare.
type GrantPrivilegesToShareStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// GrantPrivilegesToShare is the Schema for the grantprivilegestoshares API.
// It grants a Snowflake privilege to a share. Shares have a restricted set
// of grantable privileges and do not support WITH GRANT OPTION.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="PRIVILEGE",type=string,JSONPath=`.spec.privilege`,priority=0
// +kubebuilder:printcolumn:name="SHARE",type=string,JSONPath=`.spec.share`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type GrantPrivilegesToShare struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrantPrivilegesToShareSpec   `json:"spec,omitempty"`
	Status GrantPrivilegesToShareStatus `json:"status,omitempty"`
}

// GrantPrivilegesToShareList contains a list of GrantPrivilegesToShare.
// +kubebuilder:object:root=true
type GrantPrivilegesToShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrantPrivilegesToShare `json:"items"`
}

// GetSpecName returns a human-readable composite name for the grant.
func (r *GrantPrivilegesToShare) GetSpecName() string {
	return fmt.Sprintf("%s %s -> SHARE %s",
		r.Spec.Privilege, r.Spec.On.Description(), r.Spec.Share)
}

func init() {
	SchemeBuilder.Register(&GrantPrivilegesToShare{}, &GrantPrivilegesToShareList{})
}
