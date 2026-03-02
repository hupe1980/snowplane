package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretWithClientCredentialsSpec defines the desired state of a Snowflake Secret
// with OAuth2 client credentials flow.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef",message="spec.databaseRef is immutable"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || self.databaseName == oldSelf.databaseName",message="spec.databaseName is immutable"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef",message="spec.schemaRef is immutable"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || self.schemaName == oldSelf.schemaName",message="spec.schemaName is immutable"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must not contain dots — use a simple Snowflake identifier (e.g. 'ANALYTICS')"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must not contain dots — use a simple Snowflake identifier (e.g. 'PUBLIC')"
// +kubebuilder:validation:XValidation:rule="self.apiAuthentication == oldSelf.apiAuthentication",message="spec.apiAuthentication is immutable (delete and recreate)"
type SecretWithClientCredentialsSpec struct {
	CommonSpec `json:",inline"`

	// Snowflake secret identifier (e.g. 'MY_SECRET').
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Reference to a Database CR in the same namespace.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// Snowflake database identifier (e.g. 'ANALYTICS').
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
	DatabaseName *string `json:"databaseName,omitempty"`

	// Reference to a Schema CR in the same namespace.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable"
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// Snowflake schema identifier (e.g. 'PUBLIC').
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable"
	SchemaName *string `json:"schemaName,omitempty"`

	// Snowflake security integration name for API authentication.
	// +kubebuilder:validation:MinLength=1
	APIAuthentication string `json:"apiAuthentication"`

	// OAuth scopes to request from the OAuth server.
	// +kubebuilder:validation:MinItems=1
	OAuthScopes []string `json:"oauthScopes"`

	// Comment for the secret.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SecretWithClientCredentialsStatus defines the observed state of a Snowflake Secret
// with OAuth2 client credentials flow.
type SecretWithClientCredentialsStatus struct {
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
// +kubebuilder:resource:categories=snowplane,shortName=swcc
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.status.fullyQualifiedName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// SecretWithClientCredentials is the Schema for the Snowflake Secret (OAuth2 Client Credentials) API.
type SecretWithClientCredentials struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretWithClientCredentialsSpec   `json:"spec,omitempty"`
	Status SecretWithClientCredentialsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecretWithClientCredentialsList contains a list of SecretWithClientCredentials.
type SecretWithClientCredentialsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretWithClientCredentials `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretWithClientCredentials{}, &SecretWithClientCredentialsList{})
}
