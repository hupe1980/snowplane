package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConstraintType specifies the type of table constraint.
// +kubebuilder:validation:Enum="PRIMARY KEY";"UNIQUE";"FOREIGN KEY"
type ConstraintType string

const (
	// ConstraintTypePrimaryKey is a PRIMARY KEY constraint.
	ConstraintTypePrimaryKey ConstraintType = "PRIMARY KEY"

	// ConstraintTypeUnique is a UNIQUE constraint.
	ConstraintTypeUnique ConstraintType = "UNIQUE"

	// ConstraintTypeForeignKey is a FOREIGN KEY constraint.
	ConstraintTypeForeignKey ConstraintType = "FOREIGN KEY"
)

// ForeignKeyProperties defines properties specific to FOREIGN KEY constraints.
type ForeignKeyProperties struct {
	// ReferencesTableName is the fully qualified Snowflake table name that
	// the foreign key references (e.g. "MY_DB"."MY_SCHEMA"."MY_TABLE").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ReferencesTableName string `json:"referencesTableName"`

	// ReferencesColumns is the list of columns in the referenced table.
	// +kubebuilder:validation:MinItems=1
	ReferencesColumns []string `json:"referencesColumns"`

	// Match is the match type for the foreign key.
	// +optional
	// +kubebuilder:validation:Enum=FULL;PARTIAL;SIMPLE
	Match *string `json:"match,omitempty"`

	// OnUpdate specifies the action performed when the primary/unique key
	// for the foreign key is updated.
	// +optional
	// +kubebuilder:validation:Enum=CASCADE;"SET NULL";"SET DEFAULT";RESTRICT;"NO ACTION"
	OnUpdate *string `json:"onUpdate,omitempty"`

	// OnDelete specifies the action performed when the primary/unique key
	// for the foreign key is deleted.
	// +optional
	// +kubebuilder:validation:Enum=CASCADE;"SET NULL";"SET DEFAULT";RESTRICT;"NO ACTION"
	OnDelete *string `json:"onDelete,omitempty"`
}

// ConstraintProperties defines optional properties for table constraints.
type ConstraintProperties struct {
	// Enforced specifies whether the constraint is enforced.
	// +optional
	Enforced *bool `json:"enforced,omitempty"`

	// Deferrable specifies whether the constraint is deferrable.
	// +optional
	Deferrable *bool `json:"deferrable,omitempty"`

	// Initially specifies whether a deferrable constraint is
	// initially deferred or immediate.
	// +optional
	// +kubebuilder:validation:Enum=DEFERRED;IMMEDIATE
	Initially *string `json:"initially,omitempty"`

	// Rely specifies whether a constraint in NOVALIDATE mode is taken
	// into account during query rewrite.
	// +optional
	Rely *bool `json:"rely,omitempty"`

	// Validate specifies whether to validate existing data on the table
	// when a constraint is created.
	// +optional
	Validate *bool `json:"validate,omitempty"`
}

// TableConstraintSpec defines the desired state of a TableConstraint.
// Manages a Snowflake table constraint via:
//
//	ALTER TABLE <table_name> ADD CONSTRAINT <name> <type> (<columns>)
//	ALTER TABLE <table_name> DROP CONSTRAINT <name>
//
// All identity fields (name, type, tableName, columns) are immutable
// after creation — changing any of them requires deleting and
// recreating the resource.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.tableName == oldSelf.tableName",message="spec.tableName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.columns == oldSelf.columns",message="spec.columns is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.foreignKeyProperties) == has(self.foreignKeyProperties) && (!has(self.foreignKeyProperties) || self.foreignKeyProperties == oldSelf.foreignKeyProperties)",message="spec.foreignKeyProperties is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == 'FOREIGN KEY' || !has(self.foreignKeyProperties)",message="spec.foreignKeyProperties can only be set when spec.type is 'FOREIGN KEY'"
// +kubebuilder:validation:XValidation:rule="self.type != 'FOREIGN KEY' || has(self.foreignKeyProperties)",message="spec.foreignKeyProperties is required when spec.type is 'FOREIGN KEY'"
type TableConstraintSpec struct {
	CommonSpec `json:",inline"`

	// Name is the constraint name in Snowflake.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type is the constraint type.
	// Immutable after creation.
	Type ConstraintType `json:"type"`

	// TableName is the fully qualified Snowflake table name
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_TABLE").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	TableName string `json:"tableName"`

	// Columns is the list of column names that make up the constraint key.
	// Immutable after creation.
	// +kubebuilder:validation:MinItems=1
	Columns []string `json:"columns"`

	// ForeignKeyProperties is required when type is FOREIGN KEY, and
	// must not be set for other constraint types.
	// Immutable after creation.
	// +optional
	ForeignKeyProperties *ForeignKeyProperties `json:"foreignKeyProperties,omitempty"`

	// Properties defines optional constraint properties (enforced, deferrable, etc.).
	// Mutable — can be altered after creation.
	// +optional
	Properties *ConstraintProperties `json:"properties,omitempty"`

	// Comment for the constraint.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// TableConstraintStatus defines the observed state of a TableConstraint.
type TableConstraintStatus struct {
	CommonStatus `json:",inline"`

	// ConstraintName is the constraint name as observed in Snowflake.
	ConstraintName string `json:"constraintName,omitempty"`

	// ConstraintType is the constraint type as observed in Snowflake.
	ConstraintType string `json:"constraintType,omitempty"`
}

// TableConstraint is the Schema for the tableconstraints API.
// It manages a Snowflake table constraint (PRIMARY KEY, UNIQUE, or FOREIGN KEY).
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=tc,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`,priority=0
// +kubebuilder:printcolumn:name="TABLE",type=string,JSONPath=`.spec.tableName`,priority=0
// +kubebuilder:printcolumn:name="CONSTRAINT",type=string,JSONPath=`.spec.name`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type TableConstraint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TableConstraintSpec   `json:"spec,omitempty"`
	Status TableConstraintStatus `json:"status,omitempty"`
}

// TableConstraintList contains a list of TableConstraint.
// +kubebuilder:object:root=true
type TableConstraintList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TableConstraint `json:"items"`
}

// GetSpecName returns a human-readable composite name for the constraint.
func (r *TableConstraint) GetSpecName() string {
	return fmt.Sprintf("%s %s ON %s", string(r.Spec.Type), r.Spec.Name, r.Spec.TableName)
}

func init() {
	SchemeBuilder.Register(&TableConstraint{}, &TableConstraintList{})
}
