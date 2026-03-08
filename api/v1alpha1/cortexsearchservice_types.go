package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CortexSearchServiceRefreshMode specifies how a Cortex Search Service refreshes its index.
// +kubebuilder:validation:Enum=FULL;INCREMENTAL
type CortexSearchServiceRefreshMode string

// Valid CortexSearchServiceRefreshMode values.
const (
	CortexSearchServiceRefreshModeFull        CortexSearchServiceRefreshMode = "FULL"
	CortexSearchServiceRefreshModeIncremental CortexSearchServiceRefreshMode = "INCREMENTAL"
)

// CortexSearchServiceInitialize controls when the initial data population occurs.
// +kubebuilder:validation:Enum=ON_CREATE;ON_SCHEDULE
type CortexSearchServiceInitialize string

// Valid CortexSearchServiceInitialize values.
const (
	CortexSearchServiceInitializeOnCreate   CortexSearchServiceInitialize = "ON_CREATE"
	CortexSearchServiceInitializeOnSchedule CortexSearchServiceInitialize = "ON_SCHEDULE"
)

// CortexSearchServiceSpec defines the desired state of a Snowflake Cortex Search Service.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.on == oldSelf.on",message="spec.on is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.query == oldSelf.query",message="spec.query is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.embeddingModel) == has(self.embeddingModel) && (!has(self.embeddingModel) || self.embeddingModel == oldSelf.embeddingModel)",message="spec.embeddingModel is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.refreshMode) == has(self.refreshMode) && (!has(self.refreshMode) || self.refreshMode == oldSelf.refreshMode)",message="spec.refreshMode is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.initialize) == has(self.initialize) && (!has(self.initialize) || self.initialize == oldSelf.initialize)",message="spec.initialize is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.warehouseRef) && !has(self.warehouseName)) || (!has(self.warehouseRef) && has(self.warehouseName))",message="exactly one of spec.warehouseRef or spec.warehouseName must be set"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
// +kubebuilder:validation:XValidation:rule="!has(self.warehouseName) || !self.warehouseName.contains('.')",message="spec.warehouseName must be a simple identifier, not a fully-qualified name"
type CortexSearchServiceSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake Cortex Search service name. Immutable after creation.
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

	// On is the text column to search on. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	On string `json:"on"`

	// Attributes specifies columns available as filters when querying the service.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Attributes []string `json:"attributes,omitempty"`

	// WarehouseRef references a Warehouse CR.
	// Mutually exclusive with WarehouseName.
	// +optional
	WarehouseRef *ObjectReference `json:"warehouseRef,omitempty"`

	// WarehouseName is the Snowflake warehouse identifier.
	// Mutually exclusive with WarehouseRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	WarehouseName *string `json:"warehouseName,omitempty"`

	// TargetLag specifies the maximum acceptable staleness of the search index.
	// Examples: "1 minute", "5 minutes", "1 hour", "1 day".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	TargetLag string `json:"targetLag"`

	// Query is the SQL query defining the data source (the AS clause).
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=65536
	Query string `json:"query"`

	// EmbeddingModel specifies the model used for vector embeddings.
	// Defaults to snowflake-arctic-embed-m-v1.5 if not set.
	// Immutable after creation.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	EmbeddingModel *string `json:"embeddingModel,omitempty"`

	// RefreshMode specifies the refresh strategy (FULL or INCREMENTAL).
	// Immutable after creation.
	// +optional
	RefreshMode *CortexSearchServiceRefreshMode `json:"refreshMode,omitempty"`

	// Initialize controls when initial data population occurs (ON_CREATE or ON_SCHEDULE).
	// Immutable after creation.
	// +optional
	Initialize *CortexSearchServiceInitialize `json:"initialize,omitempty"`

	// FullIndexBuildIntervalDays specifies the target interval in days between
	// full index rebuilds. Only applicable when primary key is set.
	// +optional
	// +kubebuilder:validation:Minimum=0
	FullIndexBuildIntervalDays *int32 `json:"fullIndexBuildIntervalDays,omitempty" snowflake:"FULL_INDEX_BUILD_INTERVAL_DAYS"`

	// Comment is an optional description for the Cortex Search service.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// CortexSearchServiceShowOutput mirrors the SHOW CORTEX SEARCH SERVICES output stored in status.
type CortexSearchServiceShowOutput struct {
	// CreatedOn is the timestamp when the service was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the service name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Warehouse is the warehouse used for refreshes.
	Warehouse string `json:"warehouse,omitempty"`

	// TargetLag is the configured target lag.
	TargetLag string `json:"targetLag,omitempty"`

	// Comment is the service description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`

	// SearchColumn is the column being searched on.
	SearchColumn string `json:"searchColumn,omitempty"`

	// Definition is the SQL query used to create the service.
	Definition string `json:"definition,omitempty"`
}

// CortexSearchServiceDescribeOutput mirrors the DESCRIBE CORTEX SEARCH SERVICE output.
type CortexSearchServiceDescribeOutput struct {
	// EmbeddingModel is the vector embedding model used.
	EmbeddingModel string `json:"embeddingModel,omitempty"`

	// IndexingState is the indexing state (RUNNING or SUSPENDED).
	IndexingState string `json:"indexingState,omitempty"`

	// ServingState is the serving state (RUNNING or SUSPENDED).
	ServingState string `json:"servingState,omitempty"`

	// SourceDataNumRows is the number of rows in the materialized source data.
	SourceDataNumRows string `json:"sourceDataNumRows,omitempty"`
}

// CortexSearchServiceStatus defines the observed state of a CortexSearchService.
type CortexSearchServiceStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// WarehouseName is the resolved Snowflake warehouse name.
	WarehouseName string `json:"warehouseName,omitempty"`

	// ShowOutput contains the raw SHOW CORTEX SEARCH SERVICES output.
	ShowOutput *CortexSearchServiceShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput contains the DESCRIBE CORTEX SEARCH SERVICE output.
	DescribeOutput *CortexSearchServiceDescribeOutput `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// CortexSearchService is the Schema for the cortexsearchservices API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=css
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type CortexSearchService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CortexSearchServiceSpec   `json:"spec,omitempty"`
	Status CortexSearchServiceStatus `json:"status,omitempty"`
}

// CortexSearchServiceList contains a list of CortexSearchService.
// +kubebuilder:object:root=true
type CortexSearchServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CortexSearchService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CortexSearchService{}, &CortexSearchServiceList{})
}
