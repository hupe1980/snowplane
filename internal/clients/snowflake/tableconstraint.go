package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TableConstraintObservation holds the result of observing a table constraint.
type TableConstraintObservation struct {
	// Exists indicates whether the constraint was found.
	Exists bool

	// ConstraintName is the constraint name as observed in Snowflake.
	ConstraintName string

	// ConstraintType is the constraint type (PRIMARY KEY, UNIQUE, FOREIGN KEY).
	ConstraintType string

	// Columns is the list of columns in the constraint key.
	Columns []string

	// Comment is the constraint comment (if any).
	Comment string
}

// TableConstraintIdentifier uniquely identifies a table constraint.
type TableConstraintIdentifier struct {
	// ConstraintName is the constraint name.
	ConstraintName string

	// TableName is the fully qualified table name.
	TableName string
}

// FullyQualifiedName returns a human-readable representation.
func (id TableConstraintIdentifier) FullyQualifiedName() string {
	return fmt.Sprintf("CONSTRAINT %s ON %s",
		sqlbuilder.QuoteIdentifier(id.ConstraintName), id.TableName)
}

// String returns the fully qualified name.
func (id TableConstraintIdentifier) String() string { return id.FullyQualifiedName() }

// AddConstraintOptions holds the parameters for adding a table constraint.
type AddConstraintOptions struct {
	ConstraintName string
	ConstraintType string // "PRIMARY KEY", "UNIQUE", "FOREIGN KEY"
	TableName      string // fully qualified table name
	Columns        []string

	// Foreign key specifics.
	ReferencesTableName string
	ReferencesColumns   []string
	Match               *string
	OnUpdate            *string
	OnDelete            *string

	// Constraint properties.
	Enforced       *bool
	Deferrable     *bool
	Initially      *string
	Rely           *bool
	ShouldValidate *bool

	// Comment — set via COMMENT ON CONSTRAINT.
	Comment *string
}

// Validate checks the AddConstraintOptions for validity.
func (o *AddConstraintOptions) Validate() error {
	if o.ConstraintName == "" {
		return fmt.Errorf("constraint name is required")
	}

	if o.TableName == "" {
		return fmt.Errorf("table name is required")
	}

	if o.ConstraintType == "" {
		return fmt.Errorf("constraint type is required")
	}

	if len(o.Columns) == 0 {
		return fmt.Errorf("at least one column is required")
	}

	if o.ConstraintType == "FOREIGN KEY" {
		if o.ReferencesTableName == "" {
			return fmt.Errorf("references table name is required for FOREIGN KEY constraints")
		}

		if len(o.ReferencesColumns) == 0 {
			return fmt.Errorf("references columns are required for FOREIGN KEY constraints")
		}
	}

	return nil
}

// AlterConstraintOptions holds the parameters for altering a table constraint's properties.
type AlterConstraintOptions struct {
	ConstraintName string
	TableName      string

	Enforced       *bool
	Deferrable     *bool
	Initially      *string
	Rely           *bool
	ShouldValidate *bool
	Comment        *string
}

// HasChanges returns true if there are any mutable changes to apply.
func (o *AlterConstraintOptions) HasChanges() bool {
	return o.Enforced != nil || o.Deferrable != nil || o.Initially != nil ||
		o.Rely != nil || o.ShouldValidate != nil || o.Comment != nil
}

// Validate checks the AlterConstraintOptions for validity.
func (o *AlterConstraintOptions) Validate() error {
	if o.ConstraintName == "" {
		return fmt.Errorf("constraint name is required")
	}

	if o.TableName == "" {
		return fmt.Errorf("table name is required")
	}

	return nil
}

// DropConstraintOptions holds parameters for dropping a table constraint.
type DropConstraintOptions struct {
	ConstraintName string
	TableName      string
	Cascade        bool
}

// TableConstraintClient provides operations against Snowflake table constraints.
type TableConstraintClient struct {
	client SQLExecutor
}

// NewTableConstraintClient creates a new TableConstraintClient.
func NewTableConstraintClient(c SQLExecutor) *TableConstraintClient {
	return &TableConstraintClient{client: c}
}

// buildAddConstraintSQL builds the ALTER TABLE ... ADD CONSTRAINT SQL.
func buildAddConstraintSQL(opts AddConstraintOptions) string {
	quotedCols := make([]string, len(opts.Columns))
	for i, col := range opts.Columns {
		quotedCols[i] = sqlbuilder.QuoteIdentifier(col)
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "ALTER TABLE %s ADD CONSTRAINT %s %s (%s)",
		opts.TableName,
		sqlbuilder.QuoteIdentifier(opts.ConstraintName),
		opts.ConstraintType,
		strings.Join(quotedCols, ", "),
	)

	if opts.ConstraintType == "FOREIGN KEY" {
		quotedRefCols := make([]string, len(opts.ReferencesColumns))
		for i, col := range opts.ReferencesColumns {
			quotedRefCols[i] = sqlbuilder.QuoteIdentifier(col)
		}

		fmt.Fprintf(&sb, " REFERENCES %s (%s)",
			opts.ReferencesTableName,
			strings.Join(quotedRefCols, ", "),
		)

		if opts.Match != nil {
			fmt.Fprintf(&sb, " MATCH %s", *opts.Match)
		}

		if opts.OnUpdate != nil {
			fmt.Fprintf(&sb, " ON UPDATE %s", *opts.OnUpdate)
		}

		if opts.OnDelete != nil {
			fmt.Fprintf(&sb, " ON DELETE %s", *opts.OnDelete)
		}
	}

	// Constraint properties.
	if opts.Enforced != nil {
		if *opts.Enforced {
			sb.WriteString(" ENFORCED")
		} else {
			sb.WriteString(" NOT ENFORCED")
		}
	}

	if opts.Deferrable != nil {
		if *opts.Deferrable {
			sb.WriteString(" DEFERRABLE")
		} else {
			sb.WriteString(" NOT DEFERRABLE")
		}
	}

	if opts.Initially != nil {
		fmt.Fprintf(&sb, " INITIALLY %s", *opts.Initially)
	}

	if opts.Rely != nil {
		if *opts.Rely {
			sb.WriteString(" RELY")
		} else {
			sb.WriteString(" NORELY")
		}
	}

	if opts.ShouldValidate != nil {
		if *opts.ShouldValidate {
			sb.WriteString(" VALIDATE")
		} else {
			sb.WriteString(" NOVALIDATE")
		}
	}

	return sb.String()
}

// buildAlterConstraintSQL builds the ALTER TABLE ... ALTER CONSTRAINT SQL.
func buildAlterConstraintSQL(opts AlterConstraintOptions) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "ALTER TABLE %s ALTER CONSTRAINT %s",
		opts.TableName,
		sqlbuilder.QuoteIdentifier(opts.ConstraintName),
	)

	if opts.Enforced != nil {
		if *opts.Enforced {
			sb.WriteString(" ENFORCED")
		} else {
			sb.WriteString(" NOT ENFORCED")
		}
	}

	if opts.Rely != nil {
		if *opts.Rely {
			sb.WriteString(" RELY")
		} else {
			sb.WriteString(" NORELY")
		}
	}

	if opts.ShouldValidate != nil {
		if *opts.ShouldValidate {
			sb.WriteString(" VALIDATE")
		} else {
			sb.WriteString(" NOVALIDATE")
		}
	}

	return sb.String()
}

// buildDropConstraintSQL builds the ALTER TABLE ... DROP CONSTRAINT SQL.
func buildDropConstraintSQL(opts DropConstraintOptions) string {
	query := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s",
		opts.TableName,
		sqlbuilder.QuoteIdentifier(opts.ConstraintName),
	)

	if opts.Cascade {
		query += " CASCADE"
	}

	return query
}

// buildCommentOnConstraintSQL builds the COMMENT ON CONSTRAINT SQL.
func buildCommentOnConstraintSQL(tableName, constraintName, comment string) string {
	return fmt.Sprintf("COMMENT ON CONSTRAINT %s ON %s IS '%s'",
		sqlbuilder.QuoteIdentifier(constraintName),
		tableName,
		sqlbuilder.EscapeString(comment),
	)
}

// buildShowConstraintSQL builds a SHOW query for the specified constraint type.
func buildShowConstraintSQL(tableName, constraintType string) string {
	switch constraintType {
	case "PRIMARY KEY":
		return fmt.Sprintf("SHOW PRIMARY KEYS IN TABLE %s", tableName)
	case "UNIQUE":
		return fmt.Sprintf("SHOW UNIQUE KEYS IN TABLE %s", tableName)
	case "FOREIGN KEY":
		return fmt.Sprintf("SHOW IMPORTED KEYS IN TABLE %s", tableName)
	default:
		return fmt.Sprintf("SHOW PRIMARY KEYS IN TABLE %s", tableName)
	}
}

// Observe queries Snowflake to check if a constraint exists on the table.
func (c *TableConstraintClient) Observe(ctx context.Context, id TableConstraintIdentifier, constraintType string) (*TableConstraintObservation, error) {
	query := buildShowConstraintSQL(id.TableName, constraintType)

	rows, err := c.client.Query(ctx, query)
	if err != nil {
		if IsObjectNotExistOrNotAuthorized(err) || IsSQLCompilationError(err) {
			return &TableConstraintObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("observing table constraint %s: %w", id, err)
	}
	defer closeRows(rows)

	return scanConstraintRows(rows, id.ConstraintName, constraintType)
}

// scanConstraintRows parses SHOW PRIMARY/UNIQUE/IMPORTED KEYS results.
// All three SHOW commands return similar column layouts. The constraint_name
// column is available in each.
func scanConstraintRows(rows *sql.Rows, expectedName, constraintType string) (*TableConstraintObservation, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	// Find column indices by name.
	constraintNameIdx := -1
	columnNameIdx := -1
	commentIdx := -1

	for i, col := range cols {
		switch strings.ToUpper(col) {
		case "CONSTRAINT_NAME":
			constraintNameIdx = i
		case "COLUMN_NAME", "FK_COLUMN_NAME":
			columnNameIdx = i
		case "COMMENT":
			commentIdx = i
		}
	}

	if constraintNameIdx < 0 {
		return &TableConstraintObservation{Exists: false}, nil
	}

	observation := &TableConstraintObservation{
		ConstraintType: constraintType,
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		valuePtrs := make([]any, len(cols))

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning constraint row: %w", err)
		}

		name := ""
		if constraintNameIdx >= 0 && values[constraintNameIdx].Valid {
			name = values[constraintNameIdx].String
		}

		if !strings.EqualFold(name, expectedName) {
			continue
		}

		observation.Exists = true
		observation.ConstraintName = name

		if columnNameIdx >= 0 && values[columnNameIdx].Valid {
			observation.Columns = append(observation.Columns, values[columnNameIdx].String)
		}

		if commentIdx >= 0 && values[commentIdx].Valid {
			observation.Comment = values[commentIdx].String
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating constraint rows: %w", err)
	}

	return observation, nil
}

// AddConstraint adds a constraint to a table.
func (c *TableConstraintClient) AddConstraint(ctx context.Context, opts AddConstraintOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid add constraint options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildAddConstraintSQL(opts)); err != nil {
		return fmt.Errorf("adding constraint %s to %s: %w", opts.ConstraintName, opts.TableName, err)
	}

	// Set comment separately if provided.
	if opts.Comment != nil && *opts.Comment != "" {
		if _, err := c.client.Exec(ctx, buildCommentOnConstraintSQL(
			opts.TableName, opts.ConstraintName, *opts.Comment,
		)); err != nil {
			return fmt.Errorf("setting comment on constraint %s: %w", opts.ConstraintName, err)
		}
	}

	return nil
}

// AlterConstraint alters a constraint's properties.
func (c *TableConstraintClient) AlterConstraint(ctx context.Context, opts AlterConstraintOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter constraint options: %w", err))
	}

	// Alter constraint properties (enforced, rely, validate).
	if opts.Enforced != nil || opts.Rely != nil || opts.ShouldValidate != nil {
		if _, err := c.client.Exec(ctx, buildAlterConstraintSQL(opts)); err != nil {
			return fmt.Errorf("altering constraint %s on %s: %w", opts.ConstraintName, opts.TableName, err)
		}
	}

	// Set comment separately.
	if opts.Comment != nil {
		if _, err := c.client.Exec(ctx, buildCommentOnConstraintSQL(
			opts.TableName, opts.ConstraintName, *opts.Comment,
		)); err != nil {
			return fmt.Errorf("setting comment on constraint %s: %w", opts.ConstraintName, err)
		}
	}

	return nil
}

// DropConstraint drops a constraint from a table.
func (c *TableConstraintClient) DropConstraint(ctx context.Context, id TableConstraintIdentifier) error {
	if _, err := c.client.Exec(ctx, buildDropConstraintSQL(DropConstraintOptions{
		ConstraintName: id.ConstraintName,
		TableName:      id.TableName,
	})); err != nil {
		if IsObjectNotExistOrNotAuthorized(err) {
			return nil // Already gone.
		}

		return fmt.Errorf("dropping constraint %s from %s: %w", id.ConstraintName, id.TableName, err)
	}

	return nil
}
