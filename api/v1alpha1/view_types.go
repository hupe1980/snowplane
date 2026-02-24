package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ViewSpec defines the desired state of a View.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type ViewSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake view name. Immutable after creation.
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

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN (e.g. '"ANALYTICS"."PUBLIC"').
	// Use this when the schema is NOT managed by Snowplane.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Statement is the SQL SELECT statement that defines the view.
	// Changing this field triggers a DROP and CREATE OR REPLACE since Snowflake
	// does not support ALTER VIEW ... AS.
	//
	// SECURITY NOTE: This statement is executed verbatim as SQL with the
	// privileges of the operator's Snowflake service account. Users with
	// permission to create or update View CRs can execute arbitrary SQL.
	// Ensure RBAC restricts View CR access to trusted principals only.
	// +kubebuilder:validation:MinLength=1
	Statement string `json:"statement"`

	// Secure enables the SECURE VIEW property.
	// +optional
	Secure bool `json:"secure,omitempty"`

	// Comment is an optional description for the view.
	// +optional
	Comment *string `json:"comment,omitempty"`

	// ChangeTracking enables change tracking on the view.
	// +optional
	ChangeTracking *bool `json:"changeTracking,omitempty"`
}

// ViewShowOutput mirrors the SHOW VIEWS output stored in status.
type ViewShowOutput struct {
	// CreatedOn is the timestamp when the view was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the view name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Comment is the view description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns the view.
	Owner string `json:"owner,omitempty"`

	// IsSecure indicates whether the view is a secure view.
	IsSecure bool `json:"isSecure,omitempty"`

	// Text is the view definition (CREATE VIEW command text).
	Text string `json:"text,omitempty"`

	// ChangeTracking indicates whether change tracking is enabled.
	ChangeTracking bool `json:"changeTracking,omitempty"`
}

// ViewStatus defines the observed state of a View.
type ViewStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW VIEWS output for this view.
	ShowOutput *ViewShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// View is the Schema for the views API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type View struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ViewSpec   `json:"spec,omitempty"`
	Status ViewStatus `json:"status,omitempty"`
}

// ViewList contains a list of View.
// +kubebuilder:object:root=true
type ViewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []View `json:"items"`
}

func init() {
	SchemeBuilder.Register(&View{}, &ViewList{})
}
