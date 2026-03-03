package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicyAttachmentSpec defines the desired state of a NetworkPolicyAttachment.
// Attaches a Snowflake network policy to an account or user:
//
//	ALTER ACCOUNT SET NETWORK_POLICY = <policy_name>
//	ALTER USER <user_name> SET NETWORK_POLICY = <policy_name>
//
// Identity fields (policyName/policyRef, targetType, targetName) are
// immutable after creation — changing any of them requires deleting and
// recreating the resource.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.policyName) == has(self.policyName) && (!has(self.policyName) || self.policyName == oldSelf.policyName)",message="spec.policyName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.policyRef) == has(self.policyRef) && (!has(self.policyRef) || self.policyRef == oldSelf.policyRef)",message="spec.policyRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.targetType == oldSelf.targetType",message="spec.targetType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.targetName) == has(self.targetName) && (!has(self.targetName) || self.targetName == oldSelf.targetName)",message="spec.targetName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.policyName) && !has(self.policyRef)) || (!has(self.policyName) && has(self.policyRef))",message="exactly one of spec.policyName or spec.policyRef must be set"
// +kubebuilder:validation:XValidation:rule="self.targetType == 'ACCOUNT' || has(self.targetName)",message="spec.targetName is required when targetType is USER"
type NetworkPolicyAttachmentSpec struct {
	CommonSpec `json:",inline"`

	// PolicyName is the Snowflake network policy name.
	// Mutually exclusive with PolicyRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	PolicyName *string `json:"policyName,omitempty"`

	// PolicyRef references a NetworkPolicy CR in the same namespace.
	// When set, the policy name is resolved from the CR's status.
	// Mutually exclusive with PolicyName.
	// +optional
	PolicyRef *LocalObjectReference `json:"policyRef,omitempty"`

	// TargetType is the kind of Snowflake object to attach the policy to.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=ACCOUNT;USER
	TargetType string `json:"targetType"`

	// TargetName is the name of the Snowflake user to attach the policy to.
	// Required when targetType is USER. Must be omitted for ACCOUNT.
	// Immutable after creation.
	// +optional
	TargetName string `json:"targetName,omitempty"`
}

// NetworkPolicyAttachmentStatus defines the observed state of a NetworkPolicyAttachment.
type NetworkPolicyAttachmentStatus struct {
	CommonStatus `json:",inline"`

	// PolicyName is the resolved network policy name.
	PolicyName string `json:"policyName,omitempty"`

	// ObservedPolicyName is the network policy currently attached in Snowflake,
	// as read from SHOW PARAMETERS LIKE 'NETWORK_POLICY'.
	ObservedPolicyName string `json:"observedPolicyName,omitempty"`
}

// NetworkPolicyAttachment is the Schema for the networkpolicyattachments API.
// It attaches a Snowflake network policy to an account or user.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=npa,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="POLICY",type=string,JSONPath=`.status.policyName`,priority=0
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.spec.targetType`,priority=0
// +kubebuilder:printcolumn:name="TARGET-NAME",type=string,JSONPath=`.spec.targetName`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type NetworkPolicyAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPolicyAttachmentSpec   `json:"spec,omitempty"`
	Status NetworkPolicyAttachmentStatus `json:"status,omitempty"`
}

// NetworkPolicyAttachmentList contains a list of NetworkPolicyAttachment.
// +kubebuilder:object:root=true
type NetworkPolicyAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPolicyAttachment `json:"items"`
}

// GetSpecName returns a human-readable composite name for the attachment.
func (r *NetworkPolicyAttachment) GetSpecName() string {
	var policy string
	if r.Spec.PolicyName != nil {
		policy = *r.Spec.PolicyName
	} else if r.Spec.PolicyRef != nil {
		policy = fmt.Sprintf("ref:%s", r.Spec.PolicyRef.Name)
	}

	if r.Spec.TargetType == "ACCOUNT" {
		return fmt.Sprintf("%s->ACCOUNT", policy)
	}

	return fmt.Sprintf("%s->USER/%s", policy, r.Spec.TargetName)
}

func init() {
	SchemeBuilder.Register(&NetworkPolicyAttachment{}, &NetworkPolicyAttachmentList{})
}
