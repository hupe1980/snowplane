package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShareGrantSpec defines the desired state of a ShareGrant.
// Share grants are simpler than role grants: they only support specific named
// objects (no ALL/FUTURE bulk grants) and do not support WITH GRANT OPTION.
//
// +kubebuilder:validation:XValidation:rule="self.privilege == oldSelf.privilege",message="spec.privilege is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.objectType == oldSelf.objectType",message="spec.objectType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.objectName == oldSelf.objectName",message="spec.objectName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.share == oldSelf.share",message="spec.share is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type ShareGrantSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant.
	// Shares only support a limited set: USAGE, SELECT, REFERENCE_USAGE, READ, EVOLVE SCHEMA.
	// +kubebuilder:validation:MinLength=1
	Privilege string `json:"privilege"`

	// ObjectType is the type of object the privilege is granted on
	// (e.g. DATABASE, SCHEMA, TABLE, VIEW).
	// +kubebuilder:validation:MinLength=1
	ObjectType string `json:"objectType"`

	// ObjectName is the fully qualified name of the object
	// (e.g. MY_DB, MY_DB.PUBLIC, MY_DB.PUBLIC.MY_TABLE).
	// +kubebuilder:validation:MinLength=1
	ObjectName string `json:"objectName"`

	// Share is the name of the share to grant the privilege to.
	// +kubebuilder:validation:MinLength=1
	Share string `json:"share"`
}

// ShareGrantStatus defines the observed state of a ShareGrant.
type ShareGrantStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// ShareGrant is the Schema for the sharegrants API.
// It grants a Snowflake privilege to a share. Shares have a restricted set
// of grantable privileges and do not support WITH GRANT OPTION.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type ShareGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShareGrantSpec   `json:"spec,omitempty"`
	Status ShareGrantStatus `json:"status,omitempty"`
}

// ShareGrantList contains a list of ShareGrant.
// +kubebuilder:object:root=true
type ShareGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ShareGrant `json:"items"`
}

// GetSpecName returns a human-readable composite name for the grant.
func (r *ShareGrant) GetSpecName() string {
	return fmt.Sprintf("%s ON %s %s -> SHARE %s",
		r.Spec.Privilege, r.Spec.ObjectType, r.Spec.ObjectName, r.Spec.Share)
}

func init() {
	SchemeBuilder.Register(&ShareGrant{}, &ShareGrantList{})
}
