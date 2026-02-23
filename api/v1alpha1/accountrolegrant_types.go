package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountRoleGrantSpec defines the desired state of an AccountRoleGrant.
// All grant fields are immutable after creation — changing any field requires
// a REVOKE + re-GRANT (use the force-new annotation to trigger recreation).
type AccountRoleGrantSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant (e.g. USAGE, SELECT, CREATE SCHEMA).
	// +kubebuilder:validation:MinLength=1
	Privilege string `json:"privilege"`

	// On defines what the privilege is granted on.
	// Exactly one variant must be set.
	On GrantOn `json:"on"`

	// AccountRole is the name of the account role to grant the privilege to.
	// Mutually exclusive with AccountRoleRef.
	// +optional
	AccountRole string `json:"accountRole,omitempty"`

	// AccountRoleRef references an AccountRole CR in the same namespace.
	// When set, the role name is resolved from the CR's spec.name.
	// Mutually exclusive with AccountRole.
	// +optional
	AccountRoleRef *LocalObjectReference `json:"accountRoleRef,omitempty"`

	// WithGrantOption allows the grantee to re-grant the privilege to other roles.
	//
	// SECURITY NOTE: When set to true, the grantee can re-grant this privilege
	// to other roles, creating delegation chains. Revoking this CR does
	// NOT cascade-revoke grants made by the grantee. Use with caution.
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// AccountRoleGrantStatus defines the observed state of an AccountRoleGrant.
type AccountRoleGrantStatus struct {
	CommonStatus `json:",inline"`

	// Kind indicates the resolved grant kind (Regular, Future, All).
	Kind GrantKind `json:"kind,omitempty"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// AccountRoleGrant is the Schema for the accountrolegrants API.
// It grants a Snowflake privilege to an account role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type AccountRoleGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountRoleGrantSpec   `json:"spec,omitempty"`
	Status AccountRoleGrantStatus `json:"status,omitempty"`
}

// AccountRoleGrantList contains a list of AccountRoleGrant.
// +kubebuilder:object:root=true
type AccountRoleGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AccountRoleGrant `json:"items"`
}

// GetConditions returns the conditions.
func (r *AccountRoleGrant) GetConditions() []metav1.Condition { return r.Status.Conditions }

// SetConditions sets the conditions.
func (r *AccountRoleGrant) SetConditions(c []metav1.Condition) { r.Status.Conditions = c }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (r *AccountRoleGrant) GetDeletionPolicy() DeletionPolicy {
	if r.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return r.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (r *AccountRoleGrant) GetFullyQualifiedName() string { return r.Status.FullyQualifiedName }

// GetSpecName returns a human-readable composite name for the grant.
func (r *AccountRoleGrant) GetSpecName() string {
	role := r.Spec.AccountRole
	if role == "" && r.Spec.AccountRoleRef != nil {
		role = "(ref: " + r.Spec.AccountRoleRef.Name + ")"
	}

	return fmt.Sprintf("%s %s -> ROLE %s", r.Spec.Privilege, r.Spec.On.Description(), role)
}

// GetProviderRef returns the provider reference.
func (r *AccountRoleGrant) GetProviderRef() ProviderReference { return r.Spec.ProviderRef }

// GetUseRole returns the use role (the role that executes the GRANT).
func (r *AccountRoleGrant) GetUseRole() *string { return r.Spec.UseRole }

// GetObservedGeneration returns the observed generation.
func (r *AccountRoleGrant) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation.
func (r *AccountRoleGrant) SetObservedGeneration(v int64) { r.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash.
func (r *AccountRoleGrant) GetLastAppliedSpecHash() string { return r.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash.
func (r *AccountRoleGrant) SetLastAppliedSpecHash(v string) { r.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns nil — grants don't track parameters.
func (r *AccountRoleGrant) GetTrackedParametersList() []string { return nil }

// SetTrackedParametersList is a no-op for grants.
func (r *AccountRoleGrant) SetTrackedParametersList(_ []string) {}

// GetOwner returns the granting role from status.
func (r *AccountRoleGrant) GetOwner() string {
	if r.Status.ShowOutput != nil {
		return r.Status.ShowOutput.GrantedBy
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (r *AccountRoleGrant) ValidateSpec() error { return r.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec.
func (r *AccountRoleGrant) ComputeSpecHash() (string, error) { return ComputeSpecHash(r.Spec) }

// ResolvedAccountRole returns the resolved account role name (either direct or from ref).
func (r *AccountRoleGrant) ResolvedAccountRole() string { return r.Spec.AccountRole }

// ResolveKind determines the GrantKind from the spec.
func (s *AccountRoleGrantSpec) ResolveKind() GrantKind {
	return resolveGrantKind(&s.On)
}

func init() {
	SchemeBuilder.Register(&AccountRoleGrant{}, &AccountRoleGrantList{})
}
