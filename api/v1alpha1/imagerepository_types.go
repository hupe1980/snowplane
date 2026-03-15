package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ImageRepositorySpec defines the desired state of a Snowpark Container Services Image Repository.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type ImageRepositorySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the image repository.
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
}

// ImageRepositoryShowOutput represents the Snowflake SHOW IMAGE REPOSITORIES output.
type ImageRepositoryShowOutput struct {
	CreatedOn     string `json:"createdOn,omitempty"`
	Name          string `json:"name,omitempty"`
	DatabaseName  string `json:"databaseName,omitempty"`
	SchemaName    string `json:"schemaName,omitempty"`
	RepositoryURL string `json:"repositoryUrl,omitempty"`
	Owner         string `json:"owner,omitempty"`
}

// ImageRepositoryStatus defines the observed state of a Snowpark Container Services Image Repository.
type ImageRepositoryStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput is the observed Snowflake SHOW IMAGE REPOSITORIES output.
	ShowOutput *ImageRepositoryShowOutput `json:"showOutput,omitempty"`

	// DatabaseName is the resolved database name for deletion fallback.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema name for deletion fallback.
	SchemaName string `json:"schemaName,omitempty"`

	// TrackedParameters lists the Snowflake parameters currently managed by this resource.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=imgrepo
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="REPOSITORY-URL",type=string,JSONPath=`.status.showOutput.repositoryUrl`,priority=1
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// ImageRepository is the Schema for the imagerepositories API (Snowpark Container Services).
type ImageRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ImageRepositorySpec   `json:"spec,omitempty"`
	Status            ImageRepositoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageRepositoryList contains a list of ImageRepository resources.
type ImageRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRepository{}, &ImageRepositoryList{})
}
