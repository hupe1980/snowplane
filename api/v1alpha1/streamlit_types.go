package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StreamlitSpec defines the desired state of a Snowflake Streamlit application.
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
// +kubebuilder:validation:XValidation:rule="!has(self.warehouseName) || !self.warehouseName.contains('.')",message="spec.warehouseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="(!has(self.warehouseRef) && !has(self.warehouseName)) || (has(self.warehouseRef) && !has(self.warehouseName)) || (!has(self.warehouseRef) && has(self.warehouseName))",message="at most one of spec.warehouseRef or spec.warehouseName may be set"
type StreamlitSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake Streamlit app name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SchemaName *string `json:"schemaName,omitempty"`

	// From specifies the stage path to copy source files from during creation.
	// Used only at CREATE time; changes after creation have no effect.
	// +optional
	// +kubebuilder:validation:MaxLength=4096
	From *string `json:"from,omitempty"`

	// MainFile specifies the Streamlit entrypoint file.
	// Defaults to 'streamlit_app.py' if not set.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	MainFile *string `json:"mainFile,omitempty" snowflake:"MAIN_FILE"`

	// WarehouseRef references a Warehouse CR.
	// Mutually exclusive with WarehouseName.
	// +optional
	WarehouseRef *ObjectReference `json:"warehouseRef,omitempty"`

	// WarehouseName is the warehouse used by the Streamlit app (QUERY_WAREHOUSE).
	// Mutually exclusive with WarehouseRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	WarehouseName *string `json:"warehouseName,omitempty"`

	// Comment is an optional description for the Streamlit app.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Title is the display title for the Streamlit app in Snowsight.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Title *string `json:"title,omitempty" snowflake:"TITLE"`

	// ExternalAccessIntegrations lists external access integrations for outbound network access.
	// +optional
	ExternalAccessIntegrations []string `json:"externalAccessIntegrations,omitempty"`
}

// StreamlitShowOutput mirrors the SHOW STREAMLITS output stored in status.
type StreamlitShowOutput struct {
	// CreatedOn is the timestamp when the Streamlit was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the Streamlit name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Title is the display title.
	Title string `json:"title,omitempty"`

	// Comment is the Streamlit description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Owner is the role that owns the Streamlit app.
	Owner string `json:"owner,omitempty"`

	// QueryWarehouse is the warehouse used for queries.
	QueryWarehouse string `json:"queryWarehouse,omitempty"`

	// URLID is the unique URL identifier.
	URLID string `json:"urlId,omitempty"`

	// OwnerRoleType is the type of role that owns the Streamlit.
	OwnerRoleType string `json:"ownerRoleType,omitempty"`
}

// StreamlitDescribeOutput mirrors the DESCRIBE STREAMLIT output stored in status.
type StreamlitDescribeOutput struct {
	// Title is the display title.
	Title string `json:"title,omitempty"`

	// MainFile is the Streamlit entrypoint file.
	MainFile string `json:"mainFile,omitempty"`

	// QueryWarehouse is the warehouse used for queries.
	QueryWarehouse string `json:"queryWarehouse,omitempty"`

	// Name is the Streamlit name.
	Name string `json:"name,omitempty"`

	// Comment is the Streamlit description.
	Comment string `json:"comment,omitempty"`

	// ExternalAccessIntegrations lists the external access integrations.
	ExternalAccessIntegrations string `json:"externalAccessIntegrations,omitempty"`
}

// StreamlitStatus defines the observed state of a Streamlit.
type StreamlitStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// WarehouseName is the resolved Snowflake warehouse name.
	WarehouseName string `json:"warehouseName,omitempty"`

	// ShowOutput contains the raw SHOW STREAMLITS output.
	ShowOutput *StreamlitShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the DESCRIBE STREAMLIT output.
	DescribeOutput *StreamlitDescribeOutput `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// Streamlit is the Schema for the streamlits API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=st
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Streamlit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StreamlitSpec   `json:"spec,omitempty"`
	Status StreamlitStatus `json:"status,omitempty"`
}

// StreamlitList contains a list of Streamlit.
// +kubebuilder:object:root=true
type StreamlitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Streamlit `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Streamlit{}, &StreamlitList{})
}
