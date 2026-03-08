package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SQLStatementExpectation defines a column/value expectation for the
// observe query result. The resource is considered "existing" when all
// expectations match at least one row.
type SQLStatementExpectation struct {
	// Column is the result set column name to inspect.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Column string `json:"column"`

	// Value is the expected column value (case-insensitive string comparison).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Value string `json:"value"`
}

// SQLStatementSpec defines the desired state of a SQLStatement resource.
// SQLStatement is an escape-hatch CRD that allows executing arbitrary SQL
// against Snowflake. It is intended for resources not yet covered by typed
// CRDs, one-off DDL, custom grants, and advanced use cases.
//
// IMPORTANT: This resource bypasses Snowplane's SQL builder validation and
// escaping. Users are responsible for SQL correctness and safety. Prefer
// typed CRDs (Database, Schema, etc.) whenever possible.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type SQLStatementSpec struct {
	CommonSpec `json:",inline"`

	// Execute is the SQL statement to run when creating the resource.
	// For idempotent statements, use IF NOT EXISTS or CREATE OR REPLACE patterns.
	// Multi-statement SQL (separated by semicolons) is supported.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=65536
	Execute string `json:"execute"`

	// Revert is the SQL statement to run when deleting the resource.
	// This should undo what Execute created (e.g. DROP, REVOKE).
	// When omitted, no SQL is executed on deletion (orphan behavior for the
	// Snowflake-side resource, though the Kubernetes CR is still deleted).
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=65536
	Revert *string `json:"revert,omitempty"`

	// Observe is a SQL query whose result set is used for existence checks
	// and drift detection. The resource is considered "existing" when the
	// query returns at least one row matching all ObserveExpect expectations.
	// When omitted, the resource is always considered "not observable" —
	// Execute runs once and the resource stays in Ready state without drift
	// detection.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=65536
	Observe *string `json:"observe,omitempty"`

	// ObserveExpect defines column/value expectations that the observe query
	// result must satisfy. All expectations must match at least one row for
	// the resource to be considered "existing". When empty (but observe is
	// set), any non-empty result set indicates existence.
	// +optional
	// +listType=atomic
	ObserveExpect []SQLStatementExpectation `json:"observeExpect,omitempty"`

	// Idempotent indicates whether the execute SQL is safe to run multiple
	// times (e.g. uses IF NOT EXISTS or CREATE OR REPLACE). This field is
	// informational metadata for human operators and GitOps tooling — the
	// reconciler prevents re-execution via the creation-initiated guard and
	// status.executeHash tracking regardless of this field's value.
	// +kubebuilder:default=false
	// +optional
	Idempotent bool `json:"idempotent,omitempty"`

	// DangerousAllowDestructive must be set to true for execute or revert SQL
	// that contains destructive keywords (DROP, TRUNCATE, DELETE, REMOVE).
	// This provides an explicit opt-in for operations that could cause data loss.
	// +kubebuilder:default=false
	// +optional
	DangerousAllowDestructive bool `json:"dangerousAllowDestructive,omitempty"`
}

// SQLStatementObserveResult captures the output of the observe query.
type SQLStatementObserveResult struct {
	// RowCount is the number of rows returned by the observe query.
	RowCount int32 `json:"rowCount"`

	// Matched indicates whether all ObserveExpect expectations were satisfied.
	Matched bool `json:"matched"`
}

// SQLStatementStatus defines the observed state of a SQLStatement resource.
type SQLStatementStatus struct {
	CommonStatus `json:",inline"`

	// ObserveResult captures the latest observe query result.
	// +optional
	ObserveResult *SQLStatementObserveResult `json:"observeResult,omitempty"`

	// ExecuteHash is the SHA-256 hash of the execute SQL at the time of
	// last successful execution. Used to detect spec.execute changes that
	// require re-execution (force-new semantics).
	ExecuteHash string `json:"executeHash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=sqlstmt
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="IDEMPOTENT",type=boolean,JSONPath=`.spec.idempotent`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// SQLStatement is an escape-hatch CRD for executing arbitrary SQL against
// Snowflake. It is gated behind the --enable-sql-statement flag due to its
// inherent risks (arbitrary SQL execution, no type safety, potential for
// non-idempotent operations). Prefer typed CRDs whenever possible.
type SQLStatement struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SQLStatementSpec   `json:"spec,omitempty"`
	Status SQLStatementStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SQLStatementList contains a list of SQLStatement resources.
type SQLStatementList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SQLStatement `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SQLStatement{}, &SQLStatementList{})
}

// GetSpecName returns the Kubernetes object name as the resource's identity.
// SQLStatement has no Snowflake object name — the K8s metadata.name uniquely
// identifies the statement within the cluster.
func (s *SQLStatement) GetSpecName() string {
	return s.Name
}
