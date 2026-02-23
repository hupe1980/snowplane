package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseRoleSpec defines the desired state of a DatabaseRole.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
type DatabaseRoleSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake database role name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier (e.g. "ANALYTICS").
	// Use this when the database is NOT managed by Snowplane.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// Comment is an optional description for the database role.
	Comment *string `json:"comment,omitempty"`
}

// DatabaseRoleShowOutput mirrors the SHOW DATABASE ROLES output stored in status.
type DatabaseRoleShowOutput struct {
	// CreatedOn is the timestamp when the database role was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the database role name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// Comment is the database role description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns this database role.
	Owner string `json:"owner,omitempty"`

	// GrantedToRoles is the number of roles this database role is granted to.
	GrantedToRoles int32 `json:"grantedToRoles,omitempty"`

	// GrantedRoles is the number of roles granted to this database role.
	GrantedRoles int32 `json:"grantedRoles,omitempty"`
}

// DatabaseRoleStatus defines the observed state of a DatabaseRole.
type DatabaseRoleStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// ShowOutput contains the raw SHOW DATABASE ROLES output for this role.
	ShowOutput *DatabaseRoleShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// DatabaseRole is the Schema for the databaseroles API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type DatabaseRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleSpec   `json:"spec,omitempty"`
	Status DatabaseRoleStatus `json:"status,omitempty"`
}

// DatabaseRoleList contains a list of DatabaseRole.
// +kubebuilder:object:root=true
type DatabaseRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRole `json:"items"`
}

// GetConditions returns the conditions of the DatabaseRole.
func (r *DatabaseRole) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions of the DatabaseRole.
func (r *DatabaseRole) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (r *DatabaseRole) GetDeletionPolicy() DeletionPolicy {
	if r.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return r.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (r *DatabaseRole) GetFullyQualifiedName() string {
	return r.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (r *DatabaseRole) GetSpecName() string { return r.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (r *DatabaseRole) GetProviderRef() ProviderReference { return r.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (r *DatabaseRole) GetUseRole() *string { return r.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (r *DatabaseRole) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (r *DatabaseRole) SetObservedGeneration(v int64) { r.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (r *DatabaseRole) GetLastAppliedSpecHash() string { return r.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (r *DatabaseRole) SetLastAppliedSpecHash(v string) { r.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (r *DatabaseRole) GetTrackedParametersList() []string { return r.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (r *DatabaseRole) SetTrackedParametersList(v []string) { r.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (r *DatabaseRole) GetOwner() string {
	if r.Status.ShowOutput != nil {
		return r.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (r *DatabaseRole) ValidateSpec() error { return r.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (r *DatabaseRole) ComputeSpecHash() (string, error) { return ComputeSpecHash(r.Spec) }

func init() {
	SchemeBuilder.Register(&DatabaseRole{}, &DatabaseRoleList{})
}
