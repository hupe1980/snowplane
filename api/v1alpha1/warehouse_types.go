package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WarehouseType specifies the type of warehouse.
type WarehouseType string

const (
	// WarehouseTypeStandard is the standard warehouse type.
	WarehouseTypeStandard WarehouseType = "STANDARD"
	// WarehouseTypeSnowparkOptimized is the Snowpark-optimized warehouse type.
	WarehouseTypeSnowparkOptimized WarehouseType = "SNOWPARK-OPTIMIZED"
)

// WarehouseSize specifies the size of the warehouse.
type WarehouseSize string

const (
	// WarehouseSizeXSmall is the X-Small warehouse size.
	WarehouseSizeXSmall WarehouseSize = "XSMALL"
	// WarehouseSizeSmall is the Small warehouse size.
	WarehouseSizeSmall WarehouseSize = "SMALL"
	// WarehouseSizeMedium is the Medium warehouse size.
	WarehouseSizeMedium WarehouseSize = "MEDIUM"
	// WarehouseSizeLarge is the Large warehouse size.
	WarehouseSizeLarge WarehouseSize = "LARGE"
	// WarehouseSizeXLarge is the X-Large warehouse size.
	WarehouseSizeXLarge WarehouseSize = "XLARGE"
	// WarehouseSize2XLarge is the 2X-Large warehouse size.
	WarehouseSize2XLarge WarehouseSize = "2XLARGE"
	// WarehouseSize3XLarge is the 3X-Large warehouse size.
	WarehouseSize3XLarge WarehouseSize = "3XLARGE"
	// WarehouseSize4XLarge is the 4X-Large warehouse size.
	WarehouseSize4XLarge WarehouseSize = "4XLARGE"
	// WarehouseSize5XLarge is the 5X-Large warehouse size.
	WarehouseSize5XLarge WarehouseSize = "5XLARGE"
	// WarehouseSize6XLarge is the 6X-Large warehouse size.
	WarehouseSize6XLarge WarehouseSize = "6XLARGE"
)

// ScalingPolicy specifies how multi-cluster warehouses scale.
type ScalingPolicy string

const (
	// ScalingPolicyStandard is the standard scaling policy.
	ScalingPolicyStandard ScalingPolicy = "STANDARD"
	// ScalingPolicyEconomy is the economy scaling policy.
	ScalingPolicyEconomy ScalingPolicy = "ECONOMY"
)

// ResourceConstraint specifies how resources are constrained within a warehouse.
type ResourceConstraint string

const (
	// ResourceConstraintMemory constrains warehouse resources by memory.
	ResourceConstraintMemory ResourceConstraint = "MEMORY"
)

// WarehouseState represents the running state of a warehouse.
type WarehouseState string

const (
	// WarehouseStateStarted indicates the warehouse is running.
	WarehouseStateStarted WarehouseState = "STARTED"
	// WarehouseStateSuspended indicates the warehouse is suspended.
	WarehouseStateSuspended WarehouseState = "SUSPENDED"
	// WarehouseStateResizing indicates the warehouse is being resized.
	WarehouseStateResizing WarehouseState = "RESIZING"
)

// WarehouseSpec defines the desired state of a Warehouse.
type WarehouseSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake warehouse name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// WarehouseType specifies the warehouse type (STANDARD or SNOWPARK-OPTIMIZED).
	// Mutable — Snowflake supports changing warehouse type on a running warehouse.
	// +kubebuilder:validation:Enum=STANDARD;SNOWPARK-OPTIMIZED
	WarehouseType *WarehouseType `json:"warehouseType,omitempty"`

	// WarehouseSize specifies the compute size.
	// +kubebuilder:validation:Enum="XSMALL";"SMALL";"MEDIUM";"LARGE";"XLARGE";"2XLARGE";"3XLARGE";"4XLARGE";"5XLARGE";"6XLARGE"
	WarehouseSize *WarehouseSize `json:"warehouseSize,omitempty"`

	// MinClusterCount is the minimum number of clusters (multi-cluster mode).
	MinClusterCount *int32 `json:"minClusterCount,omitempty"`

	// MaxClusterCount is the maximum number of clusters (multi-cluster mode).
	MaxClusterCount *int32 `json:"maxClusterCount,omitempty"`

	// ScalingPolicy controls multi-cluster scaling behavior.
	// +kubebuilder:validation:Enum=STANDARD;ECONOMY
	ScalingPolicy *ScalingPolicy `json:"scalingPolicy,omitempty"`

	// AutoSuspend is the number of seconds of inactivity before auto-suspend.
	AutoSuspend *int32 `json:"autoSuspend,omitempty"`

	// AutoResume controls whether the warehouse auto-resumes on query.
	AutoResume *bool `json:"autoResume,omitempty"`

	// InitiallySuspended creates the warehouse in a suspended state.
	// This is a CREATE-only field and is not applied on updates.
	InitiallySuspended bool `json:"initiallySuspended,omitempty"`

	// ResourceMonitor is the name of the resource monitor to attach.
	ResourceMonitor *string `json:"resourceMonitor,omitempty"`

	// Comment is an optional description.
	Comment *string `json:"comment,omitempty"`

	// EnableQueryAcceleration enables the query acceleration service.
	EnableQueryAcceleration *bool `json:"enableQueryAcceleration,omitempty"`

	// QueryAccelerationMaxScaleFactor limits query acceleration scaling (0–100).
	QueryAccelerationMaxScaleFactor *int32 `json:"queryAccelerationMaxScaleFactor,omitempty"`

	// MaxConcurrencyLevel limits concurrent queries (1–32, default 8).
	MaxConcurrencyLevel *int32 `json:"maxConcurrencyLevel,omitempty"`

	// StatementQueuedTimeoutInSeconds controls how long queries queue before timing out.
	StatementQueuedTimeoutInSeconds *int32 `json:"statementQueuedTimeoutInSeconds,omitempty"`

	// StatementTimeoutInSeconds controls the max execution time per statement.
	StatementTimeoutInSeconds *int32 `json:"statementTimeoutInSeconds,omitempty"`

	// ResourceConstraint specifies resource constraints (e.g., MEMORY).
	// +kubebuilder:validation:Enum=MEMORY
	ResourceConstraint *ResourceConstraint `json:"resourceConstraint,omitempty"`
}

// WarehouseShowOutput mirrors the SHOW WAREHOUSES output stored in status.
type WarehouseShowOutput struct {
	CreatedOn       string `json:"createdOn,omitempty"`
	Name            string `json:"name,omitempty"`
	State           string `json:"state,omitempty"`
	Type            string `json:"type,omitempty"`
	Size            string `json:"size,omitempty"`
	Comment         string `json:"comment,omitempty"`
	Owner           string `json:"owner,omitempty"`
	AutoSuspend     int32  `json:"autoSuspend,omitempty"`
	AutoResume      bool   `json:"autoResume,omitempty"`
	MinClusterCount int32  `json:"minClusterCount,omitempty"`
	MaxClusterCount int32  `json:"maxClusterCount,omitempty"`
	ScalingPolicy   string `json:"scalingPolicy,omitempty"`
	ResourceMonitor string `json:"resourceMonitor,omitempty"`
}

// WarehouseStatus defines the observed state of a Warehouse.
type WarehouseStatus struct {
	CommonStatus `json:",inline"`

	// State is the current running state (STARTED, SUSPENDED, RESIZING).
	State string `json:"state,omitempty"`

	// ShowOutput stores the last observed SHOW WAREHOUSES output.
	ShowOutput *WarehouseShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which Snowflake parameters are actively managed.
	TrackedParameters []string `json:"trackedParameters,omitempty"`

	// LastAppliedResourceConstraint stores the last resource constraint value
	// applied to Snowflake. Because SHOW WAREHOUSES does not surface this
	// field, tracking it prevents unnecessary ALTER on every reconcile.
	LastAppliedResourceConstraint string `json:"lastAppliedResourceConstraint,omitempty"`
}

// Warehouse is the Schema for the warehouses API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="SIZE",type=string,JSONPath=`.spec.warehouseSize`
// +kubebuilder:printcolumn:name="STATE",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type Warehouse struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WarehouseSpec   `json:"spec,omitempty"`
	Status WarehouseStatus `json:"status,omitempty"`
}

// WarehouseList contains a list of Warehouse.
// +kubebuilder:object:root=true
type WarehouseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Warehouse `json:"items"`
}

// GetConditions returns the warehouse conditions.
func (w *Warehouse) GetConditions() []metav1.Condition { return w.Status.Conditions }

// SetConditions sets the warehouse conditions.
func (w *Warehouse) SetConditions(conditions []metav1.Condition) { w.Status.Conditions = conditions }

// GetDeletionPolicy returns the deletion policy.
func (w *Warehouse) GetDeletionPolicy() DeletionPolicy {
	if w.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return w.Spec.DeletionPolicy
}

// GetFullyQualifiedName returns the warehouse FQN.
func (w *Warehouse) GetFullyQualifiedName() string { return w.Status.FullyQualifiedName }

func init() { SchemeBuilder.Register(&Warehouse{}, &WarehouseList{}) }

// GetSpecName returns the Snowflake resource name from the spec.
func (w *Warehouse) GetSpecName() string { return w.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (w *Warehouse) GetProviderRef() ProviderReference { return w.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (w *Warehouse) GetUseRole() *string { return w.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (w *Warehouse) GetObservedGeneration() int64 { return w.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (w *Warehouse) SetObservedGeneration(v int64) { w.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (w *Warehouse) GetLastAppliedSpecHash() string { return w.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (w *Warehouse) SetLastAppliedSpecHash(v string) { w.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (w *Warehouse) GetTrackedParametersList() []string { return w.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (w *Warehouse) SetTrackedParametersList(v []string) { w.Status.TrackedParameters = v }

// GetOwner returns the use role from status.
func (w *Warehouse) GetOwner() string {
	if w.Status.ShowOutput != nil {
		return w.Status.ShowOutput.Owner
	}

	return ""
}

// ValidateSpec validates the resource spec.
func (w *Warehouse) ValidateSpec() error { return w.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (w *Warehouse) ComputeSpecHash() (string, error) { return ComputeSpecHash(w.Spec) }
