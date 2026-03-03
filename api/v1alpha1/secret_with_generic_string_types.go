package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretWithGenericStringSpec defines the desired state of a Snowflake Secret
// with a generic string value.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type SecretWithGenericStringSpec struct {
	CommonSpec `json:",inline"`

	// Snowflake secret identifier (e.g. 'MY_SECRET').
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Reference to a Database CR in the same namespace.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// Snowflake database identifier (e.g. 'ANALYTICS').
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
	DatabaseName *string `json:"databaseName,omitempty"`

	// Reference to a Schema CR in the same namespace.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable"
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// Snowflake schema identifier (e.g. 'PUBLIC').
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable"
	SchemaName *string `json:"schemaName,omitempty"`

	// The secret string value (e.g. API token).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SecretString string `json:"secretString"`

	// Comment for the secret.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SecretWithGenericStringStatus defines the observed state of a Snowflake Secret
// with a generic string value.
type SecretWithGenericStringStatus struct {
	CommonStatus `json:",inline"`

	// Observed database name (resolved from ref or spec).
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// Observed schema name (resolved from ref or spec).
	// +optional
	SchemaName string `json:"schemaName,omitempty"`

	// Output from SHOW SECRETS.
	// +optional
	ShowOutput *SecretShowOutput `json:"showOutput,omitempty"`

	// Output from DESCRIBE SECRET.
	// +optional
	DescribeOutput *SecretDescribeOutput `json:"describeOutput,omitempty"`

	// Tracked parameters for UNSET operations.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=swgs
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.status.fullyQualifiedName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// SecretWithGenericString is the Schema for the Snowflake Secret (Generic String) API.
type SecretWithGenericString struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretWithGenericStringSpec   `json:"spec,omitempty"`
	Status SecretWithGenericStringStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecretWithGenericStringList contains a list of SecretWithGenericString.
type SecretWithGenericStringList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretWithGenericString `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretWithGenericString{}, &SecretWithGenericStringList{})
}
