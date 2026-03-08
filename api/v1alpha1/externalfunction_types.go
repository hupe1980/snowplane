package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ExternalFunctionCompression defines the compression format for the external function.
type ExternalFunctionCompression string

// ExternalFunctionCompression constants define supported compression formats.
const (
	ExternalFunctionCompressionAuto    ExternalFunctionCompression = "AUTO"
	ExternalFunctionCompressionGZIP    ExternalFunctionCompression = "GZIP"
	ExternalFunctionCompressionDeflate ExternalFunctionCompression = "DEFLATE"
	ExternalFunctionCompressionNone    ExternalFunctionCompression = "NONE"
)

// ExternalFunctionArg defines a function argument.
type ExternalFunctionArg struct {
	// Name is the argument name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type is the Snowflake data type of the argument.
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
}

// ExternalFunctionHeader defines an HTTP header.
type ExternalFunctionHeader struct {
	// Name is the header name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the header value.
	Value string `json:"value"`
}

// ExternalFunctionSpec defines the desired state of a Snowflake External Function.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"
type ExternalFunctionSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the external function.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseName is the name of the Snowflake database containing the function.
	// +optional
	DatabaseName *string `json:"databaseName,omitempty"`

	// DatabaseRef is a reference to a Snowplane Database resource.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// SchemaName is the name of the Snowflake schema containing the function.
	// +optional
	SchemaName *string `json:"schemaName,omitempty"`

	// SchemaRef is a reference to a Snowplane Schema resource.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// Args is the list of function arguments.
	// +optional
	Args []ExternalFunctionArg `json:"args,omitempty"`

	// ReturnType is the Snowflake return data type.
	// +kubebuilder:validation:MinLength=1
	ReturnType string `json:"returnType"`

	// ReturnNullValues indicates if the function can return NULL.
	// +kubebuilder:default=true
	// +optional
	ReturnNullValues *bool `json:"returnNullValues,omitempty"`

	// ReturnBehavior specifies whether the function is volatile or immutable.
	// +optional
	ReturnBehavior *string `json:"returnBehavior,omitempty"`

	// APIIntegrationName is the name of the API integration.
	// +optional
	APIIntegrationName *string `json:"apiIntegrationName,omitempty"`

	// APIIntegrationRef is a reference to a Snowplane APIIntegration resource.
	// +optional
	APIIntegrationRef *ObjectReference `json:"apiIntegrationRef,omitempty"`

	// URL is the HTTPS endpoint invoked by the external function.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// Headers are optional HTTP headers sent with each request.
	// +optional
	Headers []ExternalFunctionHeader `json:"headers,omitempty"`

	// MaxBatchRows limits the number of rows per batch request.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxBatchRows *int32 `json:"maxBatchRows,omitempty"`

	// Compression specifies the request/response compression format.
	// +kubebuilder:validation:Enum=AUTO;GZIP;DEFLATE;NONE
	// +optional
	Compression *ExternalFunctionCompression `json:"compression,omitempty"`

	// RequestTranslator is the fully qualified name of a request translator function.
	// +optional
	RequestTranslator *string `json:"requestTranslator,omitempty"`

	// ResponseTranslator is the fully qualified name of a response translator function.
	// +optional
	ResponseTranslator *string `json:"responseTranslator,omitempty"`

	// Comment is an optional description.
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// ExternalFunctionShowOutput represents the Snowflake SHOW EXTERNAL FUNCTIONS output.
type ExternalFunctionShowOutput struct {
	CreatedOn      string `json:"createdOn,omitempty"`
	Name           string `json:"name,omitempty"`
	SchemaName     string `json:"schemaName,omitempty"`
	DatabaseName   string `json:"databaseName,omitempty"`
	Language       string `json:"language,omitempty"`
	IsExternalFunc string `json:"isExternalFunction,omitempty"`
	Arguments      string `json:"arguments,omitempty"`
	Description    string `json:"description,omitempty"`
}

// ExternalFunctionStatus defines the observed state of a Snowflake External Function.
type ExternalFunctionStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput is the observed Snowflake SHOW output.
	ShowOutput *ExternalFunctionShowOutput `json:"showOutput,omitempty"`

	// DatabaseName is the resolved database name for deletion fallback.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema name for deletion fallback.
	SchemaName string `json:"schemaName,omitempty"`

	// APIIntegrationName is the resolved API integration name.
	APIIntegrationName string `json:"apiIntegrationName,omitempty"`

	// URL is the cached HTTPS endpoint URL for immutable field validation.
	URL string `json:"url,omitempty"`

	// TrackedParameters lists the Snowflake parameters currently managed by this resource.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=extfn
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// ExternalFunction is the Schema for the externalfunctions API.
type ExternalFunction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ExternalFunctionSpec   `json:"spec,omitempty"`
	Status            ExternalFunctionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ExternalFunctionList contains a list of ExternalFunction resources.
type ExternalFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExternalFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ExternalFunction{}, &ExternalFunctionList{})
}
