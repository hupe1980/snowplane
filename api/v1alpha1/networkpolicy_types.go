package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicySpec defines the desired state of a Snowflake Network Policy.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type NetworkPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake network policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// AllowedIPList specifies IPv4 addresses (or CIDR ranges) allowed access.
	// +optional
	AllowedIPList []string `json:"allowedIPList,omitempty" snowflake:"ALLOWED_IP_LIST,nounset"`

	// BlockedIPList specifies IPv4 addresses (or CIDR ranges) denied access.
	// +optional
	BlockedIPList []string `json:"blockedIPList,omitempty" snowflake:"BLOCKED_IP_LIST,nounset"`

	// AllowedNetworkRuleList specifies network rule names that allow access.
	// +optional
	AllowedNetworkRuleList []string `json:"allowedNetworkRuleList,omitempty" snowflake:"ALLOWED_NETWORK_RULE_LIST,nounset"`

	// BlockedNetworkRuleList specifies network rule names that deny access.
	// +optional
	BlockedNetworkRuleList []string `json:"blockedNetworkRuleList,omitempty" snowflake:"BLOCKED_NETWORK_RULE_LIST,nounset"`

	// Comment is an optional description for the network policy.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// NetworkPolicyShowOutput mirrors the SHOW NETWORK POLICIES output stored in status.
type NetworkPolicyShowOutput struct {
	// CreatedOn is the timestamp when the policy was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the policy name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Comment is the policy description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// EntriesInAllowedIPList is the number of entries in the allowed IP list.
	EntriesInAllowedIPList string `json:"entriesInAllowedIPList,omitempty"`

	// EntriesInBlockedIPList is the number of entries in the blocked IP list.
	EntriesInBlockedIPList string `json:"entriesInBlockedIPList,omitempty"`
}

// NetworkPolicyStatus defines the observed state of a NetworkPolicy.
type NetworkPolicyStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW NETWORK POLICIES output for this policy.
	ShowOutput *NetworkPolicyShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// NetworkPolicy is the Schema for the networkpolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type NetworkPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkPolicySpec   `json:"spec,omitempty"`
	Status NetworkPolicyStatus `json:"status,omitempty"`
}

// NetworkPolicyList contains a list of NetworkPolicy.
// +kubebuilder:object:root=true
type NetworkPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkPolicy{}, &NetworkPolicyList{})
}
