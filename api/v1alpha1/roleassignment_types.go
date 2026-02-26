package v1alpha1

// RoleAssignmentShowOutput mirrors the SHOW GRANTS OF ROLE output stored in status.
// Used by both AccountRoleAssignment and DatabaseRoleAssignment.
type RoleAssignmentShowOutput struct {
	// CreatedOn is the timestamp when the role assignment was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Role is the role name that was assigned.
	Role string `json:"role,omitempty"`

	// GrantedTo is the grantee category: "ROLE", "USER", or "DATABASE_ROLE".
	GrantedTo string `json:"grantedTo,omitempty"`

	// GranteeName is the name of the role, user, or database role receiving the assignment.
	GranteeName string `json:"granteeName,omitempty"`

	// GrantedBy is the role that performed the assignment.
	GrantedBy string `json:"grantedBy,omitempty"`
}
