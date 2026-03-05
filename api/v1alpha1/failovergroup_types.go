package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FailoverGroupSpec defines the desired state of a Snowflake Failover Group (primary).
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type FailoverGroupSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake failover group name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// ObjectTypes is the list of object types to include in the failover group.
	// Valid values: ACCOUNT PARAMETERS, DATABASES, INTEGRATIONS, NETWORK POLICIES,
	// RESOURCE MONITORS, ROLES, SHARES, USERS, WAREHOUSES.
	// +kubebuilder:validation:MinItems=1
	ObjectTypes []string `json:"objectTypes" snowflake:"OBJECT_TYPES,nounset"`

	// AllowedAccounts is the list of target accounts for replication/failover
	// in the format "org_name.account_name".
	// +kubebuilder:validation:MinItems=1
	AllowedAccounts []string `json:"allowedAccounts" snowflake:"ALLOWED_ACCOUNTS,nounset"`

	// AllowedDatabases is a list of databases to include when DATABASES is in ObjectTypes.
	// +optional
	AllowedDatabases []string `json:"allowedDatabases,omitempty" snowflake:"ALLOWED_DATABASES"`

	// AllowedShares is a list of shares to include when SHARES is in ObjectTypes.
	// +optional
	AllowedShares []string `json:"allowedShares,omitempty" snowflake:"ALLOWED_SHARES"`

	// AllowedIntegrationTypes is a list of integration types to include when INTEGRATIONS is in ObjectTypes.
	// Valid values: SECURITY INTEGRATIONS, API INTEGRATIONS, STORAGE INTEGRATIONS, NOTIFICATION INTEGRATIONS.
	// +optional
	AllowedIntegrationTypes []string `json:"allowedIntegrationTypes,omitempty" snowflake:"ALLOWED_INTEGRATION_TYPES"`

	// IgnoreEditionCheck allows replication to accounts of lower editions.
	// +optional
	IgnoreEditionCheck *bool `json:"ignoreEditionCheck,omitempty"`

	// ReplicationSchedule is the replication schedule (e.g., "10 MINUTE" or "USING CRON 0 */4 * * * UTC").
	// +optional
	ReplicationSchedule *string `json:"replicationSchedule,omitempty" snowflake:"REPLICATION_SCHEDULE"`

	// ErrorIntegration is the name of a notification integration for refresh error alerts.
	// +optional
	ErrorIntegration *string `json:"errorIntegration,omitempty" snowflake:"ERROR_INTEGRATION"`

	// Comment is an optional description for the failover group.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// FailoverGroupShowOutput mirrors the SHOW FAILOVER GROUPS output.
type FailoverGroupShowOutput struct {
	// CreatedOn is the timestamp when the failover group was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the failover group name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the group type (FAILOVER).
	Type string `json:"type,omitempty"`

	// Comment is the failover group description.
	Comment string `json:"comment,omitempty"`

	// IsPrimary indicates whether this is the primary failover group.
	IsPrimary bool `json:"isPrimary,omitempty"`

	// Primary is the fully qualified name of the primary group (org.account.name).
	Primary string `json:"primary,omitempty"`

	// ObjectTypes is the comma-separated list of replicated object types.
	ObjectTypes string `json:"objectTypes,omitempty"`

	// AllowedAccounts is the comma-separated list of target accounts.
	AllowedAccounts string `json:"allowedAccounts,omitempty"`

	// ReplicationSchedule is the configured schedule, if any.
	ReplicationSchedule string `json:"replicationSchedule,omitempty"`

	// Owner is the role with OWNERSHIP on the failover group.
	Owner string `json:"owner,omitempty"`
}

// FailoverGroupStatus defines the observed state of a FailoverGroup.
type FailoverGroupStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW FAILOVER GROUPS output.
	ShowOutput *FailoverGroupShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// FailoverGroup is the Schema for the failovergroups API.
// It manages a Snowflake Failover Group (primary) for cross-account replication and failover.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=fg
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type FailoverGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FailoverGroupSpec   `json:"spec,omitempty"`
	Status FailoverGroupStatus `json:"status,omitempty"`
}

// FailoverGroupList contains a list of FailoverGroup.
// +kubebuilder:object:root=true
type FailoverGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FailoverGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FailoverGroup{}, &FailoverGroupList{})
}
