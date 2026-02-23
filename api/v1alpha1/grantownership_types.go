package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CurrentGrantsBehavior specifies how existing outbound privileges on an object
// are handled when ownership is transferred.
// +kubebuilder:validation:Enum=COPY;REVOKE
type CurrentGrantsBehavior string

const (
	// CurrentGrantsBehaviorCopy copies all existing outbound privileges to the new owner.
	CurrentGrantsBehaviorCopy CurrentGrantsBehavior = "COPY"

	// CurrentGrantsBehaviorRevoke revokes all existing outbound privileges before transfer.
	CurrentGrantsBehaviorRevoke CurrentGrantsBehavior = "REVOKE"
)

// GrantOwnershipSpec defines the desired state of a GrantOwnership.
// All fields are immutable after creation — changing any field requires
// deleting and recreating the resource.
type GrantOwnershipSpec struct {
	CommonSpec `json:",inline"`

	// ObjectType is the Snowflake object type whose ownership is being transferred.
	// Examples: DATABASE, SCHEMA, TABLE, VIEW, WAREHOUSE, STAGE, TASK, STREAM,
	// TAG, MASKING POLICY, ROW ACCESS POLICY, NETWORK POLICY, RESOURCE MONITOR.
	// +kubebuilder:validation:MinLength=1
	ObjectType string `json:"objectType"`

	// ObjectName is the fully qualified name of the object
	// (e.g. "MY_DB" for a database, "MY_DB"."MY_SCHEMA"."MY_TABLE" for a table).
	// +kubebuilder:validation:MinLength=1
	ObjectName string `json:"objectName"`

	// AccountRole is the name of the account role to transfer ownership to.
	// Mutually exclusive with AccountRoleRef, DatabaseRole, and DatabaseRoleRef.
	// +optional
	AccountRole string `json:"accountRole,omitempty"`

	// AccountRoleRef references an AccountRole CR in the same namespace.
	// Mutually exclusive with AccountRole, DatabaseRole, and DatabaseRoleRef.
	// +optional
	AccountRoleRef *LocalObjectReference `json:"accountRoleRef,omitempty"`

	// DatabaseRole is the fully qualified database role name (e.g. "MY_DB"."MY_ROLE").
	// Mutually exclusive with AccountRole, AccountRoleRef, and DatabaseRoleRef.
	// +optional
	DatabaseRole string `json:"databaseRole,omitempty"`

	// DatabaseRoleRef references a DatabaseRole CR in the same namespace.
	// Mutually exclusive with AccountRole, AccountRoleRef, and DatabaseRole.
	// +optional
	DatabaseRoleRef *LocalObjectReference `json:"databaseRoleRef,omitempty"`

	// CurrentGrantsBehavior specifies how existing outbound privileges
	// on the object are handled during the ownership transfer.
	// COPY: preserves existing privileges with the new owner as grantor.
	// REVOKE: removes all existing outbound privileges before transfer.
	// If not set, Snowflake requires that no outbound privileges exist.
	// +optional
	CurrentGrantsBehavior *CurrentGrantsBehavior `json:"currentGrantsBehavior,omitempty"`
}

// Validate returns an error if the spec is semantically invalid.
func (s *GrantOwnershipSpec) Validate() error {
	var errs []error

	if s.ObjectType == "" {
		errs = append(errs, fmt.Errorf("spec.objectType is required"))
	}

	if s.ObjectName == "" {
		errs = append(errs, fmt.Errorf("spec.objectName is required"))
	}

	// Exactly one of accountRole/accountRoleRef/databaseRole/databaseRoleRef must be set.
	count := 0
	if s.AccountRole != "" {
		count++
	}

	if s.AccountRoleRef != nil {
		count++
	}

	if s.DatabaseRole != "" {
		count++
	}

	if s.DatabaseRoleRef != nil {
		count++
	}

	if count != 1 {
		errs = append(errs, fmt.Errorf("exactly one of accountRole, accountRoleRef, databaseRole, or databaseRoleRef must be set"))
	}

	if err := s.CommonSpec.Validate(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid GrantOwnershipSpec: %v", errs)
	}

	return nil
}

// GrantOwnershipStatus defines the observed state of a GrantOwnership.
type GrantOwnershipStatus struct {
	CommonStatus `json:",inline"`

	// RoleName is the resolved name of the target role.
	// +optional
	RoleName string `json:"roleName,omitempty"`

	// ShowOutput contains the most recently observed SHOW GRANTS output
	// for the OWNERSHIP privilege.
	// +optional
	ShowOutput *GrantOwnershipShowOutput `json:"showOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	// +optional
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// GrantOwnershipShowOutput contains the fields from SHOW GRANTS
// for the OWNERSHIP privilege.
type GrantOwnershipShowOutput struct {
	// CreatedOn is the timestamp when the ownership was granted.
	CreatedOn string `json:"createdOn,omitempty"`

	// Privilege is always OWNERSHIP.
	Privilege string `json:"privilege,omitempty"`

	// GrantedOn is the object type (e.g. DATABASE, TABLE).
	GrantedOn string `json:"grantedOn,omitempty"`

	// Name is the fully qualified object name.
	Name string `json:"name,omitempty"`

	// GrantedTo is the grantee type (ROLE or DATABASE_ROLE).
	GrantedTo string `json:"grantedTo,omitempty"`

	// GranteeName is the name of the role that owns the object.
	GranteeName string `json:"granteeName,omitempty"`
}

// GrantOwnership is the Schema for the grantownerships API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=gow,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="OBJECT-TYPE",type=string,JSONPath=`.spec.objectType`
// +kubebuilder:printcolumn:name="OBJECT-NAME",type=string,JSONPath=`.spec.objectName`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type GrantOwnership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GrantOwnershipSpec   `json:"spec,omitempty"`
	Status GrantOwnershipStatus `json:"status,omitempty"`
}

// GrantOwnershipList contains a list of GrantOwnership.
// +kubebuilder:object:root=true
type GrantOwnershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GrantOwnership `json:"items"`
}

// GetConditions returns the conditions of the GrantOwnership.
func (g *GrantOwnership) GetConditions() []metav1.Condition { return g.Status.Conditions }

// SetConditions sets the conditions of the GrantOwnership.
func (g *GrantOwnership) SetConditions(c []metav1.Condition) { g.Status.Conditions = c }

// GetFullyQualifiedName returns the Snowflake fully qualified identifier from status.
func (g *GrantOwnership) GetFullyQualifiedName() string { return g.Status.FullyQualifiedName }

// GetProviderRef returns the provider reference from the spec.
func (g *GrantOwnership) GetProviderRef() ProviderReference { return g.Spec.ProviderRef }

// GetUseRole returns the use role from the spec.
func (g *GrantOwnership) GetUseRole() *string { return g.Spec.UseRole }

// GetObservedGeneration returns the observed generation from status.
func (g *GrantOwnership) GetObservedGeneration() int64 { return g.Status.ObservedGeneration }

// SetObservedGeneration sets the observed generation in status.
func (g *GrantOwnership) SetObservedGeneration(v int64) { g.Status.ObservedGeneration = v }

// GetLastAppliedSpecHash returns the last applied spec hash from status.
func (g *GrantOwnership) GetLastAppliedSpecHash() string { return g.Status.LastAppliedSpecHash }

// SetLastAppliedSpecHash sets the last applied spec hash in status.
func (g *GrantOwnership) SetLastAppliedSpecHash(v string) { g.Status.LastAppliedSpecHash = v }

// GetTrackedParametersList returns the tracked parameters list from status.
func (g *GrantOwnership) GetTrackedParametersList() []string { return g.Status.TrackedParameters }

// SetTrackedParametersList sets the tracked parameters list in status.
func (g *GrantOwnership) SetTrackedParametersList(v []string) { g.Status.TrackedParameters = v }

// ValidateSpec validates the resource spec.
func (g *GrantOwnership) ValidateSpec() error { return g.Spec.Validate() }

// ComputeSpecHash returns a SHA-256 hash of the spec for drift detection.
func (g *GrantOwnership) ComputeSpecHash() (string, error) { return ComputeSpecHash(g.Spec) }

// GetSpecName returns a human-readable composite name for the ownership transfer.
func (g *GrantOwnership) GetSpecName() string {
	role := g.Spec.AccountRole
	if role == "" && g.Spec.AccountRoleRef != nil {
		role = "(ref: " + g.Spec.AccountRoleRef.Name + ")"
	}

	if role == "" {
		role = g.Spec.DatabaseRole
	}

	if role == "" && g.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + g.Spec.DatabaseRoleRef.Name + ")"
	}

	return fmt.Sprintf("OWNERSHIP ON %s %s -> %s", g.Spec.ObjectType, g.Spec.ObjectName, role)
}

// GetDeletionPolicy returns the deletion policy, defaulting to Delete.
func (g *GrantOwnership) GetDeletionPolicy() DeletionPolicy {
	if g.Spec.DeletionPolicy == "" {
		return DeletionPolicyDelete
	}

	return g.Spec.DeletionPolicy
}

// GetOwner returns empty — ownership transfers are account-scoped operations.
func (g *GrantOwnership) GetOwner() string { return "" }

func init() {
	SchemeBuilder.Register(&GrantOwnership{}, &GrantOwnershipList{})
}
