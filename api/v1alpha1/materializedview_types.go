package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaterializedViewSpec defines the desired state of a Snowflake materialized view.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
// +kubebuilder:validation:XValidation:rule="self.statement == oldSelf.statement",message="spec.statement is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.clusterBy) == has(self.clusterBy) && (!has(self.clusterBy) || self.clusterBy == oldSelf.clusterBy)",message="spec.clusterBy is immutable (delete and recreate the resource to change)"
type MaterializedViewSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake materialized view name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef is a reference to a Database CR. Mutually exclusive with databaseName.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. ANALYTICS). Mutually exclusive with databaseRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef is a reference to a Schema CR. Mutually exclusive with schemaName.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. PUBLIC). Mutually exclusive with schemaRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Statement is the SELECT query that defines the materialized view. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Statement string `json:"statement"`

	// Secure specifies whether the materialized view is a secure view.
	// +optional
	Secure bool `json:"secure,omitempty"`

	// Comment is the materialized view description.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// ClusterBy specifies clustering expressions for the materialized view. Immutable after creation.
	// +optional
	ClusterBy []string `json:"clusterBy,omitempty"`
}

// MaterializedViewShowOutput contains the fields from SHOW MATERIALIZED VIEWS.
type MaterializedViewShowOutput struct {
	// CreatedOn is the timestamp when the materialized view was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the materialized view name.
	Name string `json:"name,omitempty"`

	// DatabaseName is the database containing the materialized view.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the schema containing the materialized view.
	SchemaName string `json:"schemaName,omitempty"`

	// ClusterBy contains clustering column information.
	ClusterBy string `json:"clusterBy,omitempty"`

	// Rows is the number of rows in the materialized view.
	Rows string `json:"rows,omitempty"`

	// Bytes is the byte size of the materialized view data.
	Bytes string `json:"bytes,omitempty"`

	// SourceDatabaseName is the database of the base table.
	SourceDatabaseName string `json:"sourceDatabaseName,omitempty"`

	// SourceSchemaName is the schema of the base table.
	SourceSchemaName string `json:"sourceSchemaName,omitempty"`

	// SourceTableName is the base table name.
	SourceTableName string `json:"sourceTableName,omitempty"`

	// RefreshedOn is the timestamp of the last refresh operation.
	RefreshedOn string `json:"refreshedOn,omitempty"`

	// CompactedOn is the timestamp of the last compaction operation.
	CompactedOn string `json:"compactedOn,omitempty"`

	// Owner is the role that owns the materialized view.
	Owner string `json:"owner,omitempty"`

	// Invalid indicates whether the materialized view is currently invalid.
	Invalid string `json:"invalid,omitempty"`

	// InvalidReason provides the reason when the materialized view is invalid.
	InvalidReason string `json:"invalidReason,omitempty"`

	// BehindBy shows how far the materialized view is behind the base table.
	BehindBy string `json:"behindBy,omitempty"`

	// Comment is the materialized view description.
	Comment string `json:"comment,omitempty"`

	// Text is the SQL definition of the materialized view.
	Text string `json:"text,omitempty"`

	// IsSecure indicates whether the materialized view is a secure view.
	IsSecure string `json:"isSecure,omitempty"`

	// AutomaticClustering indicates whether automatic clustering is active.
	AutomaticClustering string `json:"automaticClustering,omitempty"`

	// OwnerRoleType is the type of role that owns the object.
	OwnerRoleType string `json:"ownerRoleType,omitempty"`
}

// MaterializedViewStatus defines the observed state of MaterializedView.
type MaterializedViewStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved Snowflake database name.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved Snowflake schema name.
	// +optional
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the output of SHOW MATERIALIZED VIEWS for this resource.
	// +optional
	ShowOutput *MaterializedViewShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters lists spec fields currently tracked for UNSET detection.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=matview;matviews
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// MaterializedView is the Schema for the materializedviews API.
type MaterializedView struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaterializedViewSpec   `json:"spec,omitempty"`
	Status MaterializedViewStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MaterializedViewList contains a list of MaterializedView.
type MaterializedViewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaterializedView `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaterializedView{}, &MaterializedViewList{})
}
