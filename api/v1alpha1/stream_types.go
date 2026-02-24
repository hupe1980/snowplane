package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StreamSourceType specifies the type of source object for a Snowflake Stream.
// +kubebuilder:validation:Enum=TABLE;VIEW;EXTERNAL_TABLE;STAGE;DYNAMIC_TABLE
type StreamSourceType string

// Valid StreamSourceType values.
const (
	StreamSourceTable         StreamSourceType = "TABLE"
	StreamSourceView          StreamSourceType = "VIEW"
	StreamSourceExternalTable StreamSourceType = "EXTERNAL_TABLE"
	StreamSourceStage         StreamSourceType = "STAGE"
	StreamSourceDynamicTable  StreamSourceType = "DYNAMIC_TABLE"
)

// StreamSpec defines the desired state of a Snowflake Stream.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.sourceType == oldSelf.sourceType",message="spec.sourceType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.sourceName == oldSelf.sourceName",message="spec.sourceName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type StreamSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake stream name. Immutable after creation.
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

	// SourceType specifies the type of object the stream is created on.
	// Immutable after creation.
	SourceType StreamSourceType `json:"sourceType"`

	// SourceName is the fully qualified name of the source object
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_TABLE"). Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	SourceName string `json:"sourceName"`

	// AppendOnly tracks row inserts only (TABLE/VIEW streams only).
	// +optional
	AppendOnly *bool `json:"appendOnly,omitempty"`

	// InsertOnly tracks row inserts only (EXTERNAL_TABLE streams only).
	// +optional
	InsertOnly *bool `json:"insertOnly,omitempty"`

	// ShowInitialRows returns existing rows on first consume (TABLE/VIEW streams only).
	// +optional
	ShowInitialRows *bool `json:"showInitialRows,omitempty"`

	// Comment is an optional description for the stream.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// StreamShowOutput mirrors the SHOW STREAMS output stored in status.
type StreamShowOutput struct {
	// CreatedOn is the timestamp when the stream was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the stream name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the stream.
	Owner string `json:"owner,omitempty"`

	// Comment is the stream description.
	Comment string `json:"comment,omitempty"`

	// TableName is the fully qualified source object name.
	TableName string `json:"tableName,omitempty"`

	// SourceType is the type of source object (TABLE, VIEW, STAGE, etc.).
	SourceType string `json:"sourceType,omitempty"`

	// Mode is the stream mode (DEFAULT, APPEND_ONLY, INSERT_ONLY).
	Mode string `json:"mode,omitempty"`

	// Stale is whether the stream is stale.
	Stale bool `json:"stale,omitempty"`

	// StaleAfter is the timestamp after which the stream becomes stale.
	StaleAfter string `json:"staleAfter,omitempty"`
}

// StreamStatus defines the observed state of a Stream.
type StreamStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW STREAMS output for this stream.
	ShowOutput *StreamShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Stream is the Schema for the streams API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Stream struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StreamSpec   `json:"spec,omitempty"`
	Status StreamStatus `json:"status,omitempty"`
}

// StreamList contains a list of Stream.
// +kubebuilder:object:root=true
type StreamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Stream `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Stream{}, &StreamList{})
}
