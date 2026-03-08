package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ShareSpec defines the desired state of a Snowflake Share.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable"
type ShareSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake identifier for the share.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Comment is an optional description for the share.
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// Accounts is the list of consumer accounts that can access this share.
	// Each entry must be a fully qualified account identifier (e.g., "ORG.ACCOUNT").
	// +optional
	Accounts []string `json:"accounts,omitempty"`
}

// ShareShowOutput represents the Snowflake SHOW SHARES output for a share.
type ShareShowOutput struct {
	CreatedOn    string `json:"createdOn,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Name         string `json:"name,omitempty"`
	DatabaseName string `json:"databaseName,omitempty"`
	To           string `json:"to,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Comment      string `json:"comment,omitempty" snowflake:"COMMENT"`
	ListingType  string `json:"listingType,omitempty"`
}

// ShareStatus defines the observed state of a Snowflake Share.
type ShareStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput is the observed Snowflake SHOW SHARES output.
	ShowOutput *ShareShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters lists the Snowflake parameters currently managed by this resource.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=shr
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// Share is the Schema for the shares API.
type Share struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ShareSpec   `json:"spec,omitempty"`
	Status            ShareStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ShareList contains a list of Share resources.
type ShareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Share `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Share{}, &ShareList{})
}
