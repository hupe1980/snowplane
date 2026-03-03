package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaskingPolicyApplicationSpec defines the desired state of a MaskingPolicyApplication.
// Applies a Snowflake masking policy to a table column:
//
//	ALTER TABLE <table_name> ALTER COLUMN <column_name> SET MASKING POLICY <policy_name>
//
// Identity fields (policyName/policyRef, tableName, columnName) are
// immutable after creation — changing any of them requires deleting and
// recreating the resource.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.policyName) == has(self.policyName) && (!has(self.policyName) || self.policyName == oldSelf.policyName)",message="spec.policyName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.policyRef) == has(self.policyRef) && (!has(self.policyRef) || self.policyRef == oldSelf.policyRef)",message="spec.policyRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.tableName == oldSelf.tableName",message="spec.tableName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.columnName == oldSelf.columnName",message="spec.columnName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.usingColumns) == has(self.usingColumns) && (!has(self.usingColumns) || self.usingColumns == oldSelf.usingColumns)",message="spec.usingColumns is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.policyName) && !has(self.policyRef)) || (!has(self.policyName) && has(self.policyRef))",message="exactly one of spec.policyName or spec.policyRef must be set"
type MaskingPolicyApplicationSpec struct {
	CommonSpec `json:",inline"`

	// PolicyName is the fully qualified Snowflake masking policy name
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY").
	// Mutually exclusive with PolicyRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	PolicyName *string `json:"policyName,omitempty"`

	// PolicyRef references a MaskingPolicy CR in the same namespace.
	// When set, the policy name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with PolicyName.
	// +optional
	PolicyRef *LocalObjectReference `json:"policyRef,omitempty"`

	// TableName is the fully qualified Snowflake table name
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_TABLE").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	TableName string `json:"tableName"`

	// ColumnName is the column in the table to apply the masking policy to.
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ColumnName string `json:"columnName"`

	// UsingColumns is an optional list of column names for a conditional masking policy.
	// The first column specifies the column for the policy conditions.
	// Additional columns evaluate to determine masking in each row.
	// Immutable after creation.
	// +optional
	UsingColumns []string `json:"usingColumns,omitempty"`
}

// MaskingPolicyApplicationStatus defines the observed state of a MaskingPolicyApplication.
type MaskingPolicyApplicationStatus struct {
	CommonStatus `json:",inline"`

	// PolicyName is the resolved fully qualified masking policy name.
	PolicyName string `json:"policyName,omitempty"`

	// ObservedPolicyName is the masking policy currently applied in Snowflake,
	// as read from POLICY_REFERENCES.
	ObservedPolicyName string `json:"observedPolicyName,omitempty"`
}

// MaskingPolicyApplication is the Schema for the maskingpolicyapplications API.
// It applies a Snowflake masking policy to a table column.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mpa,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="POLICY",type=string,JSONPath=`.status.policyName`,priority=0
// +kubebuilder:printcolumn:name="TABLE",type=string,JSONPath=`.spec.tableName`,priority=0
// +kubebuilder:printcolumn:name="COLUMN",type=string,JSONPath=`.spec.columnName`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type MaskingPolicyApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaskingPolicyApplicationSpec   `json:"spec,omitempty"`
	Status MaskingPolicyApplicationStatus `json:"status,omitempty"`
}

// MaskingPolicyApplicationList contains a list of MaskingPolicyApplication.
// +kubebuilder:object:root=true
type MaskingPolicyApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaskingPolicyApplication `json:"items"`
}

// GetSpecName returns a human-readable composite name for the application.
func (r *MaskingPolicyApplication) GetSpecName() string {
	var policy string
	if r.Spec.PolicyName != nil {
		policy = *r.Spec.PolicyName
	} else if r.Spec.PolicyRef != nil {
		policy = fmt.Sprintf("ref:%s", r.Spec.PolicyRef.Name)
	}

	return fmt.Sprintf("%s->%s.%s", policy, r.Spec.TableName, r.Spec.ColumnName)
}

func init() {
	SchemeBuilder.Register(&MaskingPolicyApplication{}, &MaskingPolicyApplicationList{})
}
