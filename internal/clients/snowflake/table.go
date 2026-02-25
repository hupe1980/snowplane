package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TableObservation holds the result of observing a Snowflake table.
type TableObservation struct {
	// Exists indicates whether the table was found.
	Exists bool

	// ShowOutput contains the SHOW TABLES row.
	ShowOutput *TableShowOutput

	// Columns contains the columns returned by DESCRIBE TABLE.
	Columns []ColumnInfo
}

// TableShowOutput contains the fields from SHOW TABLES.
type TableShowOutput struct {
	CreatedOn             string
	Name                  string
	DatabaseName          string
	SchemaName            string
	Kind                  string // TABLE, TRANSIENT, TEMPORARY
	Comment               string
	Owner                 string
	RetentionTime         int32
	ClusterBy             string
	ChangeTracking        bool
	EnableSchemaEvolution bool
}

// ColumnInfo represents a column from DESCRIBE TABLE output.
type ColumnInfo struct {
	Name    string
	Type    string
	Kind    string // COLUMN
	Null    string // Y or N
	Default string
	Comment string
}

// CreateTableOptions holds the parameters for creating a table.
type CreateTableOptions struct {
	Name                       SchemaObjectIdentifier
	Columns                    []CreateTableColumn
	Constraints                []CreateTableConstraint
	Comment                    *string
	Transient                  bool
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	ChangeTracking             *bool
	DefaultDDLCollation        *string
	EnableSchemaEvolution      *bool
	ClusterBy                  []string

	// UseCreateOrAlter emits CREATE OR ALTER TABLE instead of
	// CREATE TABLE IF NOT EXISTS. Requires Snowflake support.
	UseCreateOrAlter bool
}

// CreateTableColumn represents a column definition for CREATE TABLE.
type CreateTableColumn struct {
	Name     string
	Type     string
	Nullable *bool
	Default  *string
	Comment  *string
}

// CreateTableConstraint represents a table-level constraint for CREATE TABLE.
type CreateTableConstraint struct {
	Name    string // optional — Snowflake generates if empty
	Type    string // PRIMARY KEY, UNIQUE, FOREIGN KEY
	Columns []string
	// ForeignKeyTable and ForeignKeyColumns are set for FOREIGN KEY constraints.
	ForeignKeyTable   string
	ForeignKeyColumns []string
}

// Validate checks the CreateTableOptions for validity.
func (o *CreateTableOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("table name is required"))
	}

	if len(o.Columns) == 0 {
		errs = append(errs, fmt.Errorf("at least one column is required"))
	}

	for i, col := range o.Columns {
		if col.Name == "" {
			errs = append(errs, fmt.Errorf("column %d: name is required", i))
		}

		if col.Type == "" {
			errs = append(errs, fmt.Errorf("column %d: type is required", i))
		} else if err := sqlbuilder.ValidateColumnType(col.Type); err != nil {
			errs = append(errs, fmt.Errorf("column %d: %w", i, err))
		}

		if col.Default != nil {
			if err := sqlbuilder.ValidateColumnDefault(*col.Default); err != nil {
				errs = append(errs, fmt.Errorf("column %d: %w", i, err))
			}
		}
	}

	if err := validateDataRetention(o.DataRetentionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateMaxDataExtension(o.MaxDataExtensionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	for i, c := range o.Constraints {
		if len(c.Columns) == 0 {
			errs = append(errs, fmt.Errorf("constraint %d: at least one column is required", i))
		}

		switch c.Type {
		case "PRIMARY KEY", "UNIQUE":
			// valid
		case "FOREIGN KEY":
			if c.ForeignKeyTable == "" {
				errs = append(errs, fmt.Errorf("constraint %d: foreign key table is required", i))
			}

			if len(c.ForeignKeyColumns) == 0 {
				errs = append(errs, fmt.Errorf("constraint %d: foreign key columns are required", i))
			}

			if len(c.Columns) != len(c.ForeignKeyColumns) {
				errs = append(errs, fmt.Errorf("constraint %d: column count must match foreign key column count", i))
			}
		default:
			errs = append(errs, fmt.Errorf("constraint %d: unknown type %q (expected PRIMARY KEY, UNIQUE, or FOREIGN KEY)", i, c.Type))
		}
	}

	return errors.Join(errs...)
}

// AlterTableOptions holds the parameters for altering a table.
type AlterTableOptions struct {
	Name                       SchemaObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	ChangeTracking             *bool
	DefaultDDLCollation        *string
	EnableSchemaEvolution      *bool
	ClusterBy                  []string
	DropClusteringKey          bool

	// UnsetFields lists Snowflake parameter names to revert to their server-side defaults.
	UnsetFields []string

	// AddColumns lists columns to add via ALTER TABLE ... ADD COLUMN.
	AddColumns []CreateTableColumn

	// DropColumns lists column names to drop via ALTER TABLE ... DROP COLUMN.
	DropColumns []string

	// AlterColumns lists column modifications via ALTER TABLE ... ALTER COLUMN.
	AlterColumns []AlterColumnAction
}

// AlterColumnAction describes a column-level alteration.
type AlterColumnAction struct {
	Name        string
	SetType     *string // ALTER COLUMN ... SET DATA TYPE
	SetNotNull  *bool   // ALTER COLUMN ... SET NOT NULL / DROP NOT NULL
	SetComment  *string // ALTER COLUMN ... COMMENT
	SetDefault  *string // ALTER COLUMN ... SET DEFAULT
	DropDefault bool    // ALTER COLUMN ... DROP DEFAULT
}

// Validate checks the AlterTableOptions for validity.
func (o *AlterTableOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("table name is required"))
	}

	if err := validateDataRetention(o.DataRetentionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateMaxDataExtension(o.MaxDataExtensionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	// Validate AddColumns — same rules as CreateTableOptions.
	for i, col := range o.AddColumns {
		if col.Name == "" {
			errs = append(errs, fmt.Errorf("addColumn %d: name is required", i))
		}

		if col.Type == "" {
			errs = append(errs, fmt.Errorf("addColumn %d: type is required", i))
		} else if err := sqlbuilder.ValidateColumnType(col.Type); err != nil {
			errs = append(errs, fmt.Errorf("addColumn %d: %w", i, err))
		}

		if col.Default != nil {
			if err := sqlbuilder.ValidateColumnDefault(*col.Default); err != nil {
				errs = append(errs, fmt.Errorf("addColumn %d: %w", i, err))
			}
		}
	}

	// Validate AlterColumns — column type and default changes.
	for i, ac := range o.AlterColumns {
		if ac.Name == "" {
			errs = append(errs, fmt.Errorf("alterColumn %d: name is required", i))
		}

		if ac.SetType != nil {
			if err := sqlbuilder.ValidateColumnType(*ac.SetType); err != nil {
				errs = append(errs, fmt.Errorf("alterColumn %d setType: %w", i, err))
			}
		}

		if ac.SetDefault != nil {
			if err := sqlbuilder.ValidateColumnDefault(*ac.SetDefault); err != nil {
				errs = append(errs, fmt.Errorf("alterColumn %d setDefault: %w", i, err))
			}
		}
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterTableOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.DataRetentionTimeInDays != nil ||
		o.MaxDataExtensionTimeInDays != nil ||
		o.ChangeTracking != nil ||
		o.DefaultDDLCollation != nil ||
		o.EnableSchemaEvolution != nil ||
		len(o.ClusterBy) > 0 ||
		o.DropClusteringKey ||
		len(o.UnsetFields) > 0 ||
		len(o.AddColumns) > 0 ||
		len(o.DropColumns) > 0 ||
		len(o.AlterColumns) > 0
}

// TableClient provides operations against Snowflake tables.
type TableClient struct {
	client SQLExecutor
}

// NewTableClient creates a new TableClient backed by the given SQLExecutor.
func NewTableClient(c SQLExecutor) *TableClient {
	return &TableClient{client: c}
}

// buildCreateTableSQL builds the CREATE TABLE SQL statement.
func buildCreateTableSQL(opts CreateTableOptions) string {
	var b sqlbuilder.Builder

	if opts.UseCreateOrAlter {
		b.WriteString("CREATE OR ALTER")
	} else {
		b.WriteString("CREATE")
	}

	if opts.Transient {
		b.WriteString(" TRANSIENT")
	}

	if opts.UseCreateOrAlter {
		b.WriteString(" TABLE ")
	} else {
		b.WriteString(" TABLE IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())
	b.WriteString(" (")

	for i, col := range opts.Columns {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(sqlbuilder.QuoteIdentifier(col.Name))
		b.WriteString(" ")
		b.WriteString(col.Type)

		if col.Nullable != nil && !*col.Nullable {
			b.WriteString(" NOT NULL")
		}

		if col.Default != nil {
			fmt.Fprintf(&b.Builder, " DEFAULT %s", *col.Default)
		}

		if col.Comment != nil {
			fmt.Fprintf(&b.Builder, " COMMENT '%s'", sqlbuilder.EscapeString(*col.Comment))
		}
	}

	// Append table-level constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY).
	for _, c := range opts.Constraints {
		b.WriteString(", ")

		if c.Name != "" {
			fmt.Fprintf(&b.Builder, "CONSTRAINT %s ", sqlbuilder.QuoteIdentifier(c.Name))
		}

		b.WriteString(c.Type)
		b.WriteString(" (")

		for j, col := range c.Columns {
			if j > 0 {
				b.WriteString(", ")
			}

			b.WriteString(sqlbuilder.QuoteIdentifier(col))
		}

		b.WriteString(")")

		if c.Type == "FOREIGN KEY" && c.ForeignKeyTable != "" {
			fmt.Fprintf(&b.Builder, " REFERENCES %s", c.ForeignKeyTable)

			if len(c.ForeignKeyColumns) > 0 {
				b.WriteString(" (")

				for j, col := range c.ForeignKeyColumns {
					if j > 0 {
						b.WriteString(", ")
					}

					b.WriteString(sqlbuilder.QuoteIdentifier(col))
				}

				b.WriteString(")")
			}
		}
	}

	b.WriteString(")")

	if len(opts.ClusterBy) > 0 {
		quoted := make([]string, len(opts.ClusterBy))
		for i, c := range opts.ClusterBy {
			quoted[i] = sqlbuilder.QuoteIdentifier(c)
		}

		fmt.Fprintf(&b.Builder, " CLUSTER BY (%s)", strings.Join(quoted, ", "))
	}

	b.SetBool("ENABLE_SCHEMA_EVOLUTION", opts.EnableSchemaEvolution)
	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	b.SetInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	b.SetBool("CHANGE_TRACKING", opts.ChangeTracking)
	b.SetString("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a table in Snowflake.
func (t *TableClient) Create(ctx context.Context, opts CreateTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create table options: %w", err))
	}

	if _, err := t.client.Exec(ctx, buildCreateTableSQL(opts)); err != nil {
		return fmt.Errorf("creating table %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterTableStatements builds the ALTER TABLE SQL statements.
func buildAlterTableStatements(opts AlterTableOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// Handle cluster key changes as separate statements.
	if opts.DropClusteringKey {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP CLUSTERING KEY", fqn))
	} else if len(opts.ClusterBy) > 0 {
		quoted := make([]string, len(opts.ClusterBy))
		for i, c := range opts.ClusterBy {
			quoted[i] = sqlbuilder.QuoteIdentifier(c)
		}

		statements = append(statements, fmt.Sprintf("ALTER TABLE %s CLUSTER BY (%s)",
			fqn, strings.Join(quoted, ", ")))
	}

	// Column additions — one ADD COLUMN per column for clarity.
	for _, col := range opts.AddColumns {
		var sb strings.Builder

		fmt.Fprintf(&sb, "ALTER TABLE %s ADD COLUMN %s %s",
			fqn, sqlbuilder.QuoteIdentifier(col.Name), col.Type)

		if col.Nullable != nil && !*col.Nullable {
			sb.WriteString(" NOT NULL")
		}

		if col.Default != nil {
			fmt.Fprintf(&sb, " DEFAULT %s", *col.Default)
		}

		if col.Comment != nil {
			fmt.Fprintf(&sb, " COMMENT '%s'", sqlbuilder.EscapeString(*col.Comment))
		}

		statements = append(statements, sb.String())
	}

	// Column drops — one DROP COLUMN per column.
	for _, colName := range opts.DropColumns {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
			fqn, sqlbuilder.QuoteIdentifier(colName)))
	}

	// Column alterations.
	for _, ac := range opts.AlterColumns {
		colID := sqlbuilder.QuoteIdentifier(ac.Name)

		if ac.SetType != nil {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DATA TYPE %s",
				fqn, colID, *ac.SetType))
		}

		if ac.SetNotNull != nil {
			if *ac.SetNotNull {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
					fqn, colID))
			} else {
				statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
					fqn, colID))
			}
		}

		if ac.DropDefault {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
				fqn, colID))
		} else if ac.SetDefault != nil {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
				fqn, colID, *ac.SetDefault))
		}

		if ac.SetComment != nil {
			statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s COMMENT '%s'",
				fqn, colID, sqlbuilder.EscapeString(*ac.SetComment)))
		}
	}

	// Build SET clause for parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	sc.Int32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	sc.Bool("CHANGE_TRACKING", opts.ChangeTracking)
	sc.String("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	sc.Bool("ENABLE_SCHEMA_EVOLUTION", opts.EnableSchemaEvolution)

	alterStmts, err := sqlbuilder.BuildAlterStatements("TABLE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a table in Snowflake. Only changed fields are sent.
func (t *TableClient) Alter(ctx context.Context, opts AlterTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter table options: %w", err))
	}

	stmts, err := buildAlterTableStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter table statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := t.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering table %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropTableSQL builds the DROP TABLE SQL statement.
func buildDropTableSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("TABLE", name.FullyQualifiedName())
}

// Drop drops a table from Snowflake.
func (t *TableClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("table name is required"))
	}

	if _, err := t.client.Exec(ctx, buildDropTableSQL(name)); err != nil {
		return fmt.Errorf("dropping table %s: %w", name, err)
	}

	return nil
}

// buildShowTableByIDSQL builds the SHOW TABLES LIKE SQL statement scoped to a schema.
func buildShowTableByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("TABLES", name.Name(), scope)
}

// ShowByID queries SHOW TABLES for a specific table name within a schema.
func (t *TableClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*TableShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("table name is required"))
	}

	rows, err := t.client.Query(ctx, buildShowTableByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing table %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanTableShowOutput(rows, name.Name())
}

// buildDescribeTableSQL builds the DESCRIBE TABLE SQL statement.
func buildDescribeTableSQL(name SchemaObjectIdentifier) string {
	return fmt.Sprintf("DESCRIBE TABLE %s", name.FullyQualifiedName())
}

// DescribeTable runs DESCRIBE TABLE and returns the column definitions.
func (t *TableClient) DescribeTable(ctx context.Context, name SchemaObjectIdentifier) ([]ColumnInfo, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("table name is required"))
	}

	rows, err := t.client.Query(ctx, buildDescribeTableSQL(name))
	if err != nil {
		return nil, fmt.Errorf("describing table %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanColumnInfo(rows)
}

// scanColumnInfo scans DESCRIBE TABLE output into []ColumnInfo.
func scanColumnInfo(rows *sql.Rows) ([]ColumnInfo, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	var result []ColumnInfo

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

		// Only include COLUMN rows (skip virtual columns, etc.).
		kind := colMap["kind"]
		if kind != "" && kind != "COLUMN" {
			continue
		}

		result = append(result, ColumnInfo{
			Name:    colMap["name"],
			Type:    colMap["type"],
			Kind:    colMap["kind"],
			Null:    colMap["null?"],
			Default: colMap["default"],
			Comment: colMap["comment"],
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return result, nil
}

// Observe combines ShowByID into a TableObservation.
func (t *TableClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*TableObservation, error) {
	show, err := t.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &TableObservation{Exists: false}, nil
		}

		return nil, err
	}

	columns, err := t.DescribeTable(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("describing table columns: %w", err)
	}

	return &TableObservation{
		Exists:     true,
		ShowOutput: show,
		Columns:    columns,
	}, nil
}

// scanTableShowOutput scans SHOW TABLES results for a matching row.
func scanTableShowOutput(rows *sql.Rows, name string) (*TableShowOutput, error) {
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

		rt, _ := parseInt32(colMap["retention_time"])

		return &TableShowOutput{
			CreatedOn:             colMap["created_on"],
			Name:                  colMap["name"],
			DatabaseName:          colMap["database_name"],
			SchemaName:            colMap["schema_name"],
			Kind:                  colMap["kind"],
			Comment:               colMap["comment"],
			Owner:                 colMap["owner"],
			RetentionTime:         rt,
			ClusterBy:             colMap["cluster_by"],
			ChangeTracking:        strings.EqualFold(colMap["change_tracking"], "ON"),
			EnableSchemaEvolution: strings.EqualFold(colMap["enable_schema_evolution"], "Y"),
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
