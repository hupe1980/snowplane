package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecondaryDatabaseSpec defines the desired state of a SecondaryDatabase.
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.asReplicaOf == oldSelf.asReplicaOf",message="spec.asReplicaOf is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type SecondaryDatabaseSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake database name for the secondary (replica) database.
	// Best practice: use the same name as the primary database. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// AsReplicaOf is the fully-qualified name of the primary database to replicate.
	// Format: "<organization_name>.<account_name>.<database_name>".
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=5
	// +kubebuilder:validation:Pattern=`^[^.]+\.[^.]+\.[^.]+$`
	AsReplicaOf string `json:"asReplicaOf"`

	// Comment is an optional description.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`

	// DataRetentionTimeInDays specifies the Time Travel retention period (0-90 days).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	DataRetentionTimeInDays *int32 `json:"dataRetentionTimeInDays,omitempty" snowflake:"DATA_RETENTION_TIME_IN_DAYS"`

	// MaxDataExtensionTimeInDays specifies the maximum number of days Snowflake
	// can extend the data retention period.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=90
	MaxDataExtensionTimeInDays *int32 `json:"maxDataExtensionTimeInDays,omitempty" snowflake:"MAX_DATA_EXTENSION_TIME_IN_DAYS"`
}

// SecondaryDatabaseShowOutput mirrors the SHOW DATABASES output for a secondary database.
type SecondaryDatabaseShowOutput struct {
	// CreatedOn is the timestamp when the database was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the database name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Kind is the database kind (e.g. STANDARD).
	Kind string `json:"kind,omitempty"`

	// Comment is the database description.
	Comment string `json:"comment,omitempty"`

	// Owner is the role that owns the database.
	Owner string `json:"owner,omitempty"`

	// RetentionTime is the data retention time in days.
	RetentionTime int32 `json:"retentionTime,omitempty"`

	// Origin is the primary database identifier (org.account.db_name).
	Origin string `json:"origin,omitempty"`
}

// SecondaryDatabaseStatus defines the observed state of a SecondaryDatabase.
type SecondaryDatabaseStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW DATABASES output for this database.
	ShowOutput *SecondaryDatabaseShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET
	// in Snowflake. When a previously-managed field is removed from the spec,
	// the reconciler issues ALTER ... UNSET to revert to the server default.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// SecondaryDatabase is the Schema for the secondarydatabases API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=sdb
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="REPLICA-OF",type=string,JSONPath=`.spec.asReplicaOf`,priority=1
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type SecondaryDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecondaryDatabaseSpec   `json:"spec,omitempty"`
	Status SecondaryDatabaseStatus `json:"status,omitempty"`
}

// SecondaryDatabaseList contains a list of SecondaryDatabase.
// +kubebuilder:object:root=true
type SecondaryDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecondaryDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecondaryDatabase{}, &SecondaryDatabaseList{})
}
