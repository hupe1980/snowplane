package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TagObservation holds the result of observing a Snowflake tag.
type TagObservation struct {
	// Exists indicates whether the tag was found.
	Exists bool

	// ShowOutput contains the SHOW TAGS row.
	ShowOutput *v1alpha1.TagShowOutput
}

// CreateTagOptions holds the parameters for creating a tag.
type CreateTagOptions struct {
	Name          SchemaObjectIdentifier
	AllowedValues []string
	Comment       *string

	// UseCreateOrAlter emits CREATE OR ALTER TAG instead of
	// CREATE TAG IF NOT EXISTS.
	UseCreateOrAlter bool
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

// buildCreateTagSQL builds the CREATE TAG SQL statement.
func buildCreateTagSQL(opts CreateTagOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "TAG", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

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

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a tag in Snowflake.
func (t *TagClient) Create(ctx context.Context, opts CreateTagOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create tag options: %w", err))
	}

	sql, err := buildCreateTagSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create tag SQL: %w", err))
	}

	if _, err := t.client.Exec(ctx, sql); err != nil {
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
func (t *TagClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.TagShowOutput, error) {
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
func scanTagShowOutput(rows *sql.Rows, name string) (*v1alpha1.TagShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.TagShowOutput, error) {
		return &v1alpha1.TagShowOutput{
			CreatedOn:     m["created_on"],
			Name:          m["name"],
			DatabaseName:  m["database_name"],
			SchemaName:    m["schema_name"],
			Owner:         m["owner"],
			Comment:       m["comment"],
			AllowedValues: m["allowed_values"],
		}, nil
	})
}
