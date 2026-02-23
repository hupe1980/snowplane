package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicySpec defines the desired state of a Snowflake Network Policy.
type NetworkPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake network policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// AllowedIPList specifies IPv4 addresses (or CIDR ranges) allowed access.
	// +optional
	AllowedIPList []string `json:"allowedIPList,omitempty"`

	// BlockedIPList specifies IPv4 addresses (or CIDR ranges) denied access.
	// +optional
	BlockedIPList []string `json:"blockedIPList,omitempty"`

	// AllowedNetworkRuleList specifies network rule names that allow access.
	// +optional
	AllowedNetworkRuleList []string `json:"allowedNetworkRuleList,omitempty"`

	// BlockedNetworkRuleList specifies network rule names that deny access.
	// +optional
	BlockedNetworkRuleList []string `json:"blockedNetworkRuleList,omitempty"`

	// Comment is an optional description for the network policy.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// NetworkPolicyShowOutput mirrors the SHOW NETWORK POLICIES output stored in status.
type NetworkPolicyShowOutput struct {
	// CreatedOn is the timestamp when the policy was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the policy name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Comment is the policy description.
	Comment string `json:"comment,omitempty"`

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

// GetConditions returns the conditions of the NetworkPolicy.
func (np *NetworkPolicy) GetConditions() []metav1.Condition { return np.Status.Conditions }

// SetConditions sets the conditions of the NetworkPolicy.
func (np *NetworkPolicy) SetConditions(c []metav1.Condition) { np.Status.Conditions = c }

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (np *NetworkPolicy) GetFullyQualifiedName() string { return np.Status.FullyQualifiedName }

// GetSpecName returns the Snowflake resource name from the spec.
func (np *NetworkPolicy) GetSpecName() string { return np.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (np *NetworkPolicy) GetProviderRef() ProviderReference { return np.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (np *NetworkPolicy) GetUseRole() *string { return np.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (np *NetworkPolicy) GetObservedGeneration() int64 { return np.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (np *NetworkPolicy) SetObservedGeneration(v int64) { np.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (np *NetworkPolicy) GetLastAppliedSpecHash() string { return np.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (np *NetworkPolicy) SetLastAppliedSpecHash(v string) { np.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (np *NetworkPolicy) GetTrackedParametersList() []string { return np.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (np *NetworkPolicy) SetTrackedParametersList(v []string) { np.Status.TrackedParameters = v }

// ValidateSpec validates the resource spec.
func (np *NetworkPolicy) ValidateSpec() error { return np.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (np *NetworkPolicy) ComputeSpecHash() (string, error) { return ComputeSpecHash(np.Spec) }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (np *NetworkPolicy) GetDeletionPolicy() DeletionPolicy {
	if np.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return np.Spec.DeletionPolicy
}

// GetOwner returns the owner from status.
func (np *NetworkPolicy) GetOwner() string {
	// SHOW NETWORK POLICIES does not return an owner column.
	return ""
}

func init() {
	SchemeBuilder.Register(&NetworkPolicy{}, &NetworkPolicyList{})
}
