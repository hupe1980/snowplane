package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PrimaryConnectionSpec defines the desired state of a Snowflake Primary Connection.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type PrimaryConnectionSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake connection name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// EnableFailoverToAccounts is the list of accounts enabled for failover.
	// Each entry is in the form "orgName.accountName".
	// +optional
	EnableFailoverToAccounts []string `json:"enableFailoverToAccounts,omitempty" snowflake:"ENABLE_FAILOVER_TO_ACCOUNTS"`

	// Comment is an optional comment for the connection.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// PrimaryConnectionShowOutput holds the values from SHOW CONNECTIONS.
type PrimaryConnectionShowOutput struct {
	CreatedOn         string `json:"createdOn,omitempty"`
	Name              string `json:"name,omitempty"`
	SnowflakeRegion   string `json:"snowflakeRegion,omitempty"`
	AccountName       string `json:"accountName,omitempty"`
	OrganizationName  string `json:"organizationName,omitempty"`
	ConnectionURL     string `json:"connectionUrl,omitempty"`
	IsPrimary         bool   `json:"isPrimary,omitempty"`
	FailoverAllowedTo string `json:"failoverAllowedTo,omitempty"`
	Comment           string `json:"comment,omitempty"`
}

// PrimaryConnectionStatus defines the observed state of PrimaryConnection.
type PrimaryConnectionStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *PrimaryConnectionShowOutput `json:"showOutput,omitempty"`
	TrackedParameters []string                     `json:"trackedParameters,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pconn,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=`.status.conditions[?(@.type=='Synced')].status`
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".status.fullyQualifiedName"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

// PrimaryConnection is the Schema for the Snowflake Primary Connection API.
type PrimaryConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PrimaryConnectionSpec   `json:"spec,omitempty"`
	Status            PrimaryConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PrimaryConnectionList contains a list of PrimaryConnection.
type PrimaryConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PrimaryConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PrimaryConnection{}, &PrimaryConnectionList{})
}
