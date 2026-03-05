package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// RowAccessPolicyObservation holds the result of observing a Snowflake row access policy.
type RowAccessPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW ROW ACCESS POLICIES row.
	ShowOutput *v1alpha1.RowAccessPolicyShowOutput
}

// RowAccessPolicyArgument defines an argument in the row access policy signature.
type RowAccessPolicyArgument struct {
	Name string
	Type string
}

// CreateRowAccessPolicyOptions holds the parameters for creating a row access policy.
type CreateRowAccessPolicyOptions struct {
	Name      SchemaObjectIdentifier
	Signature []RowAccessPolicyArgument
	Body      string
	Comment   *string

	// UseCreateOrAlter emits CREATE OR ALTER ROW ACCESS POLICY instead of
	// CREATE ROW ACCESS POLICY IF NOT EXISTS.
	UseCreateOrAlter bool
}

// Validate checks the CreateRowAccessPolicyOptions for validity.
func (o *CreateRowAccessPolicyOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("row access policy name is required"))
	}

	if len(o.Signature) == 0 {
		errs = append(errs, fmt.Errorf("row access policy signature requires at least one argument"))
	}

	if o.Body == "" {
		errs = append(errs, fmt.Errorf("row access policy body is required"))
	} else if err := sqlbuilder.ValidatePolicyBody(o.Body); err != nil {
		errs = append(errs, fmt.Errorf("invalid row access policy body: %w", err))
	}

	return errors.Join(errs...)
}

// AlterRowAccessPolicyOptions holds the parameters for altering a row access policy.
type AlterRowAccessPolicyOptions struct {
	Name    SchemaObjectIdentifier
	Body    *string
	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterRowAccessPolicyOptions for validity.
func (o *AlterRowAccessPolicyOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("row access policy name is required"))
	}

	if o.Body != nil {
		if err := sqlbuilder.ValidatePolicyBody(*o.Body); err != nil {
			errs = append(errs, fmt.Errorf("invalid row access policy body: %w", err))
		}
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterRowAccessPolicyOptions) HasChanges() bool {
	return o.Body != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// RowAccessPolicyClient provides operations against Snowflake row access policies.
type RowAccessPolicyClient struct {
	client SQLExecutor
}

// NewRowAccessPolicyClient creates a new RowAccessPolicyClient backed by the given SQLExecutor.
func NewRowAccessPolicyClient(c SQLExecutor) *RowAccessPolicyClient {
	return &RowAccessPolicyClient{client: c}
}

// buildRAPSignatureClause builds the AS (arg1 type1, arg2 type2) RETURNS BOOLEAN clause.
func buildRAPSignatureClause(args []RowAccessPolicyArgument) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%s %s", arg.Name, arg.Type)
	}

	return fmt.Sprintf("AS (%s) RETURNS BOOLEAN", strings.Join(parts, ", "))
}

// buildCreateRowAccessPolicySQL builds the CREATE ROW ACCESS POLICY SQL statement.
func buildCreateRowAccessPolicySQL(opts CreateRowAccessPolicyOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "ROW ACCESS POLICY", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)
	b.WriteString(" ")
	b.WriteString(buildRAPSignatureClause(opts.Signature))
	b.WriteString(" -> ")
	b.WriteString(opts.Body)

	if opts.Comment != nil {
		b.SetString("COMMENT", opts.Comment)
	}

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a row access policy in Snowflake.
func (rap *RowAccessPolicyClient) Create(ctx context.Context, opts CreateRowAccessPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create row access policy options: %w", err))
	}

	sql, err := buildCreateRowAccessPolicySQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create row access policy SQL: %w", err))
	}

	if _, err := rap.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating row access policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterRowAccessPolicyStatements builds the ALTER ROW ACCESS POLICY SQL statements.
func buildAlterRowAccessPolicyStatements(opts AlterRowAccessPolicyOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// Body is a separate ALTER SET BODY statement.
	if opts.Body != nil {
		statements = append(statements, fmt.Sprintf("ALTER ROW ACCESS POLICY %s SET BODY -> %s", fqn, *opts.Body))
	}

	// Comment uses SET/UNSET via BuildAlterStatements.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("ROW ACCESS POLICY", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a row access policy in Snowflake.
func (rap *RowAccessPolicyClient) Alter(ctx context.Context, opts AlterRowAccessPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter row access policy options: %w", err))
	}

	stmts, err := buildAlterRowAccessPolicyStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter row access policy statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := rap.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering row access policy %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a row access policy from Snowflake.
func (rap *RowAccessPolicyClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("row access policy name is required"))
	}

	stmt := sqlbuilder.DropIfExists("ROW ACCESS POLICY", name.FullyQualifiedName())

	if _, err := rap.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping row access policy %s: %w", name, err)
	}

	return nil
}

// buildShowRowAccessPolicyByIDSQL builds a SHOW ROW ACCESS POLICIES LIKE ... IN SCHEMA SQL statement.
func buildShowRowAccessPolicyByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("ROW ACCESS POLICIES", name.Name(), scope)
}

// ShowByID queries SHOW ROW ACCESS POLICIES for a specific policy.
func (rap *RowAccessPolicyClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.RowAccessPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("row access policy name is required"))
	}

	rows, err := rap.client.Query(ctx, buildShowRowAccessPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing row access policy %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanRowAccessPolicyShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a RowAccessPolicyObservation.
func (rap *RowAccessPolicyClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*RowAccessPolicyObservation, error) {
	show, err := rap.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &RowAccessPolicyObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &RowAccessPolicyObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanRowAccessPolicyShowOutput scans SHOW ROW ACCESS POLICIES results for a matching row.
func scanRowAccessPolicyShowOutput(rows *sql.Rows, name string) (*v1alpha1.RowAccessPolicyShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.RowAccessPolicyShowOutput, error) {
		return &v1alpha1.RowAccessPolicyShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Kind:         m["kind"],
			Owner:        m["owner"],
			Comment:      m["comment"],
		}, nil
	})
}
