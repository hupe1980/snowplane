package v1alpha1

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FunctionPythonSpec defines the desired state of a Snowflake Python User-Defined Function.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.arguments == oldSelf.arguments",message="spec.arguments is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.returns == oldSelf.returns",message="spec.returns is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type FunctionPythonSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake function name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SchemaName *string `json:"schemaName,omitempty"`

	// Arguments defines the function arguments. Immutable after creation.
	// +optional
	Arguments []CallableArgument `json:"arguments,omitempty"`

	// Returns is the return type (e.g. "VARCHAR", "TABLE(col1 VARCHAR, col2 NUMBER)").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Returns string `json:"returns"`

	// Handler is the fully qualified Python handler function (e.g. "my_module.my_function").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Handler string `json:"handler"`

	// RuntimeVersion is the Python runtime version (e.g. "3.8", "3.11").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	RuntimeVersion string `json:"runtimeVersion"`

	// SnowparkPackage is the Snowpark package spec (e.g. "snowflake-snowpark-python").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SnowparkPackage string `json:"snowparkPackage"`

	// Body is the Python source code.
	// Optional — code can be deployed via stage imports instead.
	// +optional
	Body *string `json:"body,omitempty"`

	// Packages lists additional packages (e.g. "numpy", "pandas").
	// +optional
	Packages []string `json:"packages,omitempty"`

	// Imports lists stage file paths to import (e.g. "@my_stage/my_module.py").
	// +optional
	Imports []string `json:"imports,omitempty"`

	// ExternalAccessIntegrations lists external access integration names.
	// +optional
	ExternalAccessIntegrations []string `json:"externalAccessIntegrations,omitempty"`

	// Secrets binds Snowflake secrets to variables accessible from handler code.
	// +optional
	Secrets []SecretBinding `json:"secrets,omitempty"`

	// NullInputBehavior specifies how the function handles NULL arguments.
	// +optional
	// +kubebuilder:validation:Enum="CALLED ON NULL INPUT";"RETURNS NULL ON NULL INPUT";STRICT
	NullInputBehavior *string `json:"nullInputBehavior,omitempty"`

	// Volatility specifies the function volatility: VOLATILE or IMMUTABLE.
	// +optional
	// +kubebuilder:validation:Enum=VOLATILE;IMMUTABLE
	Volatility *string `json:"volatility,omitempty"`

	// Secure marks the function as secure.
	// +optional
	Secure bool `json:"secure,omitempty"`

	// Comment is an optional description for the function.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// Validate checks the FunctionPythonSpec for consistency.
func (s *FunctionPythonSpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, fmt.Errorf("spec.name is required"))
	}

	if s.Returns == "" {
		errs = append(errs, fmt.Errorf("spec.returns is required"))
	}

	if s.Handler == "" {
		errs = append(errs, fmt.Errorf("spec.handler is required for Python functions"))
	}

	if s.RuntimeVersion == "" {
		errs = append(errs, fmt.Errorf("spec.runtimeVersion is required for Python functions"))
	}

	if s.SnowparkPackage == "" {
		errs = append(errs, fmt.Errorf("spec.snowparkPackage is required for Python functions"))
	}

	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// FunctionPythonStatus defines the observed state of a FunctionPython.
type FunctionPythonStatus struct {
	CommonStatus      `json:",inline"`
	DatabaseName      string              `json:"databaseName,omitempty"`
	SchemaName        string              `json:"schemaName,omitempty"`
	ShowOutput        *FunctionShowOutput `json:"showOutput,omitempty"`
	TrackedParameters []string            `json:"trackedParameters,omitempty"`
}

// FunctionPython is the Schema for Python user-defined functions.
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
type FunctionPython struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FunctionPythonSpec   `json:"spec,omitempty"`
	Status FunctionPythonStatus `json:"status,omitempty"`
}

// FunctionPythonList contains a list of FunctionPython.
// +kubebuilder:object:root=true
type FunctionPythonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FunctionPython `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FunctionPython{}, &FunctionPythonList{})
}
