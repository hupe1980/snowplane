package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// AccountRoleObservation holds the result of observing a Snowflake role.
type AccountRoleObservation struct {
	// Exists indicates whether the role was found.
	Exists bool

	// ShowOutput contains the SHOW ROLES row.
	ShowOutput *AccountRoleShowOutput
}

// AccountRoleShowOutput contains the fields from SHOW ROLES.
type AccountRoleShowOutput struct {
	CreatedOn      string
	Name           string
	Comment        string
	Owner          string
	GrantedToRoles int32
	GrantedRoles   int32
}

// CreateAccountRoleOptions holds the parameters for creating a role.
type CreateAccountRoleOptions struct {
	Name    AccountObjectIdentifier
	Comment *string
}

// Validate checks the CreateAccountRoleOptions for validity.
func (o *CreateAccountRoleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("role name is required")
	}

	return nil
}

// AlterAccountRoleOptions holds the parameters for altering a role.
type AlterAccountRoleOptions struct {
	Name    AccountObjectIdentifier
	Comment *string

	// UnsetFields lists parameter names to revert via ALTER ROLE ... UNSET.
	UnsetFields []string
}

// Validate checks the AlterAccountRoleOptions for validity.
func (o *AlterAccountRoleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("role name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterAccountRoleOptions) HasChanges() bool {
	return o.Comment != nil || len(o.UnsetFields) > 0
}

// AccountRoleClient provides operations against Snowflake account roles.
type AccountRoleClient struct {
	client SQLExecutor
}

// NewAccountRoleClient creates a new AccountRoleClient backed by the given SQLExecutor.
func NewAccountRoleClient(c SQLExecutor) *AccountRoleClient {
	return &AccountRoleClient{client: c}
}

// buildCreateAccountRoleSQL builds the CREATE ROLE SQL statement.
func buildCreateAccountRoleSQL(opts CreateAccountRoleOptions) string {
	var b sqlbuilder.Builder

	b.WriteString("CREATE ROLE IF NOT EXISTS ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a role in Snowflake.
func (r *AccountRoleClient) Create(ctx context.Context, opts CreateAccountRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create account role options: %w", err))
	}

	if _, err := r.client.Exec(ctx, buildCreateAccountRoleSQL(opts)); err != nil {
		return fmt.Errorf("creating role %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAccountRoleStatements builds the ALTER ROLE SQL statement(s).
func buildAlterAccountRoleStatements(opts AlterAccountRoleOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("ROLE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a role in Snowflake.
func (r *AccountRoleClient) Alter(ctx context.Context, opts AlterAccountRoleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter account role options: %w", err))
	}

	stmts, err := buildAlterAccountRoleStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter role statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := r.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering role %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropAccountRoleSQL builds the DROP ROLE SQL statement.
func buildDropAccountRoleSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.DropIfExists("ROLE", name.FullyQualifiedName())
}

// Drop drops a role from Snowflake.
func (r *AccountRoleClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("role name is required"))
	}

	if _, err := r.client.Exec(ctx, buildDropAccountRoleSQL(name)); err != nil {
		return fmt.Errorf("dropping role %s: %w", name, err)
	}

	return nil
}

// buildShowAccountRoleByIDSQL builds the SHOW ROLES LIKE SQL statement.
func buildShowAccountRoleByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("ROLES", name.Name())
}

// ShowByID queries SHOW ROLES for a specific role name.
func (r *AccountRoleClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*AccountRoleShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("role name is required"))
	}

	rows, err := r.client.Query(ctx, buildShowAccountRoleByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing role %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanAccountRoleShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an AccountRoleObservation.
func (r *AccountRoleClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*AccountRoleObservation, error) {
	show, err := r.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &AccountRoleObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &AccountRoleObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanAccountRoleShowOutput scans SHOW ROLES results for a matching row.
func scanAccountRoleShowOutput(rows *sql.Rows, name string) (*AccountRoleShowOutput, error) {
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

		if v, ok := parseInt32(colMap["granted_roles"]); ok {
			grantedRoles = v
		}

		return &AccountRoleShowOutput{
			CreatedOn:      colMap["created_on"],
			Name:           colMap["name"],
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
