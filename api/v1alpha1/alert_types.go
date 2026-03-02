package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertSpec defines the desired state of a Snowflake Alert.
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
// +kubebuilder:validation:XValidation:rule="!(has(self.warehouseRef) && has(self.warehouseName))",message="spec.warehouseRef and spec.warehouseName are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.warehouseName) || !self.warehouseName.contains('.')",message="spec.warehouseName must be a simple identifier, not a fully-qualified name"
type AlertSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake alert name. Immutable after creation.
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

	// WarehouseRef references a Warehouse CR in the same namespace.
	// Mutually exclusive with WarehouseName. Omit for serverless alerts.
	// +optional
	WarehouseRef *LocalObjectReference `json:"warehouseRef,omitempty"`

	// WarehouseName is the Snowflake warehouse identifier (e.g. "COMPUTE_WH").
	// Mutually exclusive with WarehouseRef. Omit for serverless alerts.
	// +optional
	// +kubebuilder:validation:MinLength=1
	WarehouseName *string `json:"warehouseName,omitempty" snowflake:"WAREHOUSE"`

	// Schedule defines the evaluation schedule for the alert.
	// Examples: "5 MINUTE", "USING CRON 0 9-17 * * SUN America/Los_Angeles".
	// Omit for alerts triggered on new data (streaming alerts).
	// +optional
	Schedule *string `json:"schedule,omitempty" snowflake:"SCHEDULE"`

	// Condition is the SQL expression evaluated by the alert.
	// Must be a SELECT, SHOW, or CALL statement. The alert fires when the
	// condition returns at least one row.
	// +kubebuilder:validation:MinLength=1
	Condition string `json:"condition"`

	// Action is the SQL statement executed when the alert condition is met.
	// +kubebuilder:validation:MinLength=1
	Action string `json:"action"`

	// Comment is an optional description for the alert.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Suspend indicates whether the alert should be suspended. Default is true
	// (alerts are created in suspended state by Snowflake).
	// +optional
	// +kubebuilder:default=true
	Suspend *bool `json:"suspend,omitempty"`
}

// AlertShowOutput mirrors the SHOW ALERTS output stored in status.
type AlertShowOutput struct {
	CreatedOn    string `json:"createdOn,omitempty"`
	Name         string `json:"name,omitempty"`
	DatabaseName string `json:"databaseName,omitempty"`
	SchemaName   string `json:"schemaName,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Comment      string `json:"comment,omitempty" snowflake:"COMMENT"`
	Warehouse    string `json:"warehouse,omitempty" snowflake:"WAREHOUSE"`
	Schedule     string `json:"schedule,omitempty" snowflake:"SCHEDULE"`
	State        string `json:"state,omitempty"`
	Condition    string `json:"condition,omitempty"`
	Action       string `json:"action,omitempty"`
}

// AlertStatus defines the observed state of an Alert.
type AlertStatus struct {
	CommonStatus      `json:",inline"`
	DatabaseName      string           `json:"databaseName,omitempty"`
	SchemaName        string           `json:"schemaName,omitempty"`
	WarehouseName     string           `json:"warehouseName,omitempty"`
	ShowOutput        *AlertShowOutput `json:"showOutput,omitempty"`
	TrackedParameters []string         `json:"trackedParameters,omitempty"`
}

// Alert is the Schema for the alerts API.
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
type Alert struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AlertSpec   `json:"spec,omitempty"`
	Status            AlertStatus `json:"status,omitempty"`
}

// AlertList contains a list of Alert.
// +kubebuilder:object:root=true
type AlertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Alert `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Alert{}, &AlertList{})
}
