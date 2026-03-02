package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// FunctionObservation holds the result of observing a Snowflake function.
type FunctionObservation struct {
	// Exists indicates whether the function was found.
	Exists bool

	// ShowOutput contains the SHOW USER FUNCTIONS row.
	ShowOutput *FunctionShowOutput
}

// FunctionShowOutput contains the fields from SHOW USER FUNCTIONS.
type FunctionShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Arguments    string
	Description  string
	Language     string
	IsSecure     bool
	Owner        string
}

// FunctionArgument defines an argument in the function signature.
type FunctionArgument struct {
	Name string
	Type string
}

// CreateFunctionOptions holds the parameters for creating a function.
type CreateFunctionOptions struct {
	Name                       SchemaObjectIdentifier
	Arguments                  []FunctionArgument
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
	NullInputBehavior          *string
	Volatility                 *string
	Secure                     bool
	Comment                    *string

	// UseCreateOrAlter emits CREATE OR ALTER FUNCTION instead of
	// CREATE FUNCTION IF NOT EXISTS.
	UseCreateOrAlter bool
}

// Validate checks the CreateFunctionOptions for validity.
func (o *CreateFunctionOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("function name is required"))
	}

	if o.Returns == "" {
		errs = append(errs, fmt.Errorf("function return type is required"))
	}

	if o.Language == "" {
		errs = append(errs, fmt.Errorf("function language is required"))
	}

	if o.Body != nil {
		if err := sqlbuilder.ValidateDollarQuotedValue(*o.Body); err != nil {
			errs = append(errs, fmt.Errorf("invalid function body: %w", err))
		}
	}

	return errors.Join(errs...)
}

// AlterFunctionOptions holds the parameters for altering a function.
type AlterFunctionOptions struct {
	Name     SchemaObjectIdentifier
	ArgTypes []string // Argument types for identifying the function overload.
	Comment  *string
	Secure   *bool

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterFunctionOptions for validity.
func (o *AlterFunctionOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("function name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterFunctionOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.Secure != nil ||
		len(o.UnsetFields) > 0
}

// FunctionClient provides operations against Snowflake functions.
type FunctionClient struct {
	client SQLExecutor
}

// NewFunctionClient creates a new FunctionClient backed by the given SQLExecutor.
func NewFunctionClient(c SQLExecutor) *FunctionClient {
	return &FunctionClient{client: c}
}

// buildFuncArgClause builds the argument clause for CREATE FUNCTION.
func buildFuncArgClause(args []FunctionArgument) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%s %s", sqlbuilder.QuoteIdentifier(arg.Name), arg.Type)
	}

	return "(" + strings.Join(parts, ", ") + ")"
}

// buildCreateFunctionSQL builds the CREATE FUNCTION SQL statement.
func buildCreateFunctionSQL(opts CreateFunctionOptions) string {
	var b sqlbuilder.Builder

	if opts.UseCreateOrAlter {
		b.WriteString("CREATE OR ALTER FUNCTION ")
	} else {
		b.WriteString("CREATE FUNCTION IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())
	b.WriteString(buildFuncArgClause(opts.Arguments))

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

	if opts.Volatility != nil {
		b.WriteString(" " + *opts.Volatility)
	}

	if opts.Secure {
		b.WriteString(" SECURE")
	}

	b.SetString("COMMENT", opts.Comment)

	if opts.Body != nil {
		b.WriteString(" AS $$")
		b.WriteString(*opts.Body)
		b.WriteString("$$")
	}

	return b.String()
}

// Create creates a function in Snowflake.
func (f *FunctionClient) Create(ctx context.Context, opts CreateFunctionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create function options: %w", err))
	}

	if _, err := f.client.Exec(ctx, buildCreateFunctionSQL(opts)); err != nil {
		return fmt.Errorf("creating function %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterFunctionStatements builds the ALTER FUNCTION SQL statements.
func buildAlterFunctionStatements(opts AlterFunctionOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName() + buildArgSignature(opts.ArgTypes)

	// Handle SECURE as a separate SET/UNSET.
	if opts.Secure != nil {
		if *opts.Secure {
			statements = append(statements, fmt.Sprintf("ALTER FUNCTION %s SET SECURE", fqn))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER FUNCTION %s UNSET SECURE", fqn))
		}
	}

	// Comment uses SET/UNSET via BuildAlterStatements.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("FUNCTION", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a function in Snowflake.
func (f *FunctionClient) Alter(ctx context.Context, opts AlterFunctionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter function options: %w", err))
	}

	stmts, err := buildAlterFunctionStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter function statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := f.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering function %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a function from Snowflake.
func (f *FunctionClient) Drop(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("function name is required"))
	}

	stmt := sqlbuilder.DropIfExists("FUNCTION", name.FullyQualifiedName()+buildArgSignature(argTypes))

	if _, err := f.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping function %s: %w", name, err)
	}

	return nil
}

// buildShowFunctionByIDSQL builds a SHOW USER FUNCTIONS LIKE ... IN SCHEMA SQL statement.
func buildShowFunctionByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("USER FUNCTIONS", name.Name(), scope)
}

// ShowByID queries SHOW USER FUNCTIONS for a specific function.
func (f *FunctionClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) (*FunctionShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("function name is required"))
	}

	rows, err := f.client.Query(ctx, buildShowFunctionByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing function %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanFunctionShowOutput(rows, name.Name(), argTypes)
}

// Observe combines ShowByID into a FunctionObservation.
func (f *FunctionClient) Observe(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) (*FunctionObservation, error) {
	show, err := f.ShowByID(ctx, name, argTypes)
	if err != nil {
		if IsObjectNotFound(err) {
			return &FunctionObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &FunctionObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanFunctionShowOutput scans SHOW USER FUNCTIONS results for a matching row.
func scanFunctionShowOutput(rows *sql.Rows, name string, argTypes []string) (*FunctionShowOutput, error) {
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

		return &FunctionShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["catalog_name"],
			SchemaName:   colMap["schema_name"],
			Arguments:    colMap["arguments"],
			Description:  colMap["description"],
			Language:     colMap["language"],
			IsSecure:     strings.EqualFold(colMap["is_secure"], "Y"),
			Owner:        colMap["owner"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
