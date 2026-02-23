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
	Comment *string `json:"comment,omitempty"`
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

// TableConstraint defines a table-level constraint (PRIMARY KEY, UNIQUE, FOREIGN KEY).
type TableConstraint struct {
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
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type TableSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake table name. Immutable after creation.
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

	// Columns defines the table's column structure.
	// +kubebuilder:validation:MinItems=1
	Columns []ColumnDefinition `json:"columns"`

	// Constraints defines table-level constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY).
	// Constraints are applied at creation time and treated as immutable.
	// +optional
	Constraints []TableConstraint `json:"constraints,omitempty"`

	// Comment is an optional description for the table.
	// +optional
	Comment *string `json:"comment,omitempty"`

	// Transient indicates this is a transient table (no Fail-safe). Immutable after creation.
	// +optional
	Transient bool `json:"transient,omitempty"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0–90 days).
	// +optional
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	// +optional
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty"`

	// ChangeTracking enables change tracking on the table.
	// +optional
	ChangeTracking *bool `json:"changeTracking,omitempty"`

	// DefaultDDLCollation sets the default collation for string columns.
	// +optional
	DefaultDDLCollation *string `json:"defaultDdlCollation,omitempty"`

	// EnableSchemaEvolution enables automatic schema evolution.
	// +optional
	EnableSchemaEvolution *bool `json:"enableSchemaEvolution,omitempty"`

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
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns the table.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`

	// ClusterBy is the clustering key expression.
	ClusterBy string `json:"clusterBy,omitempty"`

	// ChangeTracking indicates whether change tracking is enabled.
	ChangeTracking bool `json:"changeTracking,omitempty"`

	// EnableSchemaEvolution indicates whether schema evolution is enabled.
	EnableSchemaEvolution bool `json:"enableSchemaEvolution,omitempty"`
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

// GetConditions returns the conditions of the Table.
func (t *Table) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

// SetConditions sets the conditions of the Table.
func (t *Table) SetConditions(conditions []metav1.Condition) {
	t.Status.Conditions = conditions
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (t *Table) GetDeletionPolicy() DeletionPolicy {
	if t.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return t.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (t *Table) GetFullyQualifiedName() string {
	return t.Status.FullyQualifiedName
}

// GetSpecName returns the Snowflake resource name from the spec.
func (t *Table) GetSpecName() string { return t.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (t *Table) GetProviderRef() ProviderReference { return t.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (t *Table) GetUseRole() *string { return t.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (t *Table) GetObservedGeneration() int64 { return t.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (t *Table) SetObservedGeneration(v int64) { t.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (t *Table) GetLastAppliedSpecHash() string { return t.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (t *Table) SetLastAppliedSpecHash(v string) { t.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (t *Table) GetTrackedParametersList() []string { return t.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (t *Table) SetTrackedParametersList(v []string) { t.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (t *Table) GetOwner() string {
	if t.Status.ShowOutput != nil {
		return t.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (t *Table) ValidateSpec() error { return t.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (t *Table) ComputeSpecHash() (string, error) { return ComputeSpecHash(t.Spec) }

func init() {
	SchemeBuilder.Register(&Table{}, &TableList{})
}
