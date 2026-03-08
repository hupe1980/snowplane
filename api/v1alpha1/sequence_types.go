package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SequenceSpec defines the desired state of a Snowflake sequence.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.start) == has(self.start) && (!has(self.start) || self.start == oldSelf.start)",message="spec.start is immutable after creation (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type SequenceSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the sequence.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef is a reference to a Database resource in the same namespace.
	// Mutually exclusive with DatabaseName.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the literal Snowflake database name.
	// Mutually exclusive with DatabaseRef.
	// +optional
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef is a reference to a Schema resource in the same namespace.
	// Mutually exclusive with SchemaName.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the literal Snowflake schema name.
	// Mutually exclusive with SchemaRef.
	// +optional
	SchemaName *string `json:"schemaName,omitempty"`

	// Start is the initial value for the sequence. Immutable after creation.
	// Defaults to 1 if not specified.
	// +optional
	Start *int64 `json:"start,omitempty"`

	// Increment specifies the step interval for the sequence.
	// Defaults to 1 if not specified.
	// +optional
	Increment *int64 `json:"increment,omitempty" snowflake:"INCREMENT"`

	// Ordering controls whether the sequence is guaranteed to generate
	// values in order. Valid values are ORDER and NOORDER.
	// Note: changing from NOORDER to ORDER is not allowed by Snowflake.
	// +kubebuilder:validation:Enum=ORDER;NOORDER
	// +optional
	Ordering *string `json:"ordering,omitempty" snowflake:"ORDERING"`

	// Comment is an optional description for the sequence.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SequenceShowOutput represents the SHOW SEQUENCES output stored in status.
type SequenceShowOutput struct {
	// CreatedOn is the timestamp when the sequence was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the Snowflake name of the sequence.
	Name string `json:"name,omitempty"`

	// DatabaseName is the database containing the sequence.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the schema containing the sequence.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the sequence.
	Owner string `json:"owner,omitempty"`

	// Comment is the comment set on the sequence.
	Comment string `json:"comment,omitempty"`

	// NextValue is the next value to be generated.
	NextValue string `json:"nextValue,omitempty"`

	// Interval is the increment interval.
	Interval string `json:"interval,omitempty"`

	// Ordering is ORDER or NOORDER.
	Ordering string `json:"ordering,omitempty"`
}

// SequenceStatus defines the observed state of Sequence.
type SequenceStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved fully-qualified database name.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved fully-qualified schema name.
	// +optional
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW SEQUENCES output for this sequence.
	// +optional
	ShowOutput *SequenceShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters lists the spec fields the controller is tracking.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=seq;seqs
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="DATABASE",type="string",JSONPath=".status.databaseName"
// +kubebuilder:printcolumn:name="SCHEMA",type="string",JSONPath=".status.schemaName"
// +kubebuilder:printcolumn:name="PROVIDER",type="string",JSONPath=".spec.providerRef.name",priority=1
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// Sequence is the Schema for the sequences API.
type Sequence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SequenceSpec   `json:"spec,omitempty"`
	Status SequenceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SequenceList contains a list of Sequence.
type SequenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Sequence `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Sequence{}, &SequenceList{})
}
