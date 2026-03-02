package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TableKind specifies the persistence behaviour of a Snowflake table.
type TableKind string

// Valid TableKind values.
const (
	TableKindPermanent TableKind = "PERMANENT"
	TableKindTransient TableKind = "TRANSIENT"
	TableKindTemporary TableKind = "TEMPORARY"
)

// ColumnDefinition describes a single column in a Snowflake table.
type ColumnDefinition struct {
	// Name is the column name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type is the Snowflake data type (e.g. VARCHAR, NUMBER(38,0), TIMESTAMP_NTZ).
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// Nullable indicates whether the column allows NULL values. Defaults to true.
	// +optional
	Nullable *bool `json:"nullable,omitempty"`

	// Default is the column's default value expression.
	// +optional
	Default *string `json:"default,omitempty"`

	// Comment is an optional description for the column.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// TableConstraintType specifies the kind of table constraint.
// +kubebuilder:validation:Enum=PrimaryKey;Unique;ForeignKey
type TableConstraintType string

// Valid TableConstraintType values.
const (
	TableConstraintPrimaryKey TableConstraintType = "PrimaryKey"
	TableConstraintUnique     TableConstraintType = "Unique"
	TableConstraintForeignKey TableConstraintType = "ForeignKey"
)

// ForeignKeyReference defines the target of a FOREIGN KEY constraint.
type ForeignKeyReference struct {
	// Table is the fully qualified name of the referenced table (e.g. "ANALYTICS"."PUBLIC"."ORDERS").
	// +kubebuilder:validation:MinLength=1
	Table string `json:"table"`

	// Columns lists the column names in the referenced table.
	// +kubebuilder:validation:MinItems=1
	Columns []string `json:"columns"`
}

// InlineTableConstraint defines a table-level constraint (PRIMARY KEY, UNIQUE, FOREIGN KEY)
// embedded in a Table spec at creation time.
type InlineTableConstraint struct {
	// Name is the constraint name. If omitted, Snowflake generates one.
	// +optional
	Name string `json:"name,omitempty"`

	// Type is the constraint type.
	Type TableConstraintType `json:"type"`

	// Columns lists the column names that participate in the constraint.
	// +kubebuilder:validation:MinItems=1
	Columns []string `json:"columns"`

	// ForeignKey specifies the referenced table and columns for FOREIGN KEY constraints.
	// Required when type is ForeignKey.
	// +optional
	ForeignKey *ForeignKeyReference `json:"foreignKey,omitempty"`
}

// TableSpec defines the desired state of a Table.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.transient == oldSelf.transient",message="spec.transient is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.constraints) || !has(self.constraints) || self.constraints == oldSelf.constraints",message="spec.constraints is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type TableSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake table name. Immutable after creation.
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
	// The controller constructs the FQN from databaseName + schemaName + name.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Columns defines the table's column structure.
	// +kubebuilder:validation:MinItems=1
	Columns []ColumnDefinition `json:"columns"`

	// Constraints defines table-level constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY).
	// Constraints are applied at creation time and treated as immutable.
	// +optional
	Constraints []InlineTableConstraint `json:"constraints,omitempty"`

	// Comment is an optional description for the table.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Transient indicates this is a transient table (no Fail-safe). Immutable after creation.
	// +optional
	// +kubebuilder:default=false
	Transient bool `json:"transient,omitempty"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty" snowflake:"DATA_RETENTION_TIME_IN_DAYS"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty" snowflake:"MAX_DATA_EXTENSION_TIME_IN_DAYS"`

	// ChangeTracking enables change tracking on the table.
	// +optional
	ChangeTracking *bool `json:"changeTracking,omitempty" snowflake:"CHANGE_TRACKING"`

	// DefaultDDLCollation sets the default collation for string columns.
	// +optional
	DefaultDDLCollation *string `json:"defaultDDLCollation,omitempty" snowflake:"DEFAULT_DDL_COLLATION"`

	// EnableSchemaEvolution enables automatic schema evolution.
	// +optional
	EnableSchemaEvolution *bool `json:"enableSchemaEvolution,omitempty" snowflake:"ENABLE_SCHEMA_EVOLUTION"`

	// ClusterBy specifies the clustering key expressions for the table.
	// +optional
	ClusterBy []string `json:"clusterBy,omitempty"`
}

// TableShowOutput mirrors the SHOW TABLES output stored in status.
type TableShowOutput struct {
	// CreatedOn is the timestamp when the table was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the table name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Kind is the table kind (TABLE, TRANSIENT, or TEMPORARY).
	Kind string `json:"kind,omitempty"`

	// Comment is the table description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Owner is the role that owns the table.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`

	// ClusterBy is the clustering key expression.
	ClusterBy string `json:"clusterBy,omitempty"`

	// ChangeTracking indicates whether change tracking is enabled.
	ChangeTracking bool `json:"changeTracking,omitempty" snowflake:"CHANGE_TRACKING"`

	// EnableSchemaEvolution indicates whether schema evolution is enabled.
	EnableSchemaEvolution bool `json:"enableSchemaEvolution,omitempty" snowflake:"ENABLE_SCHEMA_EVOLUTION"`
}

// TableStatus defines the observed state of a Table.
type TableStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW TABLES output for this table.
	ShowOutput *TableShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Table is the Schema for the tables API.
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
type Table struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TableSpec   `json:"spec,omitempty"`
	Status TableStatus `json:"status,omitempty"`
}

// TableList contains a list of Table.
// +kubebuilder:object:root=true
type TableList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Table `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Table{}, &TableList{})
}
