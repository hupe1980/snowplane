package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ServiceSpec defines the desired state of a Snowpark Container Services Service.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"
type ServiceSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the service.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseName is the name of the Snowflake database.
	// +optional
	DatabaseName *string `json:"databaseName,omitempty"`

	// DatabaseRef is a reference to a Snowplane Database resource.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// SchemaName is the name of the Snowflake schema.
	// +optional
	SchemaName *string `json:"schemaName,omitempty"`

	// SchemaRef is a reference to a Snowplane Schema resource.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// ComputePoolName is the name of the compute pool.
	// +optional
	ComputePoolName *string `json:"computePoolName,omitempty"`

	// ComputePoolRef is a reference to a Snowplane ComputePool resource.
	// +optional
	ComputePoolRef *ObjectReference `json:"computePoolRef,omitempty"`

	// Specification is the inline YAML/JSON service specification.
	// +kubebuilder:validation:MaxLength=65536
	// +optional
	Specification string `json:"specification,omitempty"`

	// SpecificationReference is a stage path to the service specification file
	// (e.g., "@my_db.my_schema.my_stage/spec.yaml").
	// +optional
	SpecificationReference string `json:"specificationReference,omitempty"`

	// MinInstances is the minimum number of service compute instances.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MinInstances *int32 `json:"minInstances,omitempty" snowflake:"MIN_INSTANCES"`

	// MaxInstances is the maximum number of service compute instances.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxInstances *int32 `json:"maxInstances,omitempty" snowflake:"MAX_INSTANCES"`

	// AutoResume specifies whether to automatically resume the service.
	// +optional
	AutoResume *bool `json:"autoResume,omitempty" snowflake:"AUTO_RESUME"`

	// ExternalAccessIntegrations lists the security integrations that grant egress access.
	// +optional
	ExternalAccessIntegrations []string `json:"externalAccessIntegrations,omitempty"`

	// Comment is an optional description.
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ServiceShowOutput represents the Snowflake SHOW SERVICES output.
type ServiceShowOutput struct {
	CreatedOn      string `json:"createdOn,omitempty"`
	Name           string `json:"name,omitempty"`
	DatabaseName   string `json:"databaseName,omitempty"`
	SchemaName     string `json:"schemaName,omitempty"`
	Owner          string `json:"owner,omitempty"`
	ComputePool    string `json:"computePool,omitempty"`
	Status         string `json:"status,omitempty"`
	MinInstances   int32  `json:"minInstances,omitempty"`
	MaxInstances   int32  `json:"maxInstances,omitempty"`
	AutoResume     string `json:"autoResume,omitempty"`
	ResumeAt       string `json:"resumeAt,omitempty"`
	QueryWarehouse string `json:"queryWarehouse,omitempty"`
	Comment        string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ServiceStatus defines the observed state of a Snowpark Container Services Service.
type ServiceStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput is the observed Snowflake SHOW SERVICES output.
	ShowOutput *ServiceShowOutput `json:"showOutput,omitempty"`

	// DatabaseName is the resolved database name for deletion fallback.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema name for deletion fallback.
	SchemaName string `json:"schemaName,omitempty"`

	// ComputePoolName is the resolved compute pool name.
	ComputePoolName string `json:"computePoolName,omitempty"`

	// TrackedParameters lists the Snowflake parameters currently managed by this resource.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=svc
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.showOutput.status`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// Service is the Schema for the services API (Snowpark Container Services).
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceSpec   `json:"spec,omitempty"`
	Status            ServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceList contains a list of Service resources.
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
