package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// DatabaseRoleObservation holds the result of observing a Snowflake database role.
type DatabaseRoleObservation struct {
	// Exists indicates whether the database role was found.
	Exists bool

	// ShowOutput contains the SHOW DATABASE ROLES row.
	ShowOutput *DatabaseRoleShowOutput
}

// DatabaseRoleShowOutput contains the fields from SHOW DATABASE ROLES.
type DatabaseRoleShowOutput struct {
	CreatedOn      string
	Name           string
	DatabaseName   string
	Comment        string
	Owner          string
	GrantedToRoles int32
	GrantedRoles   int32
}

// CreateDatabaseRoleOptions holds the parameters for creating a database role.
type CreateDatabaseRoleOptions struct {
	Name    DatabaseObjectIdentifier
	Comment *string
}

// Validate checks the CreateDatabaseRoleOptions for validity.
func (o *CreateDatabaseRoleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("database role name is required")
	}

	return nil
}

// AlterDatabaseRoleOptions holds the parameters for altering a database role.
type AlterDatabaseRoleOptions struct {
	Name    DatabaseObjectIdentifier
	Comment *string

	// UnsetFields lists parameter names to revert via ALTER DATABASE ROLE ... UNSET.
	UnsetFields []string
}

// Validate checks the AlterDatabaseRoleOptions for validity.
func (o *AlterDatabaseRoleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("database role name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterDatabaseRoleOptions) HasChanges() bool {
	return o.Comment != nil || len(o.UnsetFields) > 0
}

// DatabaseRoleClient provides operations against Snowflake database roles.
type DatabaseRoleClient struct {
	client SQLExecutor
}

// NewDatabaseRoleClient creates a new DatabaseRoleClient backed by the given SQLExecutor.
func NewDatabaseRoleClient(c SQLExecutor) *DatabaseRoleClient {
	return &DatabaseRoleClient{client: c}
}

// buildCreateDatabaseRoleSQL builds the CREATE DATABASE ROLE SQL statement.
func buildCreateDatabaseRoleSQL(opts CreateDatabaseRoleOptions) string {
	var b sqlbuilder.Builder

	b.WriteString("CREATE DATABASE ROLE IF NOT EXISTS ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a database role in Snowflake.
func (r *DatabaseRoleClient) Create(ctx context.Context, opts CreateDatabaseRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create database role options: %w", err))
	}

	if _, err := r.client.Exec(ctx, buildCreateDatabaseRoleSQL(opts)); err != nil {
		return fmt.Errorf("creating database role %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterDatabaseRoleStatements builds the ALTER DATABASE ROLE SQL statement(s).
func buildAlterDatabaseRoleStatements(opts AlterDatabaseRoleOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("DATABASE ROLE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a database role in Snowflake.
func (r *DatabaseRoleClient) Alter(ctx context.Context, opts AlterDatabaseRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter database role options: %w", err))
	}

	stmts, err := buildAlterDatabaseRoleStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter database role statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := r.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering database role %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropDatabaseRoleSQL builds the DROP DATABASE ROLE SQL statement.
func buildDropDatabaseRoleSQL(name DatabaseObjectIdentifier) string {
	return sqlbuilder.DropIfExists("DATABASE ROLE", name.FullyQualifiedName())
}

// Drop drops a database role from Snowflake.
func (r *DatabaseRoleClient) Drop(ctx context.Context, name DatabaseObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("database role name is required"))
	}

	if _, err := r.client.Exec(ctx, buildDropDatabaseRoleSQL(name)); err != nil {
		return fmt.Errorf("dropping database role %s: %w", name, err)
	}

	return nil
}

// buildShowDatabaseRoleByIDSQL builds the SHOW DATABASE ROLES LIKE ... IN DATABASE SQL statement.
func buildShowDatabaseRoleByIDSQL(name DatabaseObjectIdentifier) string {
	return sqlbuilder.ShowLikeIn("DATABASE ROLES", name.Name(), "DATABASE "+sqlbuilder.QuoteIdentifier(name.DatabaseName()))
}

// ShowByID queries SHOW DATABASE ROLES for a specific role name.
func (r *DatabaseRoleClient) ShowByID(ctx context.Context, name DatabaseObjectIdentifier) (*DatabaseRoleShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database role name is required"))
	}

	rows, err := r.client.Query(ctx, buildShowDatabaseRoleByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing database role %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDatabaseRoleShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a DatabaseRoleObservation.
func (r *DatabaseRoleClient) Observe(ctx context.Context, name DatabaseObjectIdentifier) (*DatabaseRoleObservation, error) {
	show, err := r.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &DatabaseRoleObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &DatabaseRoleObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanDatabaseRoleShowOutput scans SHOW DATABASE ROLES results for a matching row.
func scanDatabaseRoleShowOutput(rows *sql.Rows, name string) (*DatabaseRoleShowOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

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

		if !strings.EqualFold(colMap["name"], name) {
			continue
		}

		var grantedToRoles, grantedRoles int32
		if v, ok := parseInt32(colMap["granted_to_roles"]); ok {
			grantedToRoles = v
		}

		if v, ok := parseInt32(colMap["granted_database_roles"]); ok {
			grantedRoles = v
		}

		return &DatabaseRoleShowOutput{
			CreatedOn:      colMap["created_on"],
			Name:           colMap["name"],
			DatabaseName:   colMap["database_name"],
			Comment:        colMap["comment"],
			Owner:          colMap["owner"],
			GrantedToRoles: grantedToRoles,
			GrantedRoles:   grantedRoles,
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
