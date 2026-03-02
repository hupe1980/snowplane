package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipeSpec defines the desired state of a Snowflake Pipe.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.copyStatement == oldSelf.copyStatement",message="spec.copyStatement is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.autoIngest) == has(self.autoIngest) && (!has(self.autoIngest) || self.autoIngest == oldSelf.autoIngest)",message="spec.autoIngest is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.integrationRef) == has(self.integrationRef) && (!has(self.integrationRef) || self.integrationRef == oldSelf.integrationRef)",message="spec.integrationRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.integrationName) == has(self.integrationName) && (!has(self.integrationName) || self.integrationName == oldSelf.integrationName)",message="spec.integrationName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!(has(self.integrationRef) && has(self.integrationName))",message="at most one of spec.integrationRef or spec.integrationName may be set"
// +kubebuilder:validation:XValidation:rule="!has(self.integrationName) || !self.integrationName.contains('.')",message="spec.integrationName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!(has(self.errorIntegrationRef) && has(self.errorIntegrationName))",message="at most one of spec.errorIntegrationRef or spec.errorIntegrationName may be set"
// +kubebuilder:validation:XValidation:rule="!has(self.errorIntegrationName) || !self.errorIntegrationName.contains('.')",message="spec.errorIntegrationName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.awsSnsTopic) == has(self.awsSnsTopic) && (!has(self.awsSnsTopic) || self.awsSnsTopic == oldSelf.awsSnsTopic)",message="spec.awsSnsTopic is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type PipeSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake pipe name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// CopyStatement is the COPY INTO statement that defines the pipe behavior.
	// This is the core of the pipe — it specifies source stage, target table,
	// file format, and any transformation logic. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	CopyStatement string `json:"copyStatement"`

	// AutoIngest enables automatic data loading when new files arrive in the stage
	// (requires a configured notification integration). Immutable after creation.
	// +optional
	AutoIngest *bool `json:"autoIngest,omitempty"`

	// IntegrationRef references a NotificationIntegration CR in the same namespace.
	// Mutually exclusive with IntegrationName. Immutable after creation.
	// Required when autoIngest is true.
	// +optional
	IntegrationRef *LocalObjectReference `json:"integrationRef,omitempty"`

	// IntegrationName is the Snowflake notification integration identifier.
	// Mutually exclusive with IntegrationRef. Immutable after creation.
	// Required when autoIngest is true.
	// +optional
	// +kubebuilder:validation:MinLength=1
	IntegrationName *string `json:"integrationName,omitempty"`

	// AwsSnsTopic specifies the Amazon SNS topic ARN for S3 auto-ingest.
	// Used when auto-ingest relies on SNS rather than a notification integration.
	// Immutable after creation.
	// +optional
	AwsSnsTopic *string `json:"awsSnsTopic,omitempty"`

	// ErrorIntegrationRef references a NotificationIntegration CR for error notifications.
	// Mutually exclusive with ErrorIntegrationName.
	// +optional
	ErrorIntegrationRef *LocalObjectReference `json:"errorIntegrationRef,omitempty"`

	// ErrorIntegrationName is the Snowflake notification integration name for error notifications.
	// Mutually exclusive with ErrorIntegrationRef.
	// +optional
	ErrorIntegrationName *string `json:"errorIntegrationName,omitempty" snowflake:"ERROR_INTEGRATION"`

	// Comment is an optional description for the pipe.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// PipeShowOutput mirrors the SHOW PIPES output stored in status.
type PipeShowOutput struct {
	// CreatedOn is the timestamp when the pipe was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the pipe name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the pipe.
	Owner string `json:"owner,omitempty"`

	// Comment is the pipe description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Definition is the COPY INTO statement.
	Definition string `json:"definition,omitempty"`

	// NotificationChannel is the cloud notification channel ARN/URL.
	NotificationChannel string `json:"notificationChannel,omitempty"`

	// Integration is the notification integration name.
	Integration string `json:"integration,omitempty"`

	// ErrorIntegration is the error notification integration name.
	ErrorIntegration string `json:"errorIntegration,omitempty" snowflake:"ERROR_INTEGRATION"`

	// AwsSnsTopic is the SNS topic ARN for S3 auto-ingest.
	AwsSnsTopic string `json:"awsSnsTopic,omitempty"`
}

// PipeStatus defines the observed state of a Pipe.
type PipeStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// IntegrationName is the resolved notification integration name.
	IntegrationName string `json:"integrationName,omitempty"`

	// ErrorIntegrationName is the resolved error notification integration name.
	ErrorIntegrationName string `json:"errorIntegrationName,omitempty"`

	// NotificationChannel is the cloud notification channel for auto-ingest.
	// Users need this to configure the cloud event notification (e.g. SQS queue policy).
	NotificationChannel string `json:"notificationChannel,omitempty"`

	// ShowOutput contains the raw SHOW PIPES output for this pipe.
	ShowOutput *PipeShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Pipe is the Schema for the pipes API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Pipe struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PipeSpec   `json:"spec,omitempty"`
	Status PipeStatus `json:"status,omitempty"`
}

// PipeList contains a list of Pipe.
// +kubebuilder:object:root=true
type PipeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pipe `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipe{}, &PipeList{})
}
