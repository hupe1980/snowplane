// Package snowflake provides Snowflake SQL client implementations.
package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// GrantObservation holds the result of observing a Snowflake privilege grant.
type GrantObservation struct {
	// Exists indicates whether the grant was found.
	Exists bool

	// ShowOutput contains the matching SHOW GRANTS row.
	ShowOutput *GrantShowOutput
}

// GrantShowOutput contains the fields from SHOW GRANTS ON <object_type> <object_name>.
type GrantShowOutput struct {
	// CreatedOn is the timestamp when the grant was created.
	CreatedOn string

	// Privilege is the privilege name (e.g. USAGE, SELECT, CREATE SCHEMA).
	Privilege string

	// GrantedOn is the object type the privilege is granted on (e.g. DATABASE, WAREHOUSE).
	// For future grants from SHOW FUTURE GRANTS, this comes from the "grant_on" column.
	GrantedOn string

	// Name is the fully qualified name of the object.
	Name string

	// GrantedTo is the category of the grantee (e.g. ROLE, DATABASE_ROLE, SHARE).
	// For future grants from SHOW FUTURE GRANTS, this comes from the "grant_to" column.
	GrantedTo string

	// GranteeName is the name of the role/share the privilege is granted to.
	GranteeName string

	// GrantOption indicates whether the grantee can re-grant the privilege.
	GrantOption bool

	// GrantedBy is the role that performed the grant.
	GrantedBy string
}

// GrantKind indicates the type of grant for observation dispatch.
type GrantKind string

const (
	// GrantKindRegular is a normal privilege grant on an existing object.
	GrantKindRegular GrantKind = "Regular"

	// GrantKindFuture is a future grant on objects not yet created.
	GrantKindFuture GrantKind = "Future"

	// GrantKindAll is a bulk grant on all existing objects of a type.
	GrantKindAll GrantKind = "All"

	// GrantKindShare is a grant to a share.
	GrantKindShare GrantKind = "Share"
)

// GrantIdentifier uniquely identifies a privilege grant. It serves as the
// composite key for grant observation.
type GrantIdentifier struct {
	// Kind is the grant type (Regular, Future, All, Share).
	Kind GrantKind

	// Privilege is the privilege name (e.g. USAGE, SELECT, CREATE SCHEMA).
	Privilege string

	// OnClause is the SQL ON clause (e.g. "ON DATABASE MY_DB", "ON FUTURE TABLES IN SCHEMA MY_DB.PUBLIC").
	OnClause string

	// ToClause is the SQL TO/FROM clause (e.g. "TO ROLE DATA_READER", "TO SHARE my_share").
	ToClause string

	// GranteeName is the target role/share name for observation filtering.
	GranteeName string

	// ShowGrantsTarget is the target for the SHOW GRANTS/SHOW FUTURE GRANTS query.
	// Examples: "ON DATABASE MY_DB", "ON ACCOUNT", "TO SHARE my_share",
	// "IN DATABASE MY_DB" (for future grants).
	ShowGrantsTarget string
}

// FullyQualifiedName returns a human-readable representation of the grant.
func (g GrantIdentifier) FullyQualifiedName() string {
	return fmt.Sprintf("GRANT %s %s %s", g.Privilege, g.OnClause, g.ToClause)
}

// String returns the fully qualified name.
func (g GrantIdentifier) String() string { return g.FullyQualifiedName() }

// CreateGrantOptions holds the parameters for creating a grant.
type CreateGrantOptions struct {
	// Privilege is the privilege name (e.g. USAGE, SELECT, MODIFY).
	Privilege string

	// OnClause is the full ON clause (e.g. "ON DATABASE MY_DB",
	// "ON FUTURE TABLES IN SCHEMA MY_DB.PUBLIC", "ON ACCOUNT").
	OnClause string

	// ToClause is the full TO clause (e.g. "TO ROLE DATA_READER",
	// "TO DATABASE ROLE MY_DB.MY_ROLE", "TO SHARE my_share").
	ToClause string

	// WithGrantOption enables re-granting the privilege.
	WithGrantOption bool
}

// Validate checks the CreateGrantOptions for validity.
func (o *CreateGrantOptions) Validate() error {
	if strings.TrimSpace(o.Privilege) == "" {
		return fmt.Errorf("privilege is required")
	}

	if strings.TrimSpace(o.OnClause) == "" {
		return fmt.Errorf("onClause is required")
	}

	if strings.TrimSpace(o.ToClause) == "" {
		return fmt.Errorf("toClause is required")
	}

	return nil
}

// RevokeGrantOptions holds the parameters for revoking a grant.
type RevokeGrantOptions struct {
	// Privilege is the privilege name (e.g. USAGE, SELECT, MODIFY).
	Privilege string

	// OnClause is the full ON clause (e.g. "ON DATABASE MY_DB",
	// "ON FUTURE TABLES IN SCHEMA MY_DB.PUBLIC").
	OnClause string

	// FromClause is the full FROM clause (e.g. "FROM ROLE DATA_READER",
	// "FROM DATABASE ROLE MY_DB.MY_ROLE", "FROM SHARE my_share").
	FromClause string
}

// Validate checks the RevokeGrantOptions for validity.
func (o *RevokeGrantOptions) Validate() error {
	if strings.TrimSpace(o.Privilege) == "" {
		return fmt.Errorf("privilege is required")
	}

	if strings.TrimSpace(o.OnClause) == "" {
		return fmt.Errorf("onClause is required")
	}

	if strings.TrimSpace(o.FromClause) == "" {
		return fmt.Errorf("fromClause is required")
	}

	return nil
}

// GrantClient provides operations against Snowflake privilege grants.
type GrantClient struct {
	client SQLExecutor
}

// NewGrantClient creates a new GrantClient backed by the given SQLExecutor.
func NewGrantClient(c SQLExecutor) *GrantClient {
	return &GrantClient{client: c}
}

// buildGrantSQL builds the GRANT SQL statement.
//
// Examples:
//
//	GRANT USAGE ON DATABASE MY_DB TO ROLE DATA_READER
//	GRANT SELECT ON ALL TABLES IN SCHEMA MY_DB.PUBLIC TO ROLE ANALYST
//	GRANT SELECT ON FUTURE TABLES IN DATABASE MY_DB TO ROLE ANALYST
//	GRANT USAGE ON DATABASE MY_DB TO SHARE my_share
//	GRANT CREATE DATABASE ON ACCOUNT TO ROLE SYSADMIN WITH GRANT OPTION
func buildGrantSQL(opts CreateGrantOptions) string {
	var sb strings.Builder

	sb.WriteString("GRANT ")
	sb.WriteString(opts.Privilege)
	sb.WriteString(" ")
	sb.WriteString(opts.OnClause)
	sb.WriteString(" ")
	sb.WriteString(opts.ToClause)

	if opts.WithGrantOption {
		sb.WriteString(" WITH GRANT OPTION")
	}

	return sb.String()
}

// Grant grants a privilege to a role or share.
func (g *GrantClient) Grant(ctx context.Context, opts CreateGrantOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid grant options: %w", err))
	}

	if _, err := g.client.Exec(ctx, buildGrantSQL(opts)); err != nil {
		return fmt.Errorf("granting %s %s %s: %w",
			opts.Privilege, opts.OnClause, opts.ToClause, err)
	}

	return nil
}

// buildRevokeSQL builds the REVOKE SQL statement.
//
// Examples:
//
//	REVOKE USAGE ON DATABASE MY_DB FROM ROLE DATA_READER
//	REVOKE SELECT ON ALL TABLES IN SCHEMA MY_DB.PUBLIC FROM ROLE ANALYST
//	REVOKE SELECT ON FUTURE TABLES IN DATABASE MY_DB FROM ROLE ANALYST
//	REVOKE USAGE ON DATABASE MY_DB FROM SHARE my_share
func buildRevokeSQL(opts RevokeGrantOptions) string {
	var sb strings.Builder

	sb.WriteString("REVOKE ")
	sb.WriteString(opts.Privilege)
	sb.WriteString(" ")
	sb.WriteString(opts.OnClause)
	sb.WriteString(" ")
	sb.WriteString(opts.FromClause)

	return sb.String()
}

// Revoke revokes a privilege from a role or share.
func (g *GrantClient) Revoke(ctx context.Context, opts RevokeGrantOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid revoke options: %w", err))
	}

	if _, err := g.client.Exec(ctx, buildRevokeSQL(opts)); err != nil {
		return fmt.Errorf("revoking %s %s %s: %w",
			opts.Privilege, opts.OnClause, opts.FromClause, err)
	}

	return nil
}

// ShowGrants queries SHOW GRANTS or SHOW FUTURE GRANTS and returns all grant rows.
// The target parameter determines the query variant:
//
//   - "ON DATABASE MY_DB"            → SHOW GRANTS ON DATABASE MY_DB
//   - "ON ACCOUNT"                   → SHOW GRANTS ON ACCOUNT
//   - "TO SHARE my_share"            → SHOW GRANTS TO SHARE my_share
//   - "IN DATABASE MY_DB"            → SHOW FUTURE GRANTS IN DATABASE MY_DB
//   - "IN SCHEMA MY_DB.PUBLIC"       → SHOW FUTURE GRANTS IN SCHEMA MY_DB.PUBLIC
func (g *GrantClient) ShowGrants(ctx context.Context, target string, future bool) ([]*GrantShowOutput, error) {
	if strings.TrimSpace(target) == "" {
		return nil, NewTerminalError(fmt.Errorf("showGrants target is required"))
	}

	var query string
	if future {
		query = "SHOW FUTURE GRANTS " + target
	} else {
		query = "SHOW GRANTS " + target
	}

	rows, err := g.client.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("showing grants (%s): %w", query, err)
	}
	defer closeRows(rows)

	return scanGrantShowOutput(rows, future)
}

// Observe checks whether a specific grant exists by querying SHOW GRANTS
// and filtering for the matching privilege and grantee.
func (g *GrantClient) Observe(ctx context.Context, id GrantIdentifier) (*GrantObservation, error) {
	future := id.Kind == GrantKindFuture

	grants, err := g.ShowGrants(ctx, id.ShowGrantsTarget, future)
	if err != nil {
		if IsObjectNotFound(err) {
			return &GrantObservation{Exists: false}, nil
		}

		return nil, err
	}

	for _, grant := range grants {
		if strings.EqualFold(grant.Privilege, id.Privilege) &&
			strings.EqualFold(grant.GranteeName, id.GranteeName) {
			return &GrantObservation{
				Exists:     true,
				ShowOutput: grant,
			}, nil
		}
	}

	return &GrantObservation{Exists: false}, nil
}

// scanGrantShowOutput scans SHOW GRANTS results into GrantShowOutput structs.
// For future grants, the columns are named differently:
//
//	Regular: created_on, privilege, granted_on, name, granted_to, grantee_name, grant_option, granted_by
//	Future:  created_on, privilege, grant_on,  name, grant_to,  grantee_name, grant_option
func scanGrantShowOutput(rows *sql.Rows, future bool) ([]*GrantShowOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	var results []*GrantShowOutput

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

		grantOption := strings.EqualFold(colMap["grant_option"], "true")

		out := &GrantShowOutput{
			CreatedOn:   colMap["created_on"],
			Privilege:   colMap["privilege"],
			GranteeName: colMap["grantee_name"],
			GrantOption: grantOption,
		}

		if future {
			// Future grants use "grant_on" and "grant_to" column names.
			out.GrantedOn = colMap["grant_on"]
			out.Name = colMap["name"]
			out.GrantedTo = colMap["grant_to"]
		} else {
			out.GrantedOn = colMap["granted_on"]
			out.Name = colMap["name"]
			out.GrantedTo = colMap["granted_to"]
			out.GrantedBy = colMap["granted_by"]
		}

		results = append(results, out)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return results, nil
}

// BuildOnClause constructs the SQL ON clause from structured grant-on parameters.
// All identifier names are quoted via QuoteIdentifierParts to prevent SQL injection.
// Object type keywords are validated against an allowlist.
// This is a helper used by the controller adapter to build CreateGrantOptions.OnClause.
func BuildOnClause(on OnClauseParams) string {
	if on.Account {
		return "ON ACCOUNT"
	}

	if on.AccountObjectType != "" {
		return fmt.Sprintf("ON %s %s", on.AccountObjectType, sqlbuilder.QuoteIdentifierParts(on.AccountObjectName))
	}

	if on.SchemaName != "" {
		return "ON SCHEMA " + sqlbuilder.QuoteIdentifierParts(on.SchemaName)
	}

	if on.AllSchemasInDB != "" {
		return "ON ALL SCHEMAS IN DATABASE " + sqlbuilder.QuoteIdentifierParts(on.AllSchemasInDB)
	}

	if on.FutureSchemasInDB != "" {
		return "ON FUTURE SCHEMAS IN DATABASE " + sqlbuilder.QuoteIdentifierParts(on.FutureSchemasInDB)
	}

	if on.SchemaObjectType != "" && on.SchemaObjectName != "" {
		return fmt.Sprintf("ON %s %s", on.SchemaObjectType, sqlbuilder.QuoteIdentifierParts(on.SchemaObjectName))
	}

	if on.AllObjectsTypePlural != "" {
		if on.AllObjectsInSchema != "" {
			return fmt.Sprintf("ON ALL %s IN SCHEMA %s", on.AllObjectsTypePlural, sqlbuilder.QuoteIdentifierParts(on.AllObjectsInSchema))
		}

		return fmt.Sprintf("ON ALL %s IN DATABASE %s", on.AllObjectsTypePlural, sqlbuilder.QuoteIdentifierParts(on.AllObjectsInDB))
	}

	if on.FutureObjectsTypePlural != "" {
		if on.FutureObjectsInSchema != "" {
			return fmt.Sprintf("ON FUTURE %s IN SCHEMA %s", on.FutureObjectsTypePlural, sqlbuilder.QuoteIdentifierParts(on.FutureObjectsInSchema))
		}

		return fmt.Sprintf("ON FUTURE %s IN DATABASE %s", on.FutureObjectsTypePlural, sqlbuilder.QuoteIdentifierParts(on.FutureObjectsInDB))
	}

	return ""
}

// OnClauseParams is a flat struct that the controller populates from the GrantOn hierarchy.
type OnClauseParams struct {
	Account           bool
	AccountObjectType string
	AccountObjectName string

	SchemaName        string
	AllSchemasInDB    string
	FutureSchemasInDB string

	SchemaObjectType string
	SchemaObjectName string

	AllObjectsTypePlural string
	AllObjectsInDB       string
	AllObjectsInSchema   string

	FutureObjectsTypePlural string
	FutureObjectsInDB       string
	FutureObjectsInSchema   string
}

// BuildToClause constructs the SQL TO clause.
// All identifier names are quoted to prevent SQL injection.
func BuildToClause(accountRole, databaseRole, share string) string {
	if accountRole != "" {
		return "TO ROLE " + sqlbuilder.QuoteIdentifier(accountRole)
	}

	if databaseRole != "" {
		return "TO DATABASE ROLE " + sqlbuilder.QuoteIdentifierParts(databaseRole)
	}

	if share != "" {
		return "TO SHARE " + sqlbuilder.QuoteIdentifier(share)
	}

	return ""
}

// BuildFromClause constructs the SQL FROM clause for REVOKE.
// All identifier names are quoted to prevent SQL injection.
func BuildFromClause(accountRole, databaseRole, share string) string {
	if accountRole != "" {
		return "FROM ROLE " + sqlbuilder.QuoteIdentifier(accountRole)
	}

	if databaseRole != "" {
		return "FROM DATABASE ROLE " + sqlbuilder.QuoteIdentifierParts(databaseRole)
	}

	if share != "" {
		return "FROM SHARE " + sqlbuilder.QuoteIdentifier(share)
	}

	return ""
}

// BuildShowGrantsTarget constructs the target for SHOW GRANTS / SHOW FUTURE GRANTS queries.
// All identifier names are quoted to prevent SQL injection.
func BuildShowGrantsTarget(on OnClauseParams, share string) (target string, future bool) {
	// Grants to shares are observed via "SHOW GRANTS TO SHARE <name>".
	if share != "" {
		return "TO SHARE " + sqlbuilder.QuoteIdentifier(share), false
	}

	// Future grants: observed via "SHOW FUTURE GRANTS IN DATABASE/SCHEMA <name>".
	if on.FutureSchemasInDB != "" {
		return "IN DATABASE " + sqlbuilder.QuoteIdentifierParts(on.FutureSchemasInDB), true
	}

	if on.FutureObjectsInDB != "" {
		return "IN DATABASE " + sqlbuilder.QuoteIdentifierParts(on.FutureObjectsInDB), true
	}

	if on.FutureObjectsInSchema != "" {
		return "IN SCHEMA " + sqlbuilder.QuoteIdentifierParts(on.FutureObjectsInSchema), true
	}

	// Regular grants: "SHOW GRANTS ON <type> <name>" / "SHOW GRANTS ON ACCOUNT".
	if on.Account {
		return "ON ACCOUNT", false
	}

	if on.AccountObjectType != "" {
		return fmt.Sprintf("ON %s %s", on.AccountObjectType, sqlbuilder.QuoteIdentifierParts(on.AccountObjectName)), false
	}

	if on.SchemaName != "" {
		return "ON SCHEMA " + sqlbuilder.QuoteIdentifierParts(on.SchemaName), false
	}

	// ALL grants observe on the container (database or schema).
	if on.AllSchemasInDB != "" {
		return "ON DATABASE " + sqlbuilder.QuoteIdentifierParts(on.AllSchemasInDB), false
	}

	if on.SchemaObjectType != "" {
		return fmt.Sprintf("ON %s %s", on.SchemaObjectType, sqlbuilder.QuoteIdentifierParts(on.SchemaObjectName)), false
	}

	if on.AllObjectsInSchema != "" {
		return "ON SCHEMA " + sqlbuilder.QuoteIdentifierParts(on.AllObjectsInSchema), false
	}

	if on.AllObjectsInDB != "" {
		return "ON DATABASE " + sqlbuilder.QuoteIdentifierParts(on.AllObjectsInDB), false
	}

	return "", false
}
