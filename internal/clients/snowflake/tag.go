package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TagObservation holds the result of observing a Snowflake tag.
type TagObservation struct {
	// Exists indicates whether the tag was found.
	Exists bool

	// ShowOutput contains the SHOW TAGS row.
	ShowOutput *TagShowOutput
}

// TagShowOutput contains the fields from SHOW TAGS.
type TagShowOutput struct {
	CreatedOn     string
	Name          string
	DatabaseName  string
	SchemaName    string
	Owner         string
	Comment       string
	AllowedValues string // comma-separated
}

// CreateTagOptions holds the parameters for creating a tag.
type CreateTagOptions struct {
	Name          SchemaObjectIdentifier
	AllowedValues []string
	Comment       *string
}

// Validate checks the CreateTagOptions for validity.
func (o *CreateTagOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("tag name is required")
	}

	return nil
}

// AlterTagOptions holds the parameters for altering a tag.
type AlterTagOptions struct {
	Name          SchemaObjectIdentifier
	AllowedValues *[]string // nil = no change; empty slice = unset
	Comment       *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterTagOptions for validity.
func (o *AlterTagOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("tag name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterTagOptions) HasChanges() bool {
	return o.AllowedValues != nil || o.Comment != nil || len(o.UnsetFields) > 0
}

// TagClient provides operations against Snowflake tags.
type TagClient struct {
	client SQLExecutor
}

// NewTagClient creates a new TagClient backed by the given SQLExecutor.
func NewTagClient(c SQLExecutor) *TagClient {
	return &TagClient{client: c}
}

// buildCreateTagSQL builds the CREATE OR ALTER TAG SQL statement.
func buildCreateTagSQL(opts CreateTagOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("CREATE OR ALTER TAG ")
	b.WriteString(opts.Name.FullyQualifiedName())

	// ALLOWED_VALUES must come before other parameters.
	if len(opts.AllowedValues) > 0 {
		b.WriteString(" ALLOWED_VALUES ")

		for i, v := range opts.AllowedValues {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString("'")
			b.WriteString(sqlbuilder.EscapeString(v))
			b.WriteString("'")
		}
	}

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a tag in Snowflake.
func (t *TagClient) Create(ctx context.Context, opts CreateTagOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create tag options: %w", err))
	}

	if _, err := t.client.Exec(ctx, buildCreateTagSQL(opts)); err != nil {
		return fmt.Errorf("creating tag %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterTagStatements builds the ALTER TAG SQL statements.
func buildAlterTagStatements(opts AlterTagOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// Handle allowed_values changes.
	if opts.AllowedValues != nil {
		if len(*opts.AllowedValues) == 0 {
			// Unset allowed values: ALTER TAG ... UNSET ALLOWED_VALUES
			statements = append(statements, fmt.Sprintf("ALTER TAG %s UNSET ALLOWED_VALUES", fqn))
		} else {
			// Drop + re-add for idempotency.
			statements = append(statements, fmt.Sprintf("ALTER TAG %s UNSET ALLOWED_VALUES", fqn))

			vals := make([]string, 0, len(*opts.AllowedValues))
			for _, v := range *opts.AllowedValues {
				vals = append(vals, fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v)))
			}

			statements = append(statements, fmt.Sprintf("ALTER TAG %s SET ALLOWED_VALUES %s", fqn, strings.Join(vals, ", ")))
		}
	}

	// Handle comment and other SET/UNSET fields.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("TAG", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a tag in Snowflake.
func (t *TagClient) Alter(ctx context.Context, opts AlterTagOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter tag options: %w", err))
	}

	stmts, err := buildAlterTagStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter tag statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := t.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering tag %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropTagSQL builds the DROP TAG SQL statement.
func buildDropTagSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("TAG", name.FullyQualifiedName())
}

// Drop drops a tag from Snowflake.
func (t *TagClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("tag name is required"))
	}

	if _, err := t.client.Exec(ctx, buildDropTagSQL(name)); err != nil {
		return fmt.Errorf("dropping tag %s: %w", name, err)
	}

	return nil
}

// buildShowTagByIDSQL builds a SHOW TAGS LIKE SQL scoped to a schema.
func buildShowTagByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("TAGS", name.Name(), scope)
}

// ShowByID queries SHOW TAGS for a specific tag within a schema.
func (t *TagClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*TagShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("tag name is required"))
	}

	rows, err := t.client.Query(ctx, buildShowTagByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing tag %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanTagShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a TagObservation.
func (t *TagClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*TagObservation, error) {
	show, err := t.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &TagObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &TagObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanTagShowOutput scans SHOW TAGS results for a matching row.
func scanTagShowOutput(rows *sql.Rows, name string) (*TagShowOutput, error) {
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

		return &TagShowOutput{
			CreatedOn:     colMap["created_on"],
			Name:          colMap["name"],
			DatabaseName:  colMap["database_name"],
			SchemaName:    colMap["schema_name"],
			Owner:         colMap["owner"],
			Comment:       colMap["comment"],
			AllowedValues: colMap["allowed_values"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
