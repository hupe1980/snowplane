package v1alpha1

import (
	"fmt"
	"strings"

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
//
// +kubebuilder:validation:XValidation:rule="self.objectType == oldSelf.objectType",message="spec.objectType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.objectName == oldSelf.objectName",message="spec.objectName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.accountRole) == has(self.accountRole) && (!has(self.accountRole) || self.accountRole == oldSelf.accountRole)",message="spec.accountRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.accountRoleRef) == has(self.accountRoleRef) && (!has(self.accountRoleRef) || self.accountRoleRef == oldSelf.accountRoleRef)",message="spec.accountRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRole) == has(self.databaseRole) && (!has(self.databaseRole) || self.databaseRole == oldSelf.databaseRole)",message="spec.databaseRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.databaseRoleRef) == has(self.databaseRoleRef) && (!has(self.databaseRoleRef) || self.databaseRoleRef == oldSelf.databaseRoleRef)",message="spec.databaseRoleRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.accountRole) ? 1 : 0) + (has(self.accountRoleRef) ? 1 : 0) + (has(self.databaseRole) ? 1 : 0) + (has(self.databaseRoleRef) ? 1 : 0) == 1",message="exactly one of spec.accountRole, spec.accountRoleRef, spec.databaseRole, or spec.databaseRoleRef must be set"
type GrantOwnershipSpec struct {
	CommonSpec `json:",inline"`

	// ObjectType is the Snowflake object type whose ownership is being transferred.
	// +kubebuilder:validation:Enum=DATABASE;SCHEMA;TABLE;VIEW;"MATERIALIZED VIEW";WAREHOUSE;STAGE;"FILE FORMAT";FUNCTION;PROCEDURE;STREAM;TASK;PIPE;SEQUENCE;TAG;"MASKING POLICY";"ROW ACCESS POLICY";"NETWORK POLICY";"RESOURCE MONITOR";USER;"COMPUTE POOL";INTEGRATION;CONNECTION;"FAILOVER GROUP";"REPLICATION GROUP";"EXTERNAL VOLUME";ALERT;SECRET;MODEL;"DYNAMIC TABLE";"ICEBERG TABLE";"EVENT TABLE";"EXTERNAL TABLE";"NETWORK RULE";"PASSWORD POLICY"
	ObjectType string `json:"objectType"`

	// ObjectName is the fully qualified name of the object
	// (e.g. "MY_DB" for a database, "MY_DB"."MY_SCHEMA"."MY_TABLE" for a table).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	ObjectName string `json:"objectName"`

	// AccountRole is the name of the account role to transfer ownership to.
	// Mutually exclusive with AccountRoleRef, DatabaseRole, and DatabaseRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	AccountRole *string `json:"accountRole,omitempty"`

	// AccountRoleRef references an AccountRole CR in the same namespace.
	// Mutually exclusive with AccountRole, DatabaseRole, and DatabaseRoleRef.
	// +optional
	AccountRoleRef *LocalObjectReference `json:"accountRoleRef,omitempty"`

	// DatabaseRole is the fully qualified database role name (e.g. "MY_DB"."MY_ROLE").
	// Mutually exclusive with AccountRole, AccountRoleRef, and DatabaseRoleRef.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	DatabaseRole *string `json:"databaseRole,omitempty"`

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
	} else {
		// Validate that objectType contains only keyword-safe characters (letters, digits, underscores, spaces).
		// This prevents SQL injection through free-text ObjectType without coupling to a specific allowlist.
		for _, c := range s.ObjectType {
			if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != ' ' {
				errs = append(errs, fmt.Errorf("spec.objectType contains invalid character %q; must contain only letters, digits, underscores, and spaces", string(c)))

				break
			}
		}

		// Reject excessively long values.
		if len(s.ObjectType) > 64 {
			errs = append(errs, fmt.Errorf("spec.objectType too long (%d chars, max 64)", len(s.ObjectType)))
		}

		// Normalize and reject empty after trim.
		if strings.TrimSpace(s.ObjectType) == "" {
			errs = append(errs, fmt.Errorf("spec.objectType must not be blank"))
		}
	}

	if s.ObjectName == "" {
		errs = append(errs, fmt.Errorf("spec.objectName is required"))
	}

	// Exactly one of accountRole/accountRoleRef/databaseRole/databaseRoleRef must be set.
	count := 0
	if s.AccountRole != nil {
		count++
	}

	if s.AccountRoleRef != nil {
		count++
	}

	if s.DatabaseRole != nil {
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
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
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

// GetSpecName returns a human-readable composite name for the ownership transfer.
func (g *GrantOwnership) GetSpecName() string {
	var role string
	if g.Spec.AccountRole != nil {
		role = *g.Spec.AccountRole
	}

	if role == "" && g.Spec.AccountRoleRef != nil {
		role = "(ref: " + g.Spec.AccountRoleRef.Name + ")"
	}

	if role == "" && g.Spec.DatabaseRole != nil {
		role = *g.Spec.DatabaseRole
	}

	if role == "" && g.Spec.DatabaseRoleRef != nil {
		role = "(ref: " + g.Spec.DatabaseRoleRef.Name + ")"
	}

	return fmt.Sprintf("OWNERSHIP ON %s %s -> %s", g.Spec.ObjectType, g.Spec.ObjectName, role)
}

func init() {
	SchemeBuilder.Register(&GrantOwnership{}, &GrantOwnershipList{})
}
