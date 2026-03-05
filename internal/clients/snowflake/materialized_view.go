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

// MaterializedViewObservation holds the result of observing a Snowflake materialized view.
type MaterializedViewObservation struct {
	// Exists indicates whether the materialized view was found.
	Exists bool

	// ShowOutput contains the SHOW MATERIALIZED VIEWS row.
	ShowOutput *v1alpha1.MaterializedViewShowOutput
}

// CreateMaterializedViewOptions holds the parameters for creating a materialized view.
type CreateMaterializedViewOptions struct {
	Name      SchemaObjectIdentifier
	Statement string
	Secure    bool
	Comment   *string
	ClusterBy []string
	OrReplace bool
}

// Validate checks the CreateMaterializedViewOptions for validity.
func (o *CreateMaterializedViewOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("materialized view name is required"))
	}

	if o.Statement == "" {
		errs = append(errs, fmt.Errorf("materialized view statement (AS query) is required"))
	}

	return errors.Join(errs...)
}

// AlterMaterializedViewOptions holds the parameters for altering a materialized view.
type AlterMaterializedViewOptions struct {
	Name SchemaObjectIdentifier

	// Secure toggles the SECURE property. nil = no change.
	Secure *bool

	// Comment sets the comment. nil = no change.
	Comment *string

	// UnsetFields lists Snowflake parameter names to revert to server defaults.
	UnsetFields []string
}

// Validate checks the AlterMaterializedViewOptions for validity.
func (o *AlterMaterializedViewOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("materialized view name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterMaterializedViewOptions) HasChanges() bool {
	return o.Secure != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// MaterializedViewClient provides operations against Snowflake materialized views.
type MaterializedViewClient struct {
	client SQLExecutor
}

// NewMaterializedViewClient creates a new MaterializedViewClient.
func NewMaterializedViewClient(c SQLExecutor) *MaterializedViewClient {
	return &MaterializedViewClient{client: c}
}

// buildCreateMaterializedViewSQL builds the CREATE MATERIALIZED VIEW SQL statement.
func buildCreateMaterializedViewSQL(opts CreateMaterializedViewOptions) (string, error) {
	var b sqlbuilder.Builder

	b.WriteString("CREATE")

	if opts.OrReplace {
		b.WriteString(" OR REPLACE")
	}

	if opts.Secure {
		b.WriteString(" SECURE")
	}

	b.WriteString(" MATERIALIZED VIEW")

	if !opts.OrReplace {
		b.WriteString(" IF NOT EXISTS")
	}

	b.WriteString(" ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetString("COMMENT", opts.Comment)

	if len(opts.ClusterBy) > 0 {
		b.WriteString(" CLUSTER BY (")
		b.WriteString(strings.Join(opts.ClusterBy, ", "))
		b.WriteString(")")
	}

	b.WriteString(" AS ")
	b.WriteString(opts.Statement)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a materialized view in Snowflake.
func (c *MaterializedViewClient) Create(ctx context.Context, opts CreateMaterializedViewOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create materialized view options: %w", err))
	}

	sql, err := buildCreateMaterializedViewSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create materialized view SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating materialized view %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterMaterializedViewStatements builds the ALTER MATERIALIZED VIEW SQL statements.
func buildAlterMaterializedViewStatements(opts AlterMaterializedViewOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName()

	// Handle SECURE as a separate SET/UNSET.
	if opts.Secure != nil {
		if *opts.Secure {
			statements = append(statements, fmt.Sprintf("ALTER MATERIALIZED VIEW %s SET SECURE", fqn))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER MATERIALIZED VIEW %s UNSET SECURE", fqn))
		}
	}

	// Build SET clause for other parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("MATERIALIZED VIEW", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a materialized view in Snowflake. Only changed fields are sent.
func (c *MaterializedViewClient) Alter(ctx context.Context, opts AlterMaterializedViewOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter materialized view options: %w", err))
	}

	stmts, err := buildAlterMaterializedViewStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter materialized view statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering materialized view %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropMaterializedViewSQL builds the DROP MATERIALIZED VIEW SQL statement.
func buildDropMaterializedViewSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("MATERIALIZED VIEW", name.FullyQualifiedName())
}

// Drop drops a materialized view from Snowflake.
func (c *MaterializedViewClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("materialized view name is required"))
	}

	if _, err := c.client.Exec(ctx, buildDropMaterializedViewSQL(name)); err != nil {
		return fmt.Errorf("dropping materialized view %s: %w", name, err)
	}

	return nil
}

// buildShowMaterializedViewByIDSQL builds a SHOW MATERIALIZED VIEWS LIKE SQL statement.
func buildShowMaterializedViewByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("MATERIALIZED VIEWS", name.Name(), scope)
}

// ShowByID queries SHOW MATERIALIZED VIEWS for a specific view name within a schema.
func (c *MaterializedViewClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.MaterializedViewShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("materialized view name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowMaterializedViewByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing materialized view %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanMaterializedViewShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a MaterializedViewObservation.
func (c *MaterializedViewClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*MaterializedViewObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &MaterializedViewObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &MaterializedViewObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanMaterializedViewShowOutput scans SHOW MATERIALIZED VIEWS results for a matching row.
func scanMaterializedViewShowOutput(rows *sql.Rows, name string) (*v1alpha1.MaterializedViewShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.MaterializedViewShowOutput, error) {
		return &v1alpha1.MaterializedViewShowOutput{
			CreatedOn:           m["created_on"],
			Name:                m["name"],
			DatabaseName:        m["database_name"],
			SchemaName:          m["schema_name"],
			ClusterBy:           m["cluster_by"],
			Rows:                m["rows"],
			Bytes:               m["bytes"],
			SourceDatabaseName:  m["source_database_name"],
			SourceSchemaName:    m["source_schema_name"],
			SourceTableName:     m["source_table_name"],
			RefreshedOn:         m["refreshed_on"],
			CompactedOn:         m["compacted_on"],
			Owner:               m["owner"],
			Invalid:             m["invalid"],
			InvalidReason:       m["invalid_reason"],
			BehindBy:            m["behind_by"],
			Comment:             m["comment"],
			Text:                m["text"],
			IsSecure:            m["is_secure"],
			AutomaticClustering: m["automatic_clustering"],
			OwnerRoleType:       m["owner_role_type"],
		}, nil
	})
}
