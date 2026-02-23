package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TagSpec defines the desired state of a Snowflake Tag.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type TagSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake tag name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// AllowedValues specifies the valid string values that can be assigned
	// when the tag is set on an object. If empty, all string values are allowed.
	// +optional
	// +kubebuilder:validation:MaxItems=5000
	AllowedValues []string `json:"allowedValues,omitempty"`

	// Comment is an optional description for the tag.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// TagShowOutput mirrors the SHOW TAGS output stored in status.
type TagShowOutput struct {
	// CreatedOn is the timestamp when the tag was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the tag name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the tag.
	Owner string `json:"owner,omitempty"`

	// Comment is the tag description.
	Comment string `json:"comment,omitempty"`

	// AllowedValues is the comma-separated list of allowed values.
	AllowedValues string `json:"allowedValues,omitempty"`
}

// TagStatus defines the observed state of a Tag.
type TagStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW TAGS output for this tag.
	ShowOutput *TagShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Tag is the Schema for the tags API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Tag struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TagSpec   `json:"spec,omitempty"`
	Status TagStatus `json:"status,omitempty"`
}

// TagList contains a list of Tag.
// +kubebuilder:object:root=true
type TagList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tag `json:"items"`
}

// GetConditions returns the conditions of the Tag.
func (t *Tag) GetConditions() []metav1.Condition { return t.Status.Conditions }

// SetConditions sets the conditions of the Tag.
func (t *Tag) SetConditions(c []metav1.Condition) { t.Status.Conditions = c }

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (t *Tag) GetFullyQualifiedName() string { return t.Status.FullyQualifiedName }

// GetSpecName returns the Snowflake resource name from the spec.
func (t *Tag) GetSpecName() string { return t.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (t *Tag) GetProviderRef() ProviderReference { return t.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (t *Tag) GetUseRole() *string { return t.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (t *Tag) GetObservedGeneration() int64 { return t.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (t *Tag) SetObservedGeneration(v int64) { t.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (t *Tag) GetLastAppliedSpecHash() string { return t.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (t *Tag) SetLastAppliedSpecHash(v string) { t.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (t *Tag) GetTrackedParametersList() []string { return t.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (t *Tag) SetTrackedParametersList(v []string) { t.Status.TrackedParameters = v }

// ValidateSpec validates the resource spec.
func (t *Tag) ValidateSpec() error { return t.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (t *Tag) ComputeSpecHash() (string, error) { return ComputeSpecHash(t.Spec) }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (t *Tag) GetDeletionPolicy() DeletionPolicy {
	if t.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return t.Spec.DeletionPolicy
}

// GetOwner returns the owner from status.
func (t *Tag) GetOwner() string {
	if t.Status.ShowOutput != nil {
		return t.Status.ShowOutput.Owner
	}

	return ""
}

func init() {
	SchemeBuilder.Register(&Tag{}, &TagList{})
}
