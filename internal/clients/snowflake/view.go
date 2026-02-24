package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ViewObservation holds the result of observing a Snowflake view.
type ViewObservation struct {
	// Exists indicates whether the view was found.
	Exists bool

	// ShowOutput contains the SHOW VIEWS row.
	ShowOutput *ViewShowOutput
}

// ViewShowOutput contains the fields from SHOW VIEWS.
type ViewShowOutput struct {
	CreatedOn      string
	Name           string
	DatabaseName   string
	SchemaName     string
	Comment        string
	Owner          string
	IsSecure       bool
	Text           string // View definition
	ChangeTracking bool
}

// CreateViewOptions holds the parameters for creating a view.
type CreateViewOptions struct {
	Name           SchemaObjectIdentifier
	Statement      string
	Secure         bool
	Comment        *string
	ChangeTracking *bool
	OrReplace      bool
}

// Validate checks the CreateViewOptions for validity.
func (o *CreateViewOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("view name is required"))
	}

	if o.Statement == "" {
		errs = append(errs, fmt.Errorf("view statement (AS query) is required"))
	}

	return errors.Join(errs...)
}

// AlterViewOptions holds the parameters for altering a view.
type AlterViewOptions struct {
	Name           SchemaObjectIdentifier
	Secure         *bool
	Comment        *string
	ChangeTracking *bool

	// ReplaceStatement, when non-nil, indicates the view body (AS query) has
	// changed. Since ALTER VIEW cannot change the statement, the Alter method
	// issues a CREATE OR REPLACE VIEW instead. All other mutable fields
	// (Secure, Comment, ChangeTracking) are applied in the same operation.
	ReplaceStatement *ReplaceViewStatement

	// UnsetFields lists Snowflake parameter names to revert to server defaults.
	UnsetFields []string
}

// ReplaceViewStatement carries the full view spec needed for a
// CREATE OR REPLACE VIEW when the statement changes (R9-1).
type ReplaceViewStatement struct {
	Statement      string
	Secure         bool
	Comment        *string
	ChangeTracking *bool
}

// Validate checks the AlterViewOptions for validity.
func (o *AlterViewOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("view name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterViewOptions) HasChanges() bool {
	return o.ReplaceStatement != nil ||
		o.Secure != nil ||
		o.Comment != nil ||
		o.ChangeTracking != nil ||
		len(o.UnsetFields) > 0
}

// ViewClient provides operations against Snowflake views.
type ViewClient struct {
	client SQLExecutor
}

// NewViewClient creates a new ViewClient backed by the given SQLExecutor.
func NewViewClient(c SQLExecutor) *ViewClient {
	return &ViewClient{client: c}
}

// buildCreateViewSQL builds the CREATE VIEW SQL statement.
func buildCreateViewSQL(opts CreateViewOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("CREATE")

	if opts.OrReplace {
		b.WriteString(" OR REPLACE")
	}

	if opts.Secure {
		b.WriteString(" SECURE")
	}

	b.WriteString(" VIEW")

	if !opts.OrReplace {
		b.WriteString(" IF NOT EXISTS")
	}

	b.WriteString(" ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetBool("CHANGE_TRACKING", opts.ChangeTracking)
	b.SetString("COMMENT", opts.Comment)

	b.WriteString(" AS ")
	b.WriteString(opts.Statement)

	return b.String()
}

// Create creates a view in Snowflake.
func (v *ViewClient) Create(ctx context.Context, opts CreateViewOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create view options: %w", err))
	}

	if _, err := v.client.Exec(ctx, buildCreateViewSQL(opts)); err != nil {
		return fmt.Errorf("creating view %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterViewStatements builds the ALTER VIEW SQL statements.
func buildAlterViewStatements(opts AlterViewOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName()

	// Handle SECURE as a separate SET/UNSET.
	if opts.Secure != nil {
		if *opts.Secure {
			statements = append(statements, fmt.Sprintf("ALTER VIEW %s SET SECURE", fqn))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER VIEW %s UNSET SECURE", fqn))
		}
	}

	// Build SET clause for other parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Bool("CHANGE_TRACKING", opts.ChangeTracking)

	alterStmts, err := sqlbuilder.BuildAlterStatements("VIEW", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a view in Snowflake. Only changed fields are sent.
// When the view body (statement) changes, Snowflake's ALTER VIEW does not
// support it, so a CREATE OR REPLACE VIEW is issued instead (R9-1).
func (v *ViewClient) Alter(ctx context.Context, opts AlterViewOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter view options: %w", err))
	}

	// Statement change → CREATE OR REPLACE VIEW (atomic replacement).
	if opts.ReplaceStatement != nil {
		rs := opts.ReplaceStatement

		createOpts := CreateViewOptions{
			Name:           opts.Name,
			Statement:      rs.Statement,
			Secure:         rs.Secure,
			Comment:        rs.Comment,
			ChangeTracking: rs.ChangeTracking,
			OrReplace:      true,
		}

		if _, err := v.client.Exec(ctx, buildCreateViewSQL(createOpts)); err != nil {
			return fmt.Errorf("replacing view %s: %w", opts.Name, err)
		}

		return nil
	}

	stmts, err := buildAlterViewStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter view statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := v.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering view %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropViewSQL builds the DROP VIEW SQL statement.
func buildDropViewSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("VIEW", name.FullyQualifiedName())
}

// Drop drops a view from Snowflake.
func (v *ViewClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("view name is required"))
	}

	if _, err := v.client.Exec(ctx, buildDropViewSQL(name)); err != nil {
		return fmt.Errorf("dropping view %s: %w", name, err)
	}

	return nil
}

// buildShowViewByIDSQL builds a SHOW VIEWS LIKE SQL statement scoped to a schema.
func buildShowViewByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("VIEWS", name.Name(), scope)
}

// ShowByID queries SHOW VIEWS for a specific view name within a schema.
func (v *ViewClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*ViewShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("view name is required"))
	}

	rows, err := v.client.Query(ctx, buildShowViewByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing view %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanViewShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a ViewObservation.
func (v *ViewClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ViewObservation, error) {
	show, err := v.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ViewObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ViewObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanViewShowOutput scans SHOW VIEWS results for a matching row.
func scanViewShowOutput(rows *sql.Rows, name string) (*ViewShowOutput, error) {
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

		return &ViewShowOutput{
			CreatedOn:      colMap["created_on"],
			Name:           colMap["name"],
			DatabaseName:   colMap["database_name"],
			SchemaName:     colMap["schema_name"],
			Comment:        colMap["comment"],
			Owner:          colMap["owner"],
			IsSecure:       strings.EqualFold(colMap["is_secure"], "true"),
			Text:           colMap["text"],
			ChangeTracking: strings.EqualFold(colMap["change_tracking"], "ON"),
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
