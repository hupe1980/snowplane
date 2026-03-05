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

// ProcedureObservation holds the result of observing a Snowflake procedure.
type ProcedureObservation struct {
	// Exists indicates whether the procedure was found.
	Exists bool

	// ShowOutput contains the SHOW PROCEDURES row.
	ShowOutput *v1alpha1.ProcedureShowOutput
}

// ProcedureArgument defines an argument in the procedure signature.
type ProcedureArgument struct {
	Name string
	Type string
}

// CreateProcedureOptions holds the parameters for creating a procedure.
type CreateProcedureOptions struct {
	Name                       SchemaObjectIdentifier
	Arguments                  []ProcedureArgument
	Returns                    string
	Language                   string
	Body                       *string
	Handler                    *string
	RuntimeVersion             *string
	Packages                   []string
	Imports                    []string
	TargetPath                 *string
	ExternalAccessIntegrations []string
	Secrets                    map[string]string // variable name -> secret FQN
	ExecuteAs                  *string
	NullInputBehavior          *string
	Secure                     bool
	Comment                    *string

	// UseCreateOrAlter emits CREATE OR ALTER PROCEDURE instead of
	// CREATE PROCEDURE IF NOT EXISTS.
	UseCreateOrAlter bool
}

// Validate checks the CreateProcedureOptions for validity.
func (o *CreateProcedureOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("procedure name is required"))
	}

	if o.Returns == "" {
		errs = append(errs, fmt.Errorf("procedure return type is required"))
	}

	if o.Language == "" {
		errs = append(errs, fmt.Errorf("procedure language is required"))
	}

	if o.Body != nil {
		if err := sqlbuilder.ValidateDollarQuotedValue(*o.Body); err != nil {
			errs = append(errs, fmt.Errorf("invalid procedure body: %w", err))
		}
	}

	return errors.Join(errs...)
}

// AlterProcedureOptions holds the parameters for altering a procedure.
type AlterProcedureOptions struct {
	Name     SchemaObjectIdentifier
	ArgTypes []string // Argument types for identifying the procedure overload.
	Comment  *string

	// ExecuteAs sets the execution context: OWNER or CALLER.
	ExecuteAs *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterProcedureOptions for validity.
func (o *AlterProcedureOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("procedure name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterProcedureOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.ExecuteAs != nil ||
		len(o.UnsetFields) > 0
}

// ProcedureClient provides operations against Snowflake procedures.
type ProcedureClient struct {
	client SQLExecutor
}

// NewProcedureClient creates a new ProcedureClient backed by the given SQLExecutor.
func NewProcedureClient(c SQLExecutor) *ProcedureClient {
	return &ProcedureClient{client: c}
}

// buildArgSignature returns the parenthesized argument type list (e.g. "(VARCHAR, NUMBER)").
func buildArgSignature(argTypes []string) string {
	return "(" + strings.Join(argTypes, ", ") + ")"
}

// buildArgClause builds the argument clause for CREATE PROCEDURE.
func buildArgClause(args []ProcedureArgument) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%s %s", sqlbuilder.QuoteIdentifier(arg.Name), arg.Type)
	}

	return "(" + strings.Join(parts, ", ") + ")"
}

// buildCreateProcedureSQL builds the CREATE PROCEDURE SQL statement.
func buildCreateProcedureSQL(opts CreateProcedureOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "PROCEDURE", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)
	b.WriteString(buildArgClause(opts.Arguments))

	b.WriteString(" RETURNS ")
	b.WriteString(opts.Returns)

	b.WriteString(" LANGUAGE ")
	b.WriteString(opts.Language)

	if opts.RuntimeVersion != nil {
		fmt.Fprintf(&b.Builder, " RUNTIME_VERSION = '%s'", sqlbuilder.EscapeString(*opts.RuntimeVersion))
	}

	if len(opts.Packages) > 0 {
		quoted := make([]string, len(opts.Packages))
		for i, p := range opts.Packages {
			quoted[i] = "'" + sqlbuilder.EscapeString(p) + "'"
		}

		fmt.Fprintf(&b.Builder, " PACKAGES = (%s)", strings.Join(quoted, ", "))
	}

	if len(opts.Imports) > 0 {
		quoted := make([]string, len(opts.Imports))
		for i, imp := range opts.Imports {
			quoted[i] = "'" + sqlbuilder.EscapeString(imp) + "'"
		}

		fmt.Fprintf(&b.Builder, " IMPORTS = (%s)", strings.Join(quoted, ", "))
	}

	if opts.Handler != nil {
		fmt.Fprintf(&b.Builder, " HANDLER = '%s'", sqlbuilder.EscapeString(*opts.Handler))
	}

	if opts.TargetPath != nil {
		fmt.Fprintf(&b.Builder, " TARGET_PATH = '%s'", sqlbuilder.EscapeString(*opts.TargetPath))
	}

	if len(opts.ExternalAccessIntegrations) > 0 {
		fmt.Fprintf(&b.Builder, " EXTERNAL_ACCESS_INTEGRATIONS = (%s)", strings.Join(opts.ExternalAccessIntegrations, ", "))
	}

	if len(opts.Secrets) > 0 {
		pairs := make([]string, 0, len(opts.Secrets))
		for varName, secretFQN := range opts.Secrets {
			pairs = append(pairs, fmt.Sprintf("'%s' = %s", sqlbuilder.EscapeString(varName), secretFQN))
		}

		fmt.Fprintf(&b.Builder, " SECRETS = (%s)", strings.Join(pairs, ", "))
	}

	if opts.NullInputBehavior != nil {
		b.WriteString(" " + *opts.NullInputBehavior)
	}

	if opts.Secure {
		b.WriteString(" SECURE")
	}

	if opts.ExecuteAs != nil {
		b.WriteString(" EXECUTE AS " + *opts.ExecuteAs)
	}

	b.SetString("COMMENT", opts.Comment)

	if opts.Body != nil {
		b.WriteString(" AS $$")
		b.WriteString(*opts.Body)
		b.WriteString("$$")
	}

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a procedure in Snowflake.
func (p *ProcedureClient) Create(ctx context.Context, opts CreateProcedureOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create procedure options: %w", err))
	}

	sql, err := buildCreateProcedureSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create procedure SQL: %w", err))
	}

	if _, err := p.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating procedure %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterProcedureStatements builds the ALTER PROCEDURE SQL statements.
func buildAlterProcedureStatements(opts AlterProcedureOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName() + buildArgSignature(opts.ArgTypes)

	// ExecuteAs is a separate SET statement.
	if opts.ExecuteAs != nil {
		statements = append(statements, fmt.Sprintf("ALTER PROCEDURE %s SET EXECUTE AS %s", fqn, *opts.ExecuteAs))
	}

	// Comment uses SET/UNSET via BuildAlterStatements.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("PROCEDURE", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a procedure in Snowflake.
func (p *ProcedureClient) Alter(ctx context.Context, opts AlterProcedureOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter procedure options: %w", err))
	}

	stmts, err := buildAlterProcedureStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter procedure statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := p.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering procedure %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a procedure from Snowflake.
func (p *ProcedureClient) Drop(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("procedure name is required"))
	}

	stmt := sqlbuilder.DropIfExists("PROCEDURE", name.FullyQualifiedName()+buildArgSignature(argTypes))

	if _, err := p.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping procedure %s: %w", name, err)
	}

	return nil
}

// buildShowProcedureByIDSQL builds a SHOW PROCEDURES LIKE ... IN SCHEMA SQL statement.
func buildShowProcedureByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("PROCEDURES", name.Name(), scope)
}

// ShowByID queries SHOW PROCEDURES for a specific procedure.
func (p *ProcedureClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) (*v1alpha1.ProcedureShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("procedure name is required"))
	}

	rows, err := p.client.Query(ctx, buildShowProcedureByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing procedure %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanProcedureShowOutput(rows, name.Name(), argTypes)
}

// Observe combines ShowByID into a ProcedureObservation.
func (p *ProcedureClient) Observe(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) (*ProcedureObservation, error) {
	show, err := p.ShowByID(ctx, name, argTypes)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ProcedureObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ProcedureObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// matchProcedureArgTypes checks if the arguments column from SHOW PROCEDURES
// matches the expected argument types. The arguments column format is like:
// "MY_PROC(VARCHAR, NUMBER) RETURN VARCHAR"
func matchProcedureArgTypes(argumentsCol string, name string, argTypes []string) bool {
	// Extract the part between parentheses from the arguments column.
	upper := strings.ToUpper(argumentsCol)
	nameUpper := strings.ToUpper(name)

	// Find the arguments portion after the procedure name.
	idx := strings.Index(upper, nameUpper)
	if idx < 0 {
		return false
	}

	rest := argumentsCol[idx+len(name):]

	// Extract content between first pair of parentheses.
	openIdx := strings.Index(rest, "(")
	closeIdx := strings.Index(rest, ")")

	if openIdx < 0 || closeIdx < 0 || closeIdx <= openIdx {
		// No parentheses found; matches if no arguments are expected.
		return len(argTypes) == 0
	}

	argsStr := rest[openIdx+1 : closeIdx]

	if strings.TrimSpace(argsStr) == "" {
		return len(argTypes) == 0
	}

	showTypes := strings.Split(argsStr, ",")
	if len(showTypes) != len(argTypes) {
		return false
	}

	for i, st := range showTypes {
		if !strings.EqualFold(strings.TrimSpace(st), strings.TrimSpace(argTypes[i])) {
			return false
		}
	}

	return true
}

// scanProcedureShowOutput scans SHOW PROCEDURES results for a matching row.
func scanProcedureShowOutput(rows *sql.Rows, name string, argTypes []string) (*v1alpha1.ProcedureShowOutput, error) {
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

		// Match by argument types to handle overloading.
		if !matchProcedureArgTypes(colMap["arguments"], name, argTypes) {
			continue
		}

		return &v1alpha1.ProcedureShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["catalog_name"],
			SchemaName:   colMap["schema_name"],
			Arguments:    colMap["arguments"],
			Description:  colMap["description"],
			IsSecure:     strings.EqualFold(colMap["is_secure"], "Y"),
			Owner:        colMap["owner"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
