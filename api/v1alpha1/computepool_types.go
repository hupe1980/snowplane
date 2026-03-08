package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ComputePoolSpec defines the desired state of a Snowpark Container Services Compute Pool.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"
type ComputePoolSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the compute pool.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// MinNodes is the minimum number of nodes in the compute pool.
	// +kubebuilder:validation:Minimum=1
	MinNodes int32 `json:"minNodes"`

	// MaxNodes is the maximum number of nodes in the compute pool.
	// +kubebuilder:validation:Minimum=1
	MaxNodes int32 `json:"maxNodes"`

	// InstanceFamily is the Snowflake compute instance family (e.g., CPU_X64_XS, GPU_NV_S).
	// +kubebuilder:validation:MinLength=1
	InstanceFamily string `json:"instanceFamily"`

	// AutoResume specifies whether to automatically resume the compute pool when a service or job is submitted.
	// +optional
	AutoResume *bool `json:"autoResume,omitempty" snowflake:"AUTO_RESUME"`

	// AutoSuspendSecs is the number of idle seconds before the compute pool is automatically suspended.
	// +kubebuilder:validation:Minimum=0
	// +optional
	AutoSuspendSecs *int32 `json:"autoSuspendSecs,omitempty" snowflake:"AUTO_SUSPEND_SECS"`

	// Comment is an optional description.
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ComputePoolShowOutput represents the Snowflake SHOW COMPUTE POOLS output.
type ComputePoolShowOutput struct {
	CreatedOn      string `json:"createdOn,omitempty"`
	Name           string `json:"name,omitempty"`
	State          string `json:"state,omitempty"`
	MinNodes       int32  `json:"minNodes,omitempty"`
	MaxNodes       int32  `json:"maxNodes,omitempty"`
	InstanceFamily string `json:"instanceFamily,omitempty"`
	NumServices    int32  `json:"numServices,omitempty"`
	NumJobs        int32  `json:"numJobs,omitempty"`
	AutoResume     string `json:"autoResume,omitempty"`
	AutoSuspend    int32  `json:"autoSuspendSecs,omitempty"`
	ActiveNodes    int32  `json:"activeNodes,omitempty"`
	IdleNodes      int32  `json:"idleNodes,omitempty"`
	Owner          string `json:"owner,omitempty"`
	Comment        string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ComputePoolStatus defines the observed state of a Snowpark Container Services Compute Pool.
type ComputePoolStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput is the observed Snowflake SHOW COMPUTE POOLS output.
	ShowOutput *ComputePoolShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters lists the Snowflake parameters currently managed by this resource.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=cp
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="STATE",type=string,JSONPath=`.status.showOutput.state`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// ComputePool is the Schema for the computepools API.
type ComputePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ComputePoolSpec   `json:"spec,omitempty"`
	Status            ComputePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComputePoolList contains a list of ComputePool resources.
type ComputePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComputePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComputePool{}, &ComputePoolList{})
}
