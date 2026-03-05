package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkRuleType specifies the type of network identifiers in the rule.
// +kubebuilder:validation:Enum=IPV4;AWSVPCEID;AZURELINKID;GCPPSCID;HOST_PORT;PRIVATE_HOST_PORT
type NetworkRuleType string

// Valid NetworkRuleType values.
const (
	NetworkRuleTypeIPV4            NetworkRuleType = "IPV4"
	NetworkRuleTypeAWSVPCEID       NetworkRuleType = "AWSVPCEID"
	NetworkRuleTypeAzureLinkID     NetworkRuleType = "AZURELINKID"
	NetworkRuleTypeGCPPSCID        NetworkRuleType = "GCPPSCID"
	NetworkRuleTypeHostPort        NetworkRuleType = "HOST_PORT"
	NetworkRuleTypePrivateHostPort NetworkRuleType = "PRIVATE_HOST_PORT"
)

// NetworkRuleMode specifies the access mode of the network rule.
// +kubebuilder:validation:Enum=INGRESS;INTERNAL_STAGE;EGRESS
type NetworkRuleMode string

// Valid NetworkRuleMode values.
const (
	NetworkRuleModeIngress       NetworkRuleMode = "INGRESS"
	NetworkRuleModeInternalStage NetworkRuleMode = "INTERNAL_STAGE"
	NetworkRuleModeEgress        NetworkRuleMode = "EGRESS"
)

// NetworkRuleSpec defines the desired state of a Snowflake Network Rule.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.mode == oldSelf.mode",message="spec.mode is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type NetworkRuleSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake network rule name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a managed Database resource for the parent database.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseRef is immutable"
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.databaseName is immutable"
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a managed Schema resource for the parent schema.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaRef is immutable"
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// The controller constructs the FQN from databaseName + schemaName + name.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.schemaName is immutable"
	SchemaName *string `json:"schemaName,omitempty"`

	// Type specifies the kind of network identifiers (IPV4, AWSVPCEID, AZURELINKID, GCPPSCID, HOST_PORT, PRIVATE_HOST_PORT).
	// Immutable after creation.
	Type NetworkRuleType `json:"type"`

	// Mode specifies the access mode (INGRESS, INTERNAL_STAGE, EGRESS).
	// Immutable after creation.
	Mode NetworkRuleMode `json:"mode"`

	// ValueList is the list of network identifiers for this rule.
	// +kubebuilder:validation:MinItems=1
	ValueList []string `json:"valueList" snowflake:"VALUE_LIST,always"`

	// Comment is an optional description for the network rule.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// NetworkRuleShowOutput mirrors the SHOW NETWORK RULES output stored in status.
type NetworkRuleShowOutput struct {
	// CreatedOn is the timestamp when the rule was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the rule name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the database containing the rule.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the schema containing the rule.
	SchemaName string `json:"schemaName,omitempty"`

	// Owner is the role that owns the network rule.
	Owner string `json:"owner,omitempty"`

	// Type is the network rule type.
	Type string `json:"type,omitempty"`

	// Mode is the network rule mode.
	Mode string `json:"mode,omitempty"`

	// Comment is the rule description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// NetworkRuleStatus defines the observed state of a NetworkRule.
type NetworkRuleStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the resolved database fully-qualified name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the resolved schema fully-qualified name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW NETWORK RULES output for this rule.
	ShowOutput *NetworkRuleShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// NetworkRule is the Schema for the networkrules API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="MODE",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type NetworkRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkRuleSpec   `json:"spec,omitempty"`
	Status NetworkRuleStatus `json:"status,omitempty"`
}

// NetworkRuleList contains a list of NetworkRule.
// +kubebuilder:object:root=true
type NetworkRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkRule{}, &NetworkRuleList{})
}
