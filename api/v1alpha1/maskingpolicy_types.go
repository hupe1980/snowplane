package v1alpha1

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MaskingPolicyArgument defines an argument in a masking policy signature.
type MaskingPolicyArgument struct {
	// Name is the argument name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type is the Snowflake data type (e.g. VARCHAR, NUMBER, TIMESTAMP_NTZ).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Type string `json:"type"`
}

// MaskingPolicySpec defines the desired state of a Snowflake Masking Policy.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
// +kubebuilder:validation:XValidation:rule="!self.body.contains(';') && !self.body.upperAscii().contains('SYSTEM$') && !self.body.upperAscii().contains('EXECUTE IMMEDIATE') && !self.body.upperAscii().contains('CALL ') && !self.body.upperAscii().contains('CREATE ') && !self.body.upperAscii().contains('ALTER ') && !self.body.upperAscii().contains('DROP ') && !self.body.upperAscii().contains('GRANT ') && !self.body.upperAscii().contains('REVOKE ') && !self.body.upperAscii().contains('INSERT ') && !self.body.upperAscii().contains('UPDATE ') && !self.body.upperAscii().contains('DELETE ') && !self.body.upperAscii().contains('MERGE ') && !self.body.upperAscii().contains('COPY INTO') && !self.body.upperAscii().contains('PUT ') && !self.body.upperAscii().contains('GET ') && !self.body.upperAscii().contains('REMOVE ')",message="spec.body contains a blocked SQL pattern (potential privilege escalation)"
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRef) == has(self.databaseRef) && (!has(self.databaseRef) || self.databaseRef == oldSelf.databaseRef)",message="spec.databaseRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseName) == has(self.databaseName) && (!has(self.databaseName) || self.databaseName == oldSelf.databaseName)",message="spec.databaseName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaRef) == has(self.schemaRef) && (!has(self.schemaRef) || self.schemaRef == oldSelf.schemaRef)",message="spec.schemaRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.schemaName) == has(self.schemaName) && (!has(self.schemaName) || self.schemaName == oldSelf.schemaName)",message="spec.schemaName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.signature == oldSelf.signature",message="spec.signature is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="!has(self.databaseName) || !self.databaseName.contains('.')",message="spec.databaseName must be a simple identifier, not a fully-qualified name"
// +kubebuilder:validation:XValidation:rule="!has(self.schemaName) || !self.schemaName.contains('.')",message="spec.schemaName must be a simple identifier, not a fully-qualified name; use spec.databaseName for the database part"
type MaskingPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake masking policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *ObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the Snowflake database identifier (e.g. "ANALYTICS").
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *ObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the Snowflake schema identifier (e.g. "PUBLIC").
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SchemaName *string `json:"schemaName,omitempty"`

	// Signature defines the arguments for the masking policy.
	// The first argument is the column to mask; its type becomes the return type.
	// Immutable after creation.
	// +kubebuilder:validation:MinItems=1
	Signature []MaskingPolicyArgument `json:"signature"`

	// Body is the SQL expression that transforms the data.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:MaxLength=65536
	Body string `json:"body" snowflake:"BODY,always"`

	// ExemptOtherPolicies specifies whether other policies can reference a masked column.
	// Can only be set at creation time and cannot be altered. Use CREATE OR REPLACE to change.
	// +optional
	ExemptOtherPolicies *bool `json:"exemptOtherPolicies,omitempty" snowflake:"EXEMPT_OTHER_POLICIES,nounset"`

	// Comment is an optional description for the masking policy.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// Validate checks the MaskingPolicySpec for consistency.
func (s *MaskingPolicySpec) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, fmt.Errorf("spec.name is required"))
	}

	if len(s.Signature) == 0 {
		errs = append(errs, fmt.Errorf("spec.signature requires at least one argument"))
	}

	if s.Body == "" {
		errs = append(errs, fmt.Errorf("spec.body is required"))
	}

	// Exactly one of databaseRef or databaseName must be set.
	if err := validateDatabaseSource(s.DatabaseRef, s.DatabaseName); err != nil {
		errs = append(errs, err)
	}

	// Exactly one of schemaRef or schemaName must be set.
	if err := validateSchemaSource(s.SchemaRef, s.SchemaName); err != nil {
		errs = append(errs, err)
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// MaskingPolicyShowOutput mirrors the SHOW MASKING POLICIES output stored in status.
type MaskingPolicyShowOutput struct {
	// CreatedOn is the timestamp when the policy was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the policy name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Kind is the policy kind (MASKING_POLICY).
	Kind string `json:"kind,omitempty"`

	// Owner is the role that owns the masking policy.
	Owner string `json:"owner,omitempty"`

	// Comment is the policy description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// MaskingPolicyStatus defines the observed state of a MaskingPolicy.
type MaskingPolicyStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW MASKING POLICIES output for this policy.
	ShowOutput *MaskingPolicyShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// MaskingPolicy is the Schema for the maskingpolicies API.
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
type MaskingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MaskingPolicySpec   `json:"spec,omitempty"`
	Status MaskingPolicyStatus `json:"status,omitempty"`
}

// MaskingPolicyList contains a list of MaskingPolicy.
// +kubebuilder:object:root=true
type MaskingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MaskingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MaskingPolicy{}, &MaskingPolicyList{})
}
