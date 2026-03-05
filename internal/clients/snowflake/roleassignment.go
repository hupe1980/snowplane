package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// RoleAssignmentObservation holds the result of observing a Snowflake role assignment.
type RoleAssignmentObservation struct {
	// Exists indicates whether the role assignment was found.
	Exists bool

	// ShowOutput contains the matching SHOW GRANTS OF ROLE row.
	ShowOutput *v1alpha1.RoleAssignmentShowOutput
}

// RoleAssignmentIdentifier uniquely identifies a role assignment for observation.
type RoleAssignmentIdentifier struct {
	// RoleName is the role being assigned.
	// For account roles: just the role name (e.g. "ANALYST").
	// For database roles: fully qualified name (e.g. "MY_DB.MY_ROLE").
	RoleName string

	// IsDatabaseRole indicates whether RoleName refers to a database role.
	IsDatabaseRole bool

	// GrantedTo is the grantee category: "ROLE", "USER", or "DATABASE_ROLE".
	GrantedTo string

	// GranteeName is the target role/user/database role receiving the assignment.
	GranteeName string
}

// FullyQualifiedName returns a human-readable representation of the role assignment.
func (id RoleAssignmentIdentifier) FullyQualifiedName() string {
	rolePrefix := "ROLE"
	if id.IsDatabaseRole {
		rolePrefix = "DATABASE ROLE"
	}

	quotedRole := quoteRoleName(id.RoleName, id.IsDatabaseRole)
	quotedTarget := quoteTarget(id.GrantedTo, id.GranteeName)

	return fmt.Sprintf("GRANT %s %s TO %s %s", rolePrefix, quotedRole, id.GrantedTo, quotedTarget)
}

// String returns the fully qualified name.
func (id RoleAssignmentIdentifier) String() string { return id.FullyQualifiedName() }

// GrantRoleOptions holds the parameters for granting a role.
type GrantRoleOptions struct {
	// RoleName is the role to grant.
	RoleName string

	// IsDatabaseRole indicates whether RoleName is a database role.
	IsDatabaseRole bool

	// ToRole is the target account role (mutually exclusive with ToUser, ToDatabaseRole).
	ToRole string

	// ToUser is the target user (mutually exclusive with ToRole, ToDatabaseRole).
	ToUser string

	// ToDatabaseRole is the target database role (mutually exclusive with ToRole, ToUser).
	// Only valid when IsDatabaseRole is true.
	ToDatabaseRole string
}

// Validate checks the GrantRoleOptions for validity.
func (o *GrantRoleOptions) Validate() error {
	if strings.TrimSpace(o.RoleName) == "" {
		return fmt.Errorf("roleName is required")
	}

	count := 0
	if o.ToRole != "" {
		count++
	}

	if o.ToUser != "" {
		count++
	}

	if o.ToDatabaseRole != "" {
		count++
	}

	if count != 1 {
		return fmt.Errorf("exactly one of toRole, toUser, or toDatabaseRole must be set")
	}

	if o.ToDatabaseRole != "" && !o.IsDatabaseRole {
		return fmt.Errorf("toDatabaseRole is only valid for database role assignments")
	}

	if o.ToUser != "" && o.IsDatabaseRole {
		return fmt.Errorf("toUser is not valid for database role assignments")
	}

	return nil
}

// RevokeRoleOptions holds the parameters for revoking a role.
type RevokeRoleOptions struct {
	// RoleName is the role to revoke.
	RoleName string

	// IsDatabaseRole indicates whether RoleName is a database role.
	IsDatabaseRole bool

	// FromRole is the account role to revoke from (mutually exclusive with FromUser, FromDatabaseRole).
	FromRole string

	// FromUser is the user to revoke from (mutually exclusive with FromRole, FromDatabaseRole).
	FromUser string

	// FromDatabaseRole is the database role to revoke from (mutually exclusive with FromRole, FromUser).
	FromDatabaseRole string
}

// Validate checks the RevokeRoleOptions for validity.
func (o *RevokeRoleOptions) Validate() error {
	if strings.TrimSpace(o.RoleName) == "" {
		return fmt.Errorf("roleName is required")
	}

	count := 0
	if o.FromRole != "" {
		count++
	}

	if o.FromUser != "" {
		count++
	}

	if o.FromDatabaseRole != "" {
		count++
	}

	if count != 1 {
		return fmt.Errorf("exactly one of fromRole, fromUser, or fromDatabaseRole must be set")
	}

	if o.FromDatabaseRole != "" && !o.IsDatabaseRole {
		return fmt.Errorf("fromDatabaseRole is only valid for database role revocations")
	}

	if o.FromUser != "" && o.IsDatabaseRole {
		return fmt.Errorf("fromUser is not valid for database role revocations")
	}

	return nil
}

// RoleAssignmentClient provides operations against Snowflake role assignments.
type RoleAssignmentClient struct {
	client SQLExecutor
}

// NewRoleAssignmentClient creates a new RoleAssignmentClient backed by the given SQLExecutor.
func NewRoleAssignmentClient(c SQLExecutor) *RoleAssignmentClient {
	return &RoleAssignmentClient{client: c}
}

// buildGrantRoleSQL builds the GRANT ROLE / GRANT DATABASE ROLE SQL statement.
//
// Examples:
//
//	GRANT ROLE "ANALYST" TO ROLE "SYSADMIN"
//	GRANT ROLE "ANALYST" TO USER "john"
//	GRANT DATABASE ROLE "MY_DB"."READER" TO ROLE "SYSADMIN"
//	GRANT DATABASE ROLE "MY_DB"."READER" TO DATABASE ROLE "MY_DB"."WRITER"
func buildGrantRoleSQL(opts GrantRoleOptions) string {
	var sb strings.Builder

	sb.WriteString("GRANT ")

	if opts.IsDatabaseRole {
		sb.WriteString("DATABASE ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifierParts(opts.RoleName))
	} else {
		sb.WriteString("ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.RoleName))
	}

	if opts.ToRole != "" {
		sb.WriteString(" TO ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.ToRole))
	} else if opts.ToUser != "" {
		sb.WriteString(" TO USER ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.ToUser))
	} else if opts.ToDatabaseRole != "" {
		sb.WriteString(" TO DATABASE ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifierParts(opts.ToDatabaseRole))
	}

	return sb.String()
}

// GrantRole grants a role to another role or user.
func (c *RoleAssignmentClient) GrantRole(ctx context.Context, opts GrantRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid grant role options: %w", err))
	}

	query := buildGrantRoleSQL(opts)

	if _, err := c.client.Exec(ctx, query); err != nil {
		return fmt.Errorf("granting role %s: %w", opts.RoleName, err)
	}

	return nil
}

// buildRevokeRoleSQL builds the REVOKE ROLE / REVOKE DATABASE ROLE SQL statement.
//
// Examples:
//
//	REVOKE ROLE "ANALYST" FROM ROLE "SYSADMIN"
//	REVOKE ROLE "ANALYST" FROM USER "john"
//	REVOKE DATABASE ROLE "MY_DB"."READER" FROM ROLE "SYSADMIN"
//	REVOKE DATABASE ROLE "MY_DB"."READER" FROM DATABASE ROLE "MY_DB"."WRITER"
func buildRevokeRoleSQL(opts RevokeRoleOptions) string {
	var sb strings.Builder

	sb.WriteString("REVOKE ")

	if opts.IsDatabaseRole {
		sb.WriteString("DATABASE ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifierParts(opts.RoleName))
	} else {
		sb.WriteString("ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.RoleName))
	}

	if opts.FromRole != "" {
		sb.WriteString(" FROM ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.FromRole))
	} else if opts.FromUser != "" {
		sb.WriteString(" FROM USER ")
		sb.WriteString(sqlbuilder.QuoteIdentifier(opts.FromUser))
	} else if opts.FromDatabaseRole != "" {
		sb.WriteString(" FROM DATABASE ROLE ")
		sb.WriteString(sqlbuilder.QuoteIdentifierParts(opts.FromDatabaseRole))
	}

	return sb.String()
}

// RevokeRole revokes a role from another role or user.
func (c *RoleAssignmentClient) RevokeRole(ctx context.Context, opts RevokeRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid revoke role options: %w", err))
	}

	query := buildRevokeRoleSQL(opts)

	if _, err := c.client.Exec(ctx, query); err != nil {
		return fmt.Errorf("revoking role %s: %w", opts.RoleName, err)
	}

	return nil
}

// ShowGrantsOfRole queries SHOW GRANTS OF ROLE / SHOW GRANTS OF DATABASE ROLE
// and returns all assignment rows.
//
// Examples:
//
//	SHOW GRANTS OF ROLE "ANALYST"
//	SHOW GRANTS OF DATABASE ROLE "MY_DB"."READER"
func (c *RoleAssignmentClient) ShowGrantsOfRole(ctx context.Context, roleName string, isDatabaseRole bool) ([]*v1alpha1.RoleAssignmentShowOutput, error) {
	if strings.TrimSpace(roleName) == "" {
		return nil, NewTerminalError(fmt.Errorf("roleName is required for ShowGrantsOfRole"))
	}

	var query string
	if isDatabaseRole {
		query = "SHOW GRANTS OF DATABASE ROLE " + sqlbuilder.QuoteIdentifierParts(roleName)
	} else {
		query = "SHOW GRANTS OF ROLE " + sqlbuilder.QuoteIdentifier(roleName)
	}

	rows, err := c.client.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("showing grants of role %s: %w", roleName, err)
	}
	defer closeRows(rows)

	return scanRoleAssignmentShowOutput(rows)
}

// Observe checks whether a specific role assignment exists by querying SHOW GRANTS OF ROLE
// and filtering for the matching grantee.
func (c *RoleAssignmentClient) Observe(ctx context.Context, id RoleAssignmentIdentifier) (*RoleAssignmentObservation, error) {
	grants, err := c.ShowGrantsOfRole(ctx, id.RoleName, id.IsDatabaseRole)
	if err != nil {
		if IsObjectNotFound(err) {
			return &RoleAssignmentObservation{Exists: false}, nil
		}

		return nil, err
	}

	for _, grant := range grants {
		if strings.EqualFold(grant.GrantedTo, id.GrantedTo) &&
			matchesRoleAssignmentGrantee(grant.GranteeName, id.GranteeName) {
			return &RoleAssignmentObservation{
				Exists:     true,
				ShowOutput: grant,
			}, nil
		}
	}

	return &RoleAssignmentObservation{Exists: false}, nil
}

// matchesRoleAssignmentGrantee compares a grantee name from SHOW GRANTS output
// against an expected grantee name. For database roles, Snowflake may return
// only the unqualified role name (e.g. "MY_ROLE") while the identifier uses
// the fully qualified name (e.g. "MY_DB.MY_ROLE"), or vice versa.
func matchesRoleAssignmentGrantee(actual, expected string) bool {
	if strings.EqualFold(actual, expected) {
		return true
	}

	// Strip DB prefix from expected: expected="DB.ROLE", actual="ROLE".
	if idx := strings.LastIndex(expected, "."); idx >= 0 {
		if strings.EqualFold(actual, expected[idx+1:]) {
			return true
		}
	}

	// Strip DB prefix from actual: actual="DB.ROLE", expected="ROLE".
	if idx := strings.LastIndex(actual, "."); idx >= 0 {
		if strings.EqualFold(actual[idx+1:], expected) {
			return true
		}
	}

	return false
}

// scanRoleAssignmentShowOutput scans SHOW GRANTS OF ROLE results.
// Columns: created_on, role, granted_to, grantee_name, granted_by
func scanRoleAssignmentShowOutput(rows *sql.Rows) ([]*v1alpha1.RoleAssignmentShowOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	var results []*v1alpha1.RoleAssignmentShowOutput

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		out := &v1alpha1.RoleAssignmentShowOutput{
			CreatedOn:   colMap["created_on"],
			Role:        colMap["role"],
			GrantedTo:   colMap["granted_to"],
			GranteeName: colMap["grantee_name"],
			GrantedBy:   colMap["granted_by"],
		}

		results = append(results, out)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return results, nil
}

// quoteRoleName quotes a role name, handling database roles with dot notation.
func quoteRoleName(name string, isDatabaseRole bool) string {
	if isDatabaseRole {
		return sqlbuilder.QuoteIdentifierParts(name)
	}

	return sqlbuilder.QuoteIdentifier(name)
}

// quoteTarget quotes the target name based on the grantedTo category.
func quoteTarget(grantedTo, name string) string {
	switch strings.ToUpper(grantedTo) {
	case "DATABASE_ROLE":
		return sqlbuilder.QuoteIdentifierParts(name)
	default: // "ROLE", "USER"
		return sqlbuilder.QuoteIdentifier(name)
	}
}
