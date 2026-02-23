package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// MaskingPolicyObservation holds the result of observing a Snowflake masking policy.
type MaskingPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW MASKING POLICIES row.
	ShowOutput *MaskingPolicyShowOutput
}

// MaskingPolicyShowOutput contains the fields from SHOW MASKING POLICIES.
type MaskingPolicyShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Kind         string
	Owner        string
	Comment      string
}

// MaskingPolicyArgument defines an argument in the masking policy signature.
type MaskingPolicyArgument struct {
	Name string
	Type string
}

// CreateMaskingPolicyOptions holds the parameters for creating a masking policy.
type CreateMaskingPolicyOptions struct {
	Name                SchemaObjectIdentifier
	Signature           []MaskingPolicyArgument
	Body                string
	ExemptOtherPolicies *bool
	Comment             *string
}

// Validate checks the CreateMaskingPolicyOptions for validity.
func (o *CreateMaskingPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("masking policy name is required")
	}

	if len(o.Signature) == 0 {
		return fmt.Errorf("masking policy signature requires at least one argument")
	}

	if o.Body == "" {
		return fmt.Errorf("masking policy body is required")
	}

	return nil
}

// AlterMaskingPolicyOptions holds the parameters for altering a masking policy.
type AlterMaskingPolicyOptions struct {
	Name    SchemaObjectIdentifier
	Body    *string
	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterMaskingPolicyOptions for validity.
func (o *AlterMaskingPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("masking policy name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterMaskingPolicyOptions) HasChanges() bool {
	return o.Body != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// MaskingPolicyClient provides operations against Snowflake masking policies.
type MaskingPolicyClient struct {
	client SQLExecutor
}

// NewMaskingPolicyClient creates a new MaskingPolicyClient backed by the given SQLExecutor.
func NewMaskingPolicyClient(c SQLExecutor) *MaskingPolicyClient {
	return &MaskingPolicyClient{client: c}
}

// buildSignatureClause builds the AS (arg1 type1, arg2 type2, ...) RETURNS type1 clause.
func buildSignatureClause(args []MaskingPolicyArgument) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%s %s", arg.Name, arg.Type)
	}

	returnType := args[0].Type

	return fmt.Sprintf("AS (%s) RETURNS %s", strings.Join(parts, ", "), returnType)
}

// buildCreateMaskingPolicySQL builds the CREATE MASKING POLICY SQL statement.
func buildCreateMaskingPolicySQL(opts CreateMaskingPolicyOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("CREATE MASKING POLICY IF NOT EXISTS ")
	b.WriteString(opts.Name.FullyQualifiedName())
	b.WriteString(" ")
	b.WriteString(buildSignatureClause(opts.Signature))
	b.WriteString(" -> ")
	b.WriteString(opts.Body)

	if opts.Comment != nil {
		b.SetString("COMMENT", opts.Comment)
	}

	if opts.ExemptOtherPolicies != nil && *opts.ExemptOtherPolicies {
		b.WriteString(" EXEMPT_OTHER_POLICIES = TRUE")
	}

	return b.String()
}

// Create creates a masking policy in Snowflake.
func (mp *MaskingPolicyClient) Create(ctx context.Context, opts CreateMaskingPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create masking policy options: %w", err))
	}

	if _, err := mp.client.Exec(ctx, buildCreateMaskingPolicySQL(opts)); err != nil {
		return fmt.Errorf("creating masking policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterMaskingPolicyStatements builds the ALTER MASKING POLICY SQL statements.
func buildAlterMaskingPolicyStatements(opts AlterMaskingPolicyOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// Body is a separate ALTER SET BODY statement.
	if opts.Body != nil {
		statements = append(statements, fmt.Sprintf("ALTER MASKING POLICY %s SET BODY -> %s", fqn, *opts.Body))
	}

	// Comment uses SET/UNSET via BuildAlterStatements.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("MASKING POLICY", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a masking policy in Snowflake.
func (mp *MaskingPolicyClient) Alter(ctx context.Context, opts AlterMaskingPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter masking policy options: %w", err))
	}

	stmts, err := buildAlterMaskingPolicyStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter masking policy statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := mp.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering masking policy %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a masking policy from Snowflake.
func (mp *MaskingPolicyClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("masking policy name is required"))
	}

	stmt := sqlbuilder.DropIfExists("MASKING POLICY", name.FullyQualifiedName())

	if _, err := mp.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping masking policy %s: %w", name, err)
	}

	return nil
}

// buildShowMaskingPolicyByIDSQL builds a SHOW MASKING POLICIES LIKE ... IN SCHEMA SQL statement.
func buildShowMaskingPolicyByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("MASKING POLICIES", name.Name(), scope)
}

// ShowByID queries SHOW MASKING POLICIES for a specific policy.
func (mp *MaskingPolicyClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*MaskingPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("masking policy name is required"))
	}

	rows, err := mp.client.Query(ctx, buildShowMaskingPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing masking policy %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanMaskingPolicyShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a MaskingPolicyObservation.
func (mp *MaskingPolicyClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*MaskingPolicyObservation, error) {
	show, err := mp.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &MaskingPolicyObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &MaskingPolicyObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanMaskingPolicyShowOutput scans SHOW MASKING POLICIES results for a matching row.
func scanMaskingPolicyShowOutput(rows *sql.Rows, name string) (*MaskingPolicyShowOutput, error) {
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

		return &MaskingPolicyShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["database_name"],
			SchemaName:   colMap["schema_name"],
			Kind:         colMap["kind"],
			Owner:        colMap["owner"],
			Comment:      colMap["comment"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
