package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShareGrantSpec defines the desired state of a ShareGrant.
// Share grants are simpler than role grants: they only support specific named
// objects (no ALL/FUTURE bulk grants) and do not support WITH GRANT OPTION.
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

// GetConditions returns the conditions.
func (r *ShareGrant) GetConditions() []metav1.Condition { return r.Status.Conditions }

// SetConditions sets the conditions.
func (r *ShareGrant) SetConditions(c []metav1.Condition) { r.Status.Conditions = c }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (r *ShareGrant) GetDeletionPolicy() DeletionPolicy {
	if r.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return r.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (r *ShareGrant) GetFullyQualifiedName() string { return r.Status.FullyQualifiedName }

// GetSpecName returns a human-readable composite name for the grant.
func (r *ShareGrant) GetSpecName() string {
	return fmt.Sprintf("%s ON %s %s -> SHARE %s",
		r.Spec.Privilege, r.Spec.ObjectType, r.Spec.ObjectName, r.Spec.Share)
}

// GetProviderRef returns the provider reference.
func (r *ShareGrant) GetProviderRef() ProviderReference { return r.Spec.ProviderRef }

// GetUseRole returns the use role (the role that executes the GRANT).
func (r *ShareGrant) GetUseRole() *string { return r.Spec.UseRole }

// GetObservedGeneration returns the observed generation.
func (r *ShareGrant) GetObservedGeneration() int64 { return r.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation.
func (r *ShareGrant) SetObservedGeneration(v int64) { r.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash.
func (r *ShareGrant) GetLastAppliedSpecHash() string { return r.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash.
func (r *ShareGrant) SetLastAppliedSpecHash(v string) { r.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns nil — grants don't track parameters.
func (r *ShareGrant) GetTrackedParametersList() []string { return nil }

// SetTrackedParametersList is a no-op for grants.
func (r *ShareGrant) SetTrackedParametersList(_ []string) {}

// GetOwner returns the granting role from status.
func (r *ShareGrant) GetOwner() string {
	if r.Status.ShowOutput != nil {
		return r.Status.ShowOutput.GrantedBy
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (r *ShareGrant) ValidateSpec() error { return r.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec.
func (r *ShareGrant) ComputeSpecHash() (string, error) { return ComputeSpecHash(r.Spec) }

func init() {
	SchemeBuilder.Register(&ShareGrant{}, &ShareGrantList{})
}
