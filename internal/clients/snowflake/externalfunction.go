package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ExternalFunctionObservation holds the result of observing a Snowflake external function.
type ExternalFunctionObservation struct {
	// Exists indicates whether the external function was found.
	Exists bool

	// ShowOutput contains the SHOW EXTERNAL FUNCTIONS row.
	ShowOutput *v1alpha1.ExternalFunctionShowOutput
}

// CreateExternalFunctionOptions holds the parameters for creating an external function.
type CreateExternalFunctionOptions struct {
	Name               SchemaObjectIdentifier
	Args               []v1alpha1.ExternalFunctionArg
	ReturnType         string
	ReturnNullValues   *bool
	ReturnBehavior     *string
	APIIntegration     string
	URL                string
	Headers            []v1alpha1.ExternalFunctionHeader
	MaxBatchRows       *int32
	Compression        *v1alpha1.ExternalFunctionCompression
	RequestTranslator  *string
	ResponseTranslator *string
	Comment            *string
}

// Validate checks the CreateExternalFunctionOptions for validity.
func (o *CreateExternalFunctionOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("external function name is required")
	}

	if o.ReturnType == "" {
		return fmt.Errorf("return type is required")
	}

	if o.APIIntegration == "" {
		return fmt.Errorf("API integration is required")
	}

	if o.URL == "" {
		return fmt.Errorf("URL is required")
	}

	return nil
}

// AlterExternalFunctionOptions holds the parameters for altering an external function.
type AlterExternalFunctionOptions struct {
	Name        SchemaObjectIdentifier
	ArgTypes    []string
	Comment     *string
	UnsetFields []string
}

// Validate checks the AlterExternalFunctionOptions for validity.
func (o *AlterExternalFunctionOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("external function name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterExternalFunctionOptions) HasChanges() bool {
	return o.Comment != nil || len(o.UnsetFields) > 0
}

// ExternalFunctionClient provides operations against Snowflake external functions.
type ExternalFunctionClient struct {
	client SQLExecutor
}

// NewExternalFunctionClient creates a new ExternalFunctionClient backed by the given SQLExecutor.
func NewExternalFunctionClient(c SQLExecutor) *ExternalFunctionClient {
	return &ExternalFunctionClient{client: c}
}

// buildShowExternalFunctionByIDSQL builds a SHOW EXTERNAL FUNCTIONS LIKE query scoped to a schema.
func buildShowExternalFunctionByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("EXTERNAL FUNCTIONS", name.Name(), scope)
}

// ShowByID queries SHOW EXTERNAL FUNCTIONS for a specific function within a schema.
func (c *ExternalFunctionClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.ExternalFunctionShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("external function name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowExternalFunctionByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing external function %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanExternalFunctionShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an ExternalFunctionObservation.
func (c *ExternalFunctionClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ExternalFunctionObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) || IsObjectNotExistOrNotAuthorized(err) {
			return &ExternalFunctionObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ExternalFunctionObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanExternalFunctionShowOutput scans SHOW EXTERNAL FUNCTIONS results for a matching row.
func scanExternalFunctionShowOutput(rows *sql.Rows, name string) (*v1alpha1.ExternalFunctionShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ExternalFunctionShowOutput, error) {
		return &v1alpha1.ExternalFunctionShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			SchemaName:     m["schema_name"],
			DatabaseName:   m["database_name"],
			Language:       m["language"],
			IsExternalFunc: m["is_external_function"],
			Arguments:      m["arguments"],
			Description:    m["description"],
		}, nil
	})
}

// Create creates an external function in Snowflake.
func (c *ExternalFunctionClient) Create(ctx context.Context, opts CreateExternalFunctionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create external function options: %w", err))
	}

	var b strings.Builder

	fmt.Fprintf(&b, "CREATE EXTERNAL FUNCTION %s(", opts.Name.FullyQualifiedName())

	for i, arg := range opts.Args {
		if i > 0 {
			b.WriteString(", ")
		}

		if err := sqlbuilder.ValidateColumnType(arg.Type); err != nil {
			return NewTerminalError(fmt.Errorf("invalid arg type for %q: %w", arg.Name, err))
		}

		fmt.Fprintf(&b, "%s %s", sqlbuilder.QuoteIdentifier(arg.Name), arg.Type)
	}

	if err := sqlbuilder.ValidateColumnType(opts.ReturnType); err != nil {
		return NewTerminalError(fmt.Errorf("invalid return type %q: %w", opts.ReturnType, err))
	}

	fmt.Fprintf(&b, ")\n  RETURNS %s", opts.ReturnType)

	if opts.ReturnNullValues != nil {
		if *opts.ReturnNullValues {
			b.WriteString("\n  RETURNS NULL ON NULL INPUT")
		} else {
			b.WriteString("\n  NOT NULL")
		}
	}

	if opts.ReturnBehavior != nil {
		if err := sqlbuilder.ValidateKeywordValue(*opts.ReturnBehavior); err != nil {
			return NewTerminalError(fmt.Errorf("invalid return behavior %q: %w", *opts.ReturnBehavior, err))
		}

		fmt.Fprintf(&b, "\n  %s", *opts.ReturnBehavior)
	}

	fmt.Fprintf(&b, "\n  API_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(opts.APIIntegration))
	fmt.Fprintf(&b, "\n  AS '%s'", sqlbuilder.EscapeString(opts.URL))

	if len(opts.Headers) > 0 {
		b.WriteString("\n  HEADERS = (")

		for i, h := range opts.Headers {
			if i > 0 {
				b.WriteString(", ")
			}

			fmt.Fprintf(&b, "'%s' = '%s'", sqlbuilder.EscapeString(h.Name), sqlbuilder.EscapeString(h.Value))
		}

		b.WriteString(")")
	}

	if opts.MaxBatchRows != nil {
		fmt.Fprintf(&b, "\n  MAX_BATCH_ROWS = %d", *opts.MaxBatchRows)
	}

	if opts.Compression != nil {
		fmt.Fprintf(&b, "\n  COMPRESSION = '%s'", sqlbuilder.EscapeString(string(*opts.Compression)))
	}

	if opts.RequestTranslator != nil {
		fmt.Fprintf(&b, "\n  REQUEST_TRANSLATOR = '%s'", sqlbuilder.EscapeString(*opts.RequestTranslator))
	}

	if opts.ResponseTranslator != nil {
		fmt.Fprintf(&b, "\n  RESPONSE_TRANSLATOR = '%s'", sqlbuilder.EscapeString(*opts.ResponseTranslator))
	}

	if opts.Comment != nil {
		fmt.Fprintf(&b, "\n  COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment))
	}

	if _, err := c.client.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("creating external function %s: %w", opts.Name, err)
	}

	return nil
}

// Alter modifies an existing external function.
func (c *ExternalFunctionClient) Alter(ctx context.Context, opts AlterExternalFunctionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter external function options: %w", err))
	}

	fqn := opts.Name.FullyQualifiedName() + buildArgSignature(opts.ArgTypes)

	if opts.Comment != nil {
		q := fmt.Sprintf("ALTER FUNCTION %s SET COMMENT = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.Comment))
		if _, err := c.client.Exec(ctx, q); err != nil {
			return fmt.Errorf("altering external function %s: %w", opts.Name, err)
		}
	}

	if len(opts.UnsetFields) > 0 {
		q, err := sqlbuilder.BuildUnset("FUNCTION", fqn, opts.UnsetFields)
		if err != nil {
			return NewTerminalError(err)
		}

		if q != "" {
			if _, err := c.client.Exec(ctx, q); err != nil {
				return fmt.Errorf("unsetting external function fields %s: %w", opts.Name, err)
			}
		}
	}

	return nil
}

// Drop removes an external function from Snowflake.
func (c *ExternalFunctionClient) Drop(ctx context.Context, name SchemaObjectIdentifier, argTypes []string) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("external function name is required"))
	}

	q := fmt.Sprintf("DROP FUNCTION IF EXISTS %s%s", name.FullyQualifiedName(), buildArgSignature(argTypes))

	if _, err := c.client.Exec(ctx, q); err != nil {
		return fmt.Errorf("dropping external function %s: %w", name, err)
	}

	return nil
}
