package v1alpha1

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RowAccessPolicyArgument defines an argument in a row access policy signature.
type RowAccessPolicyArgument struct {
	// Name is the argument name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type is the Snowflake data type (e.g. VARCHAR, NUMBER).
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
}

// RowAccessPolicySpec defines the desired state of a Snowflake Row Access Policy.
//
// +kubebuilder:validation:XValidation:rule="(has(self.databaseRef) && !has(self.databaseName)) || (!has(self.databaseRef) && has(self.databaseName))",message="exactly one of spec.databaseRef or spec.databaseName must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.schemaRef) && !has(self.schemaName)) || (!has(self.schemaRef) && has(self.schemaName))",message="exactly one of spec.schemaRef or spec.schemaName must be set"
type RowAccessPolicySpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake row access policy name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DatabaseRef references a Database CR in the same namespace.
	// Mutually exclusive with DatabaseName. Immutable after creation.
	// +optional
	DatabaseRef *LocalObjectReference `json:"databaseRef,omitempty"`

	// DatabaseName is the raw Snowflake database identifier.
	// Mutually exclusive with DatabaseRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	DatabaseName *string `json:"databaseName,omitempty"`

	// SchemaRef references a Schema CR in the same namespace.
	// Mutually exclusive with SchemaName. Immutable after creation.
	// +optional
	SchemaRef *LocalObjectReference `json:"schemaRef,omitempty"`

	// SchemaName is the raw Snowflake schema FQN.
	// Mutually exclusive with SchemaRef. Immutable after creation.
	// +optional
	// +kubebuilder:validation:MinLength=1
	SchemaName *string `json:"schemaName,omitempty"`

	// Signature defines the arguments for the row access policy.
	// Immutable after creation.
	// +kubebuilder:validation:MinItems=1
	Signature []RowAccessPolicyArgument `json:"signature"`

	// Body is the SQL expression that returns BOOLEAN to determine row visibility.
	// +kubebuilder:validation:MinLength=1
	Body string `json:"body"`

	// Comment is an optional description for the row access policy.
	// +optional
	Comment *string `json:"comment,omitempty"`
}

// Validate checks the RowAccessPolicySpec for consistency.
func (s *RowAccessPolicySpec) Validate() error {
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

// RowAccessPolicyShowOutput mirrors the SHOW ROW ACCESS POLICIES output stored in status.
type RowAccessPolicyShowOutput struct {
	// CreatedOn is the timestamp when the policy was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the policy name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// DatabaseName is the parent database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// Kind is the policy kind (ROW_ACCESS_POLICY).
	Kind string `json:"kind,omitempty"`

	// Owner is the role that owns the row access policy.
	Owner string `json:"owner,omitempty"`

	// Comment is the policy description.
	Comment string `json:"comment,omitempty"`
}

// RowAccessPolicyStatus defines the observed state of a RowAccessPolicy.
type RowAccessPolicyStatus struct {
	CommonStatus `json:",inline"`

	// DatabaseName is the parent Snowflake database name.
	DatabaseName string `json:"databaseName,omitempty"`

	// SchemaName is the parent Snowflake schema name.
	SchemaName string `json:"schemaName,omitempty"`

	// ShowOutput contains the raw SHOW ROW ACCESS POLICIES output for this policy.
	ShowOutput *RowAccessPolicyShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// RowAccessPolicy is the Schema for the rowaccesspolicies API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="DATABASE",type=string,JSONPath=`.status.databaseName`
// +kubebuilder:printcolumn:name="SCHEMA",type=string,JSONPath=`.status.schemaName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type RowAccessPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RowAccessPolicySpec   `json:"spec,omitempty"`
	Status RowAccessPolicyStatus `json:"status,omitempty"`
}

// RowAccessPolicyList contains a list of RowAccessPolicy.
// +kubebuilder:object:root=true
type RowAccessPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RowAccessPolicy `json:"items"`
}

// GetConditions returns the conditions of the RowAccessPolicy.
func (rap *RowAccessPolicy) GetConditions() []metav1.Condition { return rap.Status.Conditions }

// SetConditions sets the conditions of the RowAccessPolicy.
func (rap *RowAccessPolicy) SetConditions(c []metav1.Condition) { rap.Status.Conditions = c }

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (rap *RowAccessPolicy) GetFullyQualifiedName() string { return rap.Status.FullyQualifiedName }

// GetSpecName returns the Snowflake resource name from the spec.
func (rap *RowAccessPolicy) GetSpecName() string { return rap.Spec.Name }

// GetProviderRef returns the provider reference from the spec.
func (rap *RowAccessPolicy) GetProviderRef() ProviderReference { return rap.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (rap *RowAccessPolicy) GetUseRole() *string { return rap.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (rap *RowAccessPolicy) GetObservedGeneration() int64 { return rap.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (rap *RowAccessPolicy) SetObservedGeneration(v int64) { rap.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (rap *RowAccessPolicy) GetLastAppliedSpecHash() string { return rap.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (rap *RowAccessPolicy) SetLastAppliedSpecHash(v string) { rap.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (rap *RowAccessPolicy) GetTrackedParametersList() []string { return rap.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (rap *RowAccessPolicy) SetTrackedParametersList(v []string) { rap.Status.TrackedParameters = v }

// ValidateSpec validates the resource spec.
func (rap *RowAccessPolicy) ValidateSpec() error { return rap.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (rap *RowAccessPolicy) ComputeSpecHash() (string, error) { return ComputeSpecHash(rap.Spec) }

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (rap *RowAccessPolicy) GetDeletionPolicy() DeletionPolicy {
	if rap.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return rap.Spec.DeletionPolicy
}

// GetOwner returns the owner from status.
func (rap *RowAccessPolicy) GetOwner() string {
	if rap.Status.ShowOutput != nil {
		return rap.Status.ShowOutput.Owner
	}

	return ""
}

func init() {
	SchemeBuilder.Register(&RowAccessPolicy{}, &RowAccessPolicyList{})
}
