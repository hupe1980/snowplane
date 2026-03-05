package snowflake

import (
	"context"
	"database/sql"
	"fmt"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// DatabaseRoleObservation holds the result of observing a Snowflake database role.
type DatabaseRoleObservation struct {
	// Exists indicates whether the database role was found.
	Exists bool

	// ShowOutput contains the SHOW DATABASE ROLES row.
	ShowOutput *v1alpha1.DatabaseRoleShowOutput
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
func buildCreateDatabaseRoleSQL(opts CreateDatabaseRoleOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "DATABASE ROLE", opts.Name.FullyQualifiedName(), false, false)

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a database role in Snowflake.
func (r *DatabaseRoleClient) Create(ctx context.Context, opts CreateDatabaseRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create database role options: %w", err))
	}

	sql, err := buildCreateDatabaseRoleSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create database role SQL: %w", err))
	}

	if _, err := r.client.Exec(ctx, sql); err != nil {
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
func (r *DatabaseRoleClient) ShowByID(ctx context.Context, name DatabaseObjectIdentifier) (*v1alpha1.DatabaseRoleShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database role name is required"))
	}

	rows, err := r.client.Query(ctx, buildShowDatabaseRoleByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing database role %s: %w", name, err)
	}
	defer closeRows(rows)

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
func scanDatabaseRoleShowOutput(rows *sql.Rows, name string) (*v1alpha1.DatabaseRoleShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.DatabaseRoleShowOutput, error) {
		grantedToRoles, _ := parseInt32(m["granted_to_roles"])
		grantedRoles, _ := parseInt32(m["granted_database_roles"])

		return &v1alpha1.DatabaseRoleShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			DatabaseName:   m["database_name"],
			Comment:        m["comment"],
			Owner:          m["owner"],
			GrantedToRoles: grantedToRoles,
			GrantedRoles:   grantedRoles,
		}, nil
	})
}
