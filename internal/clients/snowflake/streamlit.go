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

// StreamlitObservation holds the result of observing a Snowflake Streamlit app.
type StreamlitObservation struct {
	// Exists indicates whether the Streamlit was found.
	Exists bool

	// ShowOutput contains the SHOW STREAMLITS row.
	ShowOutput *v1alpha1.StreamlitShowOutput

	// DescribeOutput contains the DESCRIBE STREAMLIT output.
	DescribeOutput *v1alpha1.StreamlitDescribeOutput
}

// CreateStreamlitOptions holds the parameters for creating a Streamlit.
type CreateStreamlitOptions struct {
	Name                       SchemaObjectIdentifier
	From                       *string
	MainFile                   *string
	QueryWarehouse             *string
	Comment                    *string
	Title                      *string
	ExternalAccessIntegrations []string
}

// Validate checks the CreateStreamlitOptions for validity.
func (o *CreateStreamlitOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("streamlit name is required"))
	}

	return errors.Join(errs...)
}

// AlterStreamlitOptions holds the parameters for altering a Streamlit.
type AlterStreamlitOptions struct {
	Name SchemaObjectIdentifier

	// MainFile sets the entrypoint file.
	MainFile *string

	// QueryWarehouse sets the query warehouse.
	QueryWarehouse *string

	// Comment sets the comment.
	Comment *string

	// Title sets the display title.
	Title *string

	// ExternalAccessIntegrations sets the integrations list.
	ExternalAccessIntegrations *[]string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterStreamlitOptions for validity.
func (o *AlterStreamlitOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("streamlit name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStreamlitOptions) HasChanges() bool {
	return o.MainFile != nil || o.QueryWarehouse != nil || o.Comment != nil ||
		o.Title != nil || o.ExternalAccessIntegrations != nil ||
		len(o.UnsetFields) > 0
}

// buildCreateStreamlitSQL builds the CREATE STREAMLIT SQL.
func buildCreateStreamlitSQL(opts CreateStreamlitOptions) (string, error) {
	var b sqlbuilder.Builder

	b.WriteString("CREATE STREAMLIT ")
	b.WriteString(opts.Name.FullyQualifiedName())

	if opts.From != nil {
		b.WriteString(" FROM '")
		b.WriteString(sqlbuilder.EscapeString(*opts.From))
		b.WriteString("'")
	}

	if opts.MainFile != nil {
		b.WriteString(" MAIN_FILE = '")
		b.WriteString(sqlbuilder.EscapeString(*opts.MainFile))
		b.WriteString("'")
	}

	if opts.QueryWarehouse != nil {
		b.WriteString(" QUERY_WAREHOUSE = ")
		b.WriteString(sqlbuilder.QuoteIdentifier(*opts.QueryWarehouse))
	}

	b.SetString("COMMENT", opts.Comment)

	if opts.Title != nil {
		b.WriteString(" TITLE = '")
		b.WriteString(sqlbuilder.EscapeString(*opts.Title))
		b.WriteString("'")
	}

	if len(opts.ExternalAccessIntegrations) > 0 {
		b.WriteString(" EXTERNAL_ACCESS_INTEGRATIONS = (")

		for i, name := range opts.ExternalAccessIntegrations {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString(sqlbuilder.QuoteIdentifier(name))
		}

		b.WriteString(")")
	}

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// buildAlterStreamlitStatements builds the ALTER STREAMLIT SQL statements.
func buildAlterStreamlitStatements(opts AlterStreamlitOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var stmts []string

	// Build a single SET statement with all SET fields.
	var setParts []string

	if opts.MainFile != nil {
		setParts = append(setParts, fmt.Sprintf("MAIN_FILE = '%s'", sqlbuilder.EscapeString(*opts.MainFile)))
	}

	if opts.QueryWarehouse != nil {
		setParts = append(setParts, fmt.Sprintf("QUERY_WAREHOUSE = %s", sqlbuilder.QuoteIdentifier(*opts.QueryWarehouse)))
	}

	if opts.Comment != nil {
		setParts = append(setParts, fmt.Sprintf("COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment)))
	}

	if opts.Title != nil {
		setParts = append(setParts, fmt.Sprintf("TITLE = '%s'", sqlbuilder.EscapeString(*opts.Title)))
	}

	if opts.ExternalAccessIntegrations != nil {
		names := make([]string, len(*opts.ExternalAccessIntegrations))
		for i, n := range *opts.ExternalAccessIntegrations {
			names[i] = sqlbuilder.QuoteIdentifier(n)
		}

		setParts = append(setParts, fmt.Sprintf("EXTERNAL_ACCESS_INTEGRATIONS = (%s)", strings.Join(names, ", ")))
	}

	if len(setParts) > 0 {
		stmt := fmt.Sprintf("ALTER STREAMLIT %s SET %s", fqn, strings.Join(setParts, " "))
		stmts = append(stmts, stmt)
	}

	// UNSET fields.
	for _, field := range opts.UnsetFields {
		stmt := fmt.Sprintf("ALTER STREAMLIT %s UNSET %s", fqn, field)
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// StreamlitClient provides operations against Snowflake Streamlits.
type StreamlitClient struct {
	client SQLExecutor
}

// NewStreamlitClient creates a new StreamlitClient.
func NewStreamlitClient(c SQLExecutor) *StreamlitClient {
	return &StreamlitClient{client: c}
}

// Create creates a Streamlit in Snowflake.
func (c *StreamlitClient) Create(ctx context.Context, opts CreateStreamlitOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create streamlit options: %w", err))
	}

	sql, err := buildCreateStreamlitSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create streamlit SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating streamlit %s: %w", opts.Name, err)
	}

	return nil
}

// Alter alters a Streamlit in Snowflake.
func (c *StreamlitClient) Alter(ctx context.Context, opts AlterStreamlitOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter streamlit options: %w", err))
	}

	stmts, err := buildAlterStreamlitStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter streamlit statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering streamlit %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a Streamlit from Snowflake.
func (c *StreamlitClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("streamlit name is required"))
	}

	stmt := sqlbuilder.DropIfExists("STREAMLIT", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping streamlit %s: %w", name, err)
	}

	return nil
}

// buildShowStreamlitByIDSQL builds a SHOW STREAMLITS LIKE SQL.
func buildShowStreamlitByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("STREAMLITS", name.Name(), scope)
}

// buildDescribeStreamlitSQL builds the DESCRIBE STREAMLIT SQL.
func buildDescribeStreamlitSQL(name SchemaObjectIdentifier) string {
	return fmt.Sprintf("DESCRIBE STREAMLIT %s", name.FullyQualifiedName())
}

// ShowByID queries SHOW STREAMLITS for a specific Streamlit within a schema.
func (c *StreamlitClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.StreamlitShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("streamlit name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowStreamlitByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing streamlit %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStreamlitShowOutput(rows, name.Name())
}

// Describe queries DESCRIBE STREAMLIT.
func (c *StreamlitClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.StreamlitDescribeOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("streamlit name is required"))
	}

	rows, err := c.client.Query(ctx, buildDescribeStreamlitSQL(name))
	if err != nil {
		return nil, fmt.Errorf("describing streamlit %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStreamlitDescribeOutput(rows)
}

// Observe combines ShowByID and Describe into a StreamlitObservation.
func (c *StreamlitClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*StreamlitObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StreamlitObservation{Exists: false}, nil
		}

		return nil, err
	}

	obs := &StreamlitObservation{
		Exists:     true,
		ShowOutput: show,
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		// Non-fatal: DESCRIBE may fail due to permissions, still return SHOW data.
		return obs, nil
	}

	obs.DescribeOutput = desc

	return obs, nil
}

// scanStreamlitShowOutput scans SHOW STREAMLITS results for a matching row.
func scanStreamlitShowOutput(rows *sql.Rows, name string) (*v1alpha1.StreamlitShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.StreamlitShowOutput, error) {
		return &v1alpha1.StreamlitShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			DatabaseName:   m["database_name"],
			SchemaName:     m["schema_name"],
			Title:          m["title"],
			Comment:        m["comment"],
			Owner:          m["owner"],
			QueryWarehouse: m["query_warehouse"],
			URLID:          m["url_id"],
			OwnerRoleType:  m["owner_role_type"],
		}, nil
	})
}

// scanStreamlitDescribeOutput scans DESCRIBE STREAMLIT results.
func scanStreamlitDescribeOutput(rows *sql.Rows) (*v1alpha1.StreamlitDescribeOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	result := &v1alpha1.StreamlitDescribeOutput{}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("scanning describe output: %w", err)
		}

		return result, nil
	}

	// Build a map from column name → value using sql.NullString for safety.
	values := make([]sql.NullString, len(cols))
	ptrs := make([]interface{}, len(cols))

	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("scanning describe row: %w", err)
	}

	m := make(map[string]string, len(cols))
	for i, col := range cols {
		m[strings.ToLower(col)] = values[i].String
	}

	result.Title = m["title"]
	result.MainFile = m["main_file"]
	result.QueryWarehouse = m["query_warehouse"]
	result.Name = m["name"]
	result.Comment = m["comment"]
	result.ExternalAccessIntegrations = m["external_access_integrations"]

	return result, nil
}
