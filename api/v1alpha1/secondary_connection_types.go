package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SecondaryConnectionSpec defines the desired state of a Snowflake Secondary Connection.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.asReplicaOf == oldSelf.asReplicaOf",message="spec.asReplicaOf is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type SecondaryConnectionSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake connection name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// AsReplicaOf is the fully qualified name of the primary connection to replicate
	// (e.g., "orgName.accountName.connectionName"). Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	AsReplicaOf string `json:"asReplicaOf"`

	// Comment is an optional comment for the connection.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// SecondaryConnectionShowOutput holds the values from SHOW CONNECTIONS.
type SecondaryConnectionShowOutput struct {
	CreatedOn        string `json:"createdOn,omitempty"`
	Name             string `json:"name,omitempty"`
	SnowflakeRegion  string `json:"snowflakeRegion,omitempty"`
	AccountName      string `json:"accountName,omitempty"`
	OrganizationName string `json:"organizationName,omitempty"`
	ConnectionURL    string `json:"connectionUrl,omitempty"`
	IsPrimary        bool   `json:"isPrimary,omitempty"`
	PrimaryName      string `json:"primaryName,omitempty"`
	Comment          string `json:"comment,omitempty"`
}

// SecondaryConnectionStatus defines the observed state of SecondaryConnection.
type SecondaryConnectionStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *SecondaryConnectionShowOutput `json:"showOutput,omitempty"`
	TrackedParameters []string                       `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sconn,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=`.status.conditions[?(@.type=='Synced')].status`
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".status.fullyQualifiedName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// SecondaryConnection is the Schema for the Snowflake Secondary Connection API.
type SecondaryConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SecondaryConnectionSpec   `json:"spec,omitempty"`
	Status            SecondaryConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecondaryConnectionList contains a list of SecondaryConnection.
type SecondaryConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecondaryConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecondaryConnection{}, &SecondaryConnectionList{})
}
