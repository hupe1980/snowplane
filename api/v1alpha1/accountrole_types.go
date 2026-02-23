package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountRoleSpec defines the desired state of an AccountRole.
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

// GetConditions returns the conditions of the AccountRole.
func (r *AccountRole) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions of the AccountRole.
func (r *AccountRole) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (r *AccountRole) GetDeletionPolicy() DeletionPolicy {
	if r.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return r.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (r *AccountRole) GetFullyQualifiedName() string {
	return r.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (r *AccountRole) GetSpecName() string { return r.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (r *AccountRole) GetProviderRef() ProviderReference { return r.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (r *AccountRole) GetUseRole() *string { return r.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (r *AccountRole) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (r *AccountRole) SetObservedGeneration(v int64) { r.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (r *AccountRole) GetLastAppliedSpecHash() string { return r.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (r *AccountRole) SetLastAppliedSpecHash(v string) { r.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (r *AccountRole) GetTrackedParametersList() []string { return r.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (r *AccountRole) SetTrackedParametersList(v []string) { r.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (r *AccountRole) GetOwner() string {
	if r.Status.ShowOutput != nil {
		return r.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (r *AccountRole) ValidateSpec() error { return r.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (r *AccountRole) ComputeSpecHash() (string, error) { return ComputeSpecHash(r.Spec) }

func init() {
	SchemeBuilder.Register(&AccountRole{}, &AccountRoleList{})
}
