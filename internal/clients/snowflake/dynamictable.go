package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// DynamicTableObservation holds the result of observing a Snowflake dynamic table.
type DynamicTableObservation struct {
	// Exists indicates whether the dynamic table was found.
	Exists bool

	// ShowOutput contains the SHOW DYNAMIC TABLES row.
	ShowOutput *DynamicTableShowOutput
}

// DynamicTableShowOutput contains the fields from SHOW DYNAMIC TABLES.
type DynamicTableShowOutput struct {
	CreatedOn       string
	Name            string
	DatabaseName    string
	SchemaName      string
	Owner           string
	Comment         string
	TargetLag       string
	Warehouse       string
	RefreshMode     string
	Text            string
	SchedulingState string
	ClusterBy       string
	DataTimestamp   string
}

// CreateDynamicTableOptions holds the parameters for creating a dynamic table.
type CreateDynamicTableOptions struct {
	Name                       SchemaObjectIdentifier
	Query                      string
	TargetLag                  string
	Warehouse                  string
	RefreshMode                *string
	Initialize                 *string
	Comment                    *string
	Transient                  bool
	ClusterBy                  []string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
}

// validRefreshModes contains the allowed REFRESH_MODE values for defense-in-depth
// (the CRD enum annotation is the primary gate).
var validRefreshModes = map[string]bool{
	"AUTO": true, "FULL": true, "INCREMENTAL": true,
}

// validInitializeValues contains the allowed INITIALIZE values for defense-in-depth.
var validInitializeValues = map[string]bool{
	"ON_CREATE": true, "ON_SCHEDULE": true,
}

// Validate checks the CreateDynamicTableOptions for validity.
func (o *CreateDynamicTableOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("dynamic table name is required"))
	}

	if o.Query == "" {
		errs = append(errs, fmt.Errorf("query is required"))
	}

	if o.TargetLag == "" {
		errs = append(errs, fmt.Errorf("target lag is required"))
	}

	if o.Warehouse == "" {
		errs = append(errs, fmt.Errorf("warehouse is required"))
	}

	if o.RefreshMode != nil && !validRefreshModes[*o.RefreshMode] {
		errs = append(errs, fmt.Errorf("invalid refresh mode %q: must be AUTO, FULL, or INCREMENTAL", *o.RefreshMode))
	}

	if o.Initialize != nil && !validInitializeValues[*o.Initialize] {
		errs = append(errs, fmt.Errorf("invalid initialize value %q: must be ON_CREATE or ON_SCHEDULE", *o.Initialize))
	}

	return errors.Join(errs...)
}

// AlterDynamicTableOptions holds the parameters for altering a dynamic table.
type AlterDynamicTableOptions struct {
	Name SchemaObjectIdentifier

	// TargetLag is the new target lag value.
	TargetLag *string

	// Warehouse is the new warehouse.
	Warehouse *string

	// Comment is the new comment.
	Comment *string

	// ClusterBy is the new clustering key expressions.
	ClusterBy []string

	// UnsetClusterBy removes the clustering key.
	UnsetClusterBy bool

	// DataRetentionTimeInDays sets the data retention time.
	DataRetentionTimeInDays *int32

	// MaxDataExtensionTimeInDays sets the max data extension time.
	MaxDataExtensionTimeInDays *int32

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterDynamicTableOptions for validity.
func (o *AlterDynamicTableOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("dynamic table name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterDynamicTableOptions) HasChanges() bool {
	return o.TargetLag != nil || o.Warehouse != nil || o.Comment != nil ||
		len(o.ClusterBy) > 0 || o.UnsetClusterBy ||
		o.DataRetentionTimeInDays != nil || o.MaxDataExtensionTimeInDays != nil ||
		len(o.UnsetFields) > 0
}

// DynamicTableClient provides operations against Snowflake dynamic tables.
type DynamicTableClient struct {
	client SQLExecutor
}

// NewDynamicTableClient creates a new DynamicTableClient backed by the given SQLExecutor.
func NewDynamicTableClient(c SQLExecutor) *DynamicTableClient {
	return &DynamicTableClient{client: c}
}

// buildCreateDynamicTableSQL builds the CREATE DYNAMIC TABLE SQL statement.
//
// We use plain CREATE (not CREATE OR REPLACE) to match the Terraform provider's
// default-safe approach. Snowflake does not support CREATE DYNAMIC TABLE IF NOT
// EXISTS, so a plain CREATE will fail loudly if the table already exists — this
// is preferable to silently replacing it. The reconciler guards calls to Create
// by first checking Observe to confirm the object does not exist.
func buildCreateDynamicTableSQL(opts CreateDynamicTableOptions) (string, error) {
	var b sqlbuilder.Builder

	if opts.Transient {
		b.WriteString("CREATE TRANSIENT DYNAMIC TABLE ")
	} else {
		b.WriteString("CREATE DYNAMIC TABLE ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())

	b.WriteString(" TARGET_LAG = '")
	b.WriteString(sqlbuilder.EscapeString(opts.TargetLag))
	b.WriteString("'")

	b.WriteString(" WAREHOUSE = ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.Warehouse))

	if opts.RefreshMode != nil {
		b.WriteString(" REFRESH_MODE = ")
		b.WriteString(*opts.RefreshMode)
	}

	if opts.Initialize != nil {
		b.WriteString(" INITIALIZE = ")
		b.WriteString(*opts.Initialize)
	}

	b.SetString("COMMENT", opts.Comment)

	if len(opts.ClusterBy) > 0 {
		b.WriteString(" CLUSTER BY (")
		for i, expr := range opts.ClusterBy {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(expr)
		}
		b.WriteString(")")
	}

	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	b.SetInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)

	b.WriteString(" AS ")
	b.WriteString(opts.Query)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a dynamic table in Snowflake.
func (d *DynamicTableClient) Create(ctx context.Context, opts CreateDynamicTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create dynamic table options: %w", err))
	}

	sql, err := buildCreateDynamicTableSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create dynamic table SQL: %w", err))
	}

	if _, err := d.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating dynamic table %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterDynamicTableStatements builds the ALTER DYNAMIC TABLE SQL statements.
// Dynamic tables use SET-style ALTER for target_lag and warehouse.
func buildAlterDynamicTableStatements(opts AlterDynamicTableOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var stmts []string

	// Target lag uses a dedicated SET syntax.
	if opts.TargetLag != nil {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET TARGET_LAG = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.TargetLag))
		stmts = append(stmts, stmt)
	}

	// Warehouse uses a dedicated SET syntax.
	if opts.Warehouse != nil {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET WAREHOUSE = %s",
			fqn, sqlbuilder.QuoteIdentifier(*opts.Warehouse))
		stmts = append(stmts, stmt)
	}

	// Comment uses SET/UNSET syntax.
	if opts.Comment != nil {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET COMMENT = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.Comment))
		stmts = append(stmts, stmt)
	}

	// ClusterBy uses SET/UNSET syntax.
	if len(opts.ClusterBy) > 0 {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET CLUSTER BY (%s)",
			fqn, strings.Join(opts.ClusterBy, ", "))
		stmts = append(stmts, stmt)
	} else if opts.UnsetClusterBy {
		stmts = append(stmts, fmt.Sprintf("ALTER DYNAMIC TABLE %s DROP CLUSTERING KEY", fqn))
	}

	// DataRetentionTimeInDays uses SET syntax.
	if opts.DataRetentionTimeInDays != nil {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET DATA_RETENTION_TIME_IN_DAYS = %d",
			fqn, *opts.DataRetentionTimeInDays)
		stmts = append(stmts, stmt)
	}

	// MaxDataExtensionTimeInDays uses SET syntax.
	if opts.MaxDataExtensionTimeInDays != nil {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s SET MAX_DATA_EXTENSION_TIME_IN_DAYS = %d",
			fqn, *opts.MaxDataExtensionTimeInDays)
		stmts = append(stmts, stmt)
	}

	// UNSET fields (e.g. COMMENT).
	for _, field := range opts.UnsetFields {
		stmt := fmt.Sprintf("ALTER DYNAMIC TABLE %s UNSET %s", fqn, field)
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// Alter alters a dynamic table in Snowflake.
func (d *DynamicTableClient) Alter(ctx context.Context, opts AlterDynamicTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter dynamic table options: %w", err))
	}

	stmts, err := buildAlterDynamicTableStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter dynamic table statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := d.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering dynamic table %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropDynamicTableSQL builds the DROP DYNAMIC TABLE SQL statement.
func buildDropDynamicTableSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("DYNAMIC TABLE", name.FullyQualifiedName())
}

// Drop drops a dynamic table from Snowflake.
func (d *DynamicTableClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("dynamic table name is required"))
	}

	if _, err := d.client.Exec(ctx, buildDropDynamicTableSQL(name)); err != nil {
		return fmt.Errorf("dropping dynamic table %s: %w", name, err)
	}

	return nil
}

// buildShowDynamicTableByIDSQL builds a SHOW DYNAMIC TABLES LIKE SQL scoped to a schema.
func buildShowDynamicTableByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("DYNAMIC TABLES", name.Name(), scope)
}

// ShowByID queries SHOW DYNAMIC TABLES for a specific table within a schema.
func (d *DynamicTableClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*DynamicTableShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("dynamic table name is required"))
	}

	rows, err := d.client.Query(ctx, buildShowDynamicTableByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing dynamic table %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDynamicTableShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a DynamicTableObservation.
func (d *DynamicTableClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*DynamicTableObservation, error) {
	show, err := d.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &DynamicTableObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &DynamicTableObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanDynamicTableShowOutput scans SHOW DYNAMIC TABLES results for a matching row.
func scanDynamicTableShowOutput(rows *sql.Rows, name string) (*DynamicTableShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*DynamicTableShowOutput, error) {
		return &DynamicTableShowOutput{
			CreatedOn:       m["created_on"],
			Name:            m["name"],
			DatabaseName:    m["database_name"],
			SchemaName:      m["schema_name"],
			Owner:           m["owner"],
			Comment:         m["comment"],
			TargetLag:       m["target_lag"],
			Warehouse:       m["warehouse"],
			RefreshMode:     m["refresh_mode"],
			Text:            m["text"],
			SchedulingState: m["scheduling_state"],
			ClusterBy:       m["cluster_by"],
			DataTimestamp:   m["data_timestamp"],
		}, nil
	})
}
