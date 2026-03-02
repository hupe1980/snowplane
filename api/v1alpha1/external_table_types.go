package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExternalTableColumnDefinition describes a column in a Snowflake external table.
type ExternalTableColumnDefinition struct {
	// Name is the column identifier.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type is the Snowflake data type (e.g. VARCHAR, NUMBER, TIMESTAMP_NTZ).
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// As is the SQL expression for the column value.
	// For regular columns, reference the VALUE variant (e.g. "value:col1::varchar").
	// For partition columns, use METADATA$FILENAME or METADATA$EXTERNAL_TABLE_PARTITION.
	// +kubebuilder:validation:MinLength=1
	As string `json:"as"`
}

// ExternalTableSpec defines the desired state of a Snowflake external table.
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
// +kubebuilder:validation:XValidation:rule="self.location == oldSelf.location",message="spec.location is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.fileFormat == oldSelf.fileFormat",message="spec.fileFormat is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.columns) || !has(self.columns) || self.columns == oldSelf.columns",message="spec.columns is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.partitionBy) || !has(self.partitionBy) || self.partitionBy == oldSelf.partitionBy",message="spec.partitionBy is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.partitionType) == has(self.partitionType) && (!has(self.partitionType) || self.partitionType == oldSelf.partitionType)",message="spec.partitionType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.pattern) == has(self.pattern) && (!has(self.pattern) || self.pattern == oldSelf.pattern)",message="spec.pattern is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.refreshOnCreate) == has(self.refreshOnCreate) && (!has(self.refreshOnCreate) || self.refreshOnCreate == oldSelf.refreshOnCreate)",message="spec.refreshOnCreate is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.awsSnsTopic) == has(self.awsSnsTopic) && (!has(self.awsSnsTopic) || self.awsSnsTopic == oldSelf.awsSnsTopic)",message="spec.awsSnsTopic is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.tableFormat) == has(self.tableFormat) && (!has(self.tableFormat) || self.tableFormat == oldSelf.tableFormat)",message="spec.tableFormat is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.integration) == has(self.integration) && (!has(self.integration) || self.integration == oldSelf.integration)",message="spec.integration is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.comment) == has(self.comment) && (!has(self.comment) || self.comment == oldSelf.comment)",message="spec.comment is immutable (delete and recreate the resource to change)"
type ExternalTableSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake external table name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Use this when the database is NOT managed by Snowplane.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Use this when the schema is NOT managed by Snowplane.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Location specifies the external stage and optional path (e.g. "@MYDB.MYSCHEMA.MYSTAGE/path/").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Location string `json:"location"`

	// FileFormat specifies the file format specification.
	// Examples: "TYPE = PARQUET", "TYPE = CSV FIELD_DELIMITER = '|'", "FORMAT_NAME = 'my_format'".
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	FileFormat string `json:"fileFormat"`

	// Columns defines the external table's virtual column definitions.
	// Each column specifies a name, type, and an AS expression that derives
	// the column value from the staged data (VALUE variant or METADATA$FILENAME).
	// Immutable after creation.
	// +optional
	Columns []ExternalTableColumnDefinition `json:"columns,omitempty"`

	// PartitionBy specifies one or more partition column names defined in Columns.
	// Immutable after creation.
	// +optional
	PartitionBy []string `json:"partitionBy,omitempty"`

	// PartitionType specifies the partition type.
	// Set to "USER_SPECIFIED" for manually managed partitions.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=USER_SPECIFIED
	// +optional
	PartitionType *string `json:"partitionType,omitempty"`

	// Pattern is a regular expression pattern to match filenames in the stage location.
	// Immutable after creation.
	// +optional
	Pattern *string `json:"pattern,omitempty"`

	// RefreshOnCreate controls whether the external table metadata is automatically
	// refreshed once immediately after creation. Defaults to true in Snowflake.
	// Immutable after creation.
	// +optional
	RefreshOnCreate *bool `json:"refreshOnCreate,omitempty"`

	// AutoRefresh controls whether Snowflake automatically refreshes the external
	// table metadata when new or updated data files are available.
	// This is the only mutable field after creation.
	// +optional
	AutoRefresh *bool `json:"autoRefresh,omitempty" snowflake:"AUTO_REFRESH,nounset"`

	// AwsSnsTopic is the Amazon SNS topic ARN for automatic refresh on S3.
	// Immutable after creation.
	// +optional
	AwsSnsTopic *string `json:"awsSnsTopic,omitempty"`

	// TableFormat identifies the external table format.
	// Set to "DELTA" for Delta Lake tables.
	// Immutable after creation.
	// +kubebuilder:validation:Enum=DELTA
	// +optional
	TableFormat *string `json:"tableFormat,omitempty"`

	// Integration specifies the notification integration name for GCS or Azure auto-refresh.
	// Immutable after creation.
	// +optional
	Integration *string `json:"integration,omitempty"`

	// Comment is an optional description for the external table.
	// Immutable after creation (ALTER EXTERNAL TABLE does not support SET COMMENT).
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// ExternalTableShowOutput mirrors the SHOW EXTERNAL TABLES output stored in status.
type ExternalTableShowOutput struct {
	// CreatedOn is the timestamp when the external table was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the external table name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Invalid indicates whether the referenced stage or file format is dropped.
	Invalid string `json:"invalid,omitempty"`

	// InvalidReason provides the reason when the external table is invalid.
	InvalidReason string `json:"invalidReason,omitempty"`

	// Owner is the role that owns the external table.
	Owner string `json:"owner,omitempty"`

	// Comment is the external table description.
	Comment string `json:"comment,omitempty"`

	// Stage is the fully qualified name of the referenced stage.
	Stage string `json:"stage,omitempty"`

	// Location is the external stage and folder path.
	Location string `json:"location,omitempty"`

	// FileFormatName is the named file format in the external table definition.
	FileFormatName string `json:"fileFormatName,omitempty"`

	// FileFormatType is the file format type (CSV, JSON, PARQUET, etc.).
	FileFormatType string `json:"fileFormatType,omitempty"`

	// Cloud is the cloud provider where staged data files are located.
	Cloud string `json:"cloud,omitempty"`

	// Region is the cloud region.
	Region string `json:"region,omitempty"`

	// NotificationChannel is the notification channel ARN/name.
	NotificationChannel string `json:"notificationChannel,omitempty"`

	// LastRefreshedOn is the timestamp of the last metadata refresh.
	LastRefreshedOn string `json:"lastRefreshedOn,omitempty"`

	// TableFormat is the table format (e.g. DELTA, UNSPECIFIED).
	TableFormat string `json:"tableFormat,omitempty"`

	// LastRefreshDetails contains diagnostic information about the last metadata refresh.
	LastRefreshDetails string `json:"lastRefreshDetails,omitempty"`

	// OwnerRoleType is the type of role that owns the object.
	OwnerRoleType string `json:"ownerRoleType,omitempty"`
}

// ExternalTableStatus defines the observed state of ExternalTable.
type ExternalTableStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved Snowflake database name.
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved Snowflake schema name.
	// +optional
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW EXTERNAL TABLES output for this table.
	// +optional
	ShowOutput *ExternalTableShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters lists the spec fields the controller is tracking.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=exttbl;exttbls,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="DATABASE",type="string",JSONPath=".status.databaseName"
// +kubebuilder:printcolumn:name="SCHEMA",type="string",JSONPath=".status.schemaName"
// +kubebuilder:printcolumn:name="PROVIDER",type="string",JSONPath=".spec.providerRef.name",priority=1
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// ExternalTable is the Schema for the external tables API.
type ExternalTable struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExternalTableSpec   `json:"spec,omitempty"`
	Status ExternalTableStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalTableList contains a list of ExternalTable.
type ExternalTableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalTable `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalTable{}, &ExternalTableList{})
}
