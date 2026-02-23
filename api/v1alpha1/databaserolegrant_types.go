package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseRoleGrantSpec defines the desired state of a DatabaseRoleGrant.
// All grant fields are immutable after creation — changing any field requires
// a REVOKE + re-GRANT (use the force-new annotation to trigger recreation).
type DatabaseRoleGrantSpec struct {
	CommonSpec `json:",inline"`

	// Privilege is the Snowflake privilege to grant (e.g. USAGE, SELECT, CREATE TABLE).
	// +kubebuilder:validation:MinLength=1
	Privilege string `json:"privilege"`

	// On defines what the privilege is granted on.
	// Exactly one variant must be set.
	On GrantOn `json:"on"`

	// DatabaseRole is the fully qualified database role name (e.g. MY_DB.MY_ROLE).
	// Mutually exclusive with DatabaseRoleRef.
	// +optional
	DatabaseRole string `json:"databaseRole,omitempty"`

	// DatabaseRoleRef references a DatabaseRole CR in the same namespace.
	// When set, the database role name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with DatabaseRole.
	// +optional
	DatabaseRoleRef *LocalObjectReference `json:"databaseRoleRef,omitempty"`

	// WithGrantOption allows the grantee to re-grant the privilege to other roles.
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// DatabaseRoleGrantStatus defines the observed state of a DatabaseRoleGrant.
type DatabaseRoleGrantStatus struct {
	CommonStatus `json:",inline"`

	// Kind indicates the resolved grant kind (Regular, Future, All).
	Kind GrantKind `json:"kind,omitempty"`

	// ShowOutput contains the raw SHOW GRANTS entry for this grant.
	ShowOutput *GrantShowOutput `json:"showOutput,omitempty"`
}

// DatabaseRoleGrant is the Schema for the databaserolegrants API.
// It grants a Snowflake privilege to a database role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type DatabaseRoleGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleGrantSpec   `json:"spec,omitempty"`
	Status DatabaseRoleGrantStatus `json:"status,omitempty"`
}

// DatabaseRoleGrantList contains a list of DatabaseRoleGrant.
// +kubebuilder:object:root=true
type DatabaseRoleGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRoleGrant `json:"items"`
}

// GetConditions returns the conditions.
func (r *DatabaseRoleGrant) GetConditions() []metav1.Condition { return r.Status.Conditions }

// SetConditions sets the conditions.
func (r *DatabaseRoleGrant) SetConditions(c []metav1.Condition) { r.Status.Conditions = c }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (r *DatabaseRoleGrant) GetDeletionPolicy() DeletionPolicy {
	if r.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return r.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (r *DatabaseRoleGrant) GetFullyQualifiedName() string { return r.Status.FullyQualifiedName }

// GetSpecName returns a human-readable composite name for the grant.
func (r *DatabaseRoleGrant) GetSpecName() string {
	role := r.Spec.DatabaseRole
	if role == "" && r.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + r.Spec.DatabaseRoleRef.Name + ")"
	}

	return fmt.Sprintf("%s %s -> DATABASE ROLE %s", r.Spec.Privilege, r.Spec.On.Description(), role)
}

// GetProviderRef returns the provider reference.
func (r *DatabaseRoleGrant) GetProviderRef() ProviderReference { return r.Spec.ProviderRef }

// GetUseRole returns the use role (the role that executes the GRANT).
func (r *DatabaseRoleGrant) GetUseRole() *string { return r.Spec.UseRole }

// GetObservedGeneration returns the observed generation.
func (r *DatabaseRoleGrant) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation.
func (r *DatabaseRoleGrant) SetObservedGeneration(v int64) { r.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash.
func (r *DatabaseRoleGrant) GetLastAppliedSpecHash() string { return r.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash.
func (r *DatabaseRoleGrant) SetLastAppliedSpecHash(v string) { r.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns nil — grants don't track parameters.
func (r *DatabaseRoleGrant) GetTrackedParametersList() []string { return nil }

// SetTrackedParametersList is a no-op for grants.
func (r *DatabaseRoleGrant) SetTrackedParametersList(_ []string) {}

// GetOwner returns the granting role from status.
func (r *DatabaseRoleGrant) GetOwner() string {
	if r.Status.ShowOutput != nil {
		return r.Status.ShowOutput.GrantedBy
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (r *DatabaseRoleGrant) ValidateSpec() error { return r.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec.
func (r *DatabaseRoleGrant) ComputeSpecHash() (string, error) { return ComputeSpecHash(r.Spec) }

// ResolvedDatabaseRole returns the resolved database role name (either direct or from ref).
func (r *DatabaseRoleGrant) ResolvedDatabaseRole() string { return r.Spec.DatabaseRole }

// ResolveKind determines the GrantKind from the spec.
func (s *DatabaseRoleGrantSpec) ResolveKind() GrantKind {
	return resolveGrantKind(&s.On)
}

func init() {
	SchemeBuilder.Register(&DatabaseRoleGrant{}, &DatabaseRoleGrantList{})
}
