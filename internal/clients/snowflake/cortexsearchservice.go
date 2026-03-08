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

// CortexSearchServiceObservation holds the result of observing a Snowflake Cortex Search Service.
type CortexSearchServiceObservation struct {
	// Exists indicates whether the service was found.
	Exists bool

	// ShowOutput contains the SHOW CORTEX SEARCH SERVICES row.
	ShowOutput *v1alpha1.CortexSearchServiceShowOutput

	// DescribeOutput contains the DESCRIBE CORTEX SEARCH SERVICE output.
	DescribeOutput *v1alpha1.CortexSearchServiceDescribeOutput
}

// CreateCortexSearchServiceOptions holds the parameters for creating a Cortex Search Service.
type CreateCortexSearchServiceOptions struct {
	Name                       SchemaObjectIdentifier
	On                         string
	Attributes                 []string
	Warehouse                  string
	TargetLag                  string
	Query                      string
	EmbeddingModel             *string
	RefreshMode                *string
	Initialize                 *string
	FullIndexBuildIntervalDays *int32
	Comment                    *string
}

// validCortexSearchRefreshModes contains the allowed REFRESH_MODE values.
var validCortexSearchRefreshModes = map[string]bool{
	"FULL": true, "INCREMENTAL": true,
}

// validCortexSearchInitializeValues contains the allowed INITIALIZE values.
var validCortexSearchInitializeValues = map[string]bool{
	"ON_CREATE": true, "ON_SCHEDULE": true,
}

// Validate checks the CreateCortexSearchServiceOptions for validity.
func (o *CreateCortexSearchServiceOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("cortex search service name is required"))
	}

	if o.On == "" {
		errs = append(errs, fmt.Errorf("search column (ON) is required"))
	}

	if o.Warehouse == "" {
		errs = append(errs, fmt.Errorf("warehouse is required"))
	}

	if o.TargetLag == "" {
		errs = append(errs, fmt.Errorf("target lag is required"))
	}

	if o.Query == "" {
		errs = append(errs, fmt.Errorf("query is required"))
	}

	if o.RefreshMode != nil && !validCortexSearchRefreshModes[*o.RefreshMode] {
		errs = append(errs, fmt.Errorf("invalid refresh mode %q: must be FULL or INCREMENTAL", *o.RefreshMode))
	}

	if o.Initialize != nil && !validCortexSearchInitializeValues[*o.Initialize] {
		errs = append(errs, fmt.Errorf("invalid initialize value %q: must be ON_CREATE or ON_SCHEDULE", *o.Initialize))
	}

	return errors.Join(errs...)
}

// AlterCortexSearchServiceOptions holds the parameters for altering a Cortex Search Service.
type AlterCortexSearchServiceOptions struct {
	Name SchemaObjectIdentifier

	// TargetLag is the new target lag value.
	TargetLag *string

	// Warehouse is the new warehouse.
	Warehouse *string

	// Comment is the new comment.
	Comment *string

	// FullIndexBuildIntervalDays sets the target interval between full index rebuilds.
	FullIndexBuildIntervalDays *int32

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterCortexSearchServiceOptions for validity.
func (o *AlterCortexSearchServiceOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("cortex search service name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterCortexSearchServiceOptions) HasChanges() bool {
	return o.TargetLag != nil || o.Warehouse != nil || o.Comment != nil ||
		o.FullIndexBuildIntervalDays != nil ||
		len(o.UnsetFields) > 0
}

// buildCreateCortexSearchServiceSQL builds the CREATE CORTEX SEARCH SERVICE SQL.
func buildCreateCortexSearchServiceSQL(opts CreateCortexSearchServiceOptions) (string, error) {
	var b sqlbuilder.Builder

	b.WriteString("CREATE CORTEX SEARCH SERVICE ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.WriteString(" ON ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.On))

	if len(opts.Attributes) > 0 {
		b.WriteString(" ATTRIBUTES ")

		for i, attr := range opts.Attributes {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString(sqlbuilder.QuoteIdentifier(attr))
		}
	}

	b.WriteString(" WAREHOUSE = ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.Warehouse))

	b.WriteString(" TARGET_LAG = '")
	b.WriteString(sqlbuilder.EscapeString(opts.TargetLag))
	b.WriteString("'")

	if opts.EmbeddingModel != nil {
		b.WriteString(" EMBEDDING_MODEL = '")
		b.WriteString(sqlbuilder.EscapeString(*opts.EmbeddingModel))
		b.WriteString("'")
	}

	if opts.RefreshMode != nil {
		b.WriteString(" REFRESH_MODE = ")
		b.WriteString(*opts.RefreshMode)
	}

	if opts.Initialize != nil {
		b.WriteString(" INITIALIZE = ")
		b.WriteString(*opts.Initialize)
	}

	if opts.FullIndexBuildIntervalDays != nil {
		fmt.Fprintf(&b.Builder, " FULL_INDEX_BUILD_INTERVAL_DAYS = %d", *opts.FullIndexBuildIntervalDays)
	}

	b.SetString("COMMENT", opts.Comment)

	b.WriteString(" AS (")
	b.WriteString(opts.Query)
	b.WriteString(")")

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// buildAlterCortexSearchServiceStatements builds the ALTER CORTEX SEARCH SERVICE SQL statements.
func buildAlterCortexSearchServiceStatements(opts AlterCortexSearchServiceOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var stmts []string

	// SET operations — each uses a separate ALTER statement.
	if opts.TargetLag != nil {
		stmt := fmt.Sprintf("ALTER CORTEX SEARCH SERVICE %s SET TARGET_LAG = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.TargetLag))
		stmts = append(stmts, stmt)
	}

	if opts.Warehouse != nil {
		stmt := fmt.Sprintf("ALTER CORTEX SEARCH SERVICE %s SET WAREHOUSE = %s",
			fqn, sqlbuilder.QuoteIdentifier(*opts.Warehouse))
		stmts = append(stmts, stmt)
	}

	if opts.Comment != nil {
		stmt := fmt.Sprintf("ALTER CORTEX SEARCH SERVICE %s SET COMMENT = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.Comment))
		stmts = append(stmts, stmt)
	}

	if opts.FullIndexBuildIntervalDays != nil {
		stmt := fmt.Sprintf("ALTER CORTEX SEARCH SERVICE %s SET FULL_INDEX_BUILD_INTERVAL_DAYS = %d",
			fqn, *opts.FullIndexBuildIntervalDays)
		stmts = append(stmts, stmt)
	}

	// UNSET fields (e.g. COMMENT).
	for _, field := range opts.UnsetFields {
		stmt := fmt.Sprintf("ALTER CORTEX SEARCH SERVICE %s UNSET %s", fqn, field)
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// CortexSearchServiceClient provides operations against Snowflake Cortex Search Services.
type CortexSearchServiceClient struct {
	client SQLExecutor
}

// NewCortexSearchServiceClient creates a new CortexSearchServiceClient.
func NewCortexSearchServiceClient(c SQLExecutor) *CortexSearchServiceClient {
	return &CortexSearchServiceClient{client: c}
}

// Create creates a Cortex Search Service in Snowflake.
func (c *CortexSearchServiceClient) Create(ctx context.Context, opts CreateCortexSearchServiceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create cortex search service options: %w", err))
	}

	sql, err := buildCreateCortexSearchServiceSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create cortex search service SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating cortex search service %s: %w", opts.Name, err)
	}

	return nil
}

// Alter alters a Cortex Search Service in Snowflake.
func (c *CortexSearchServiceClient) Alter(ctx context.Context, opts AlterCortexSearchServiceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter cortex search service options: %w", err))
	}

	stmts, err := buildAlterCortexSearchServiceStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter cortex search service statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering cortex search service %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a Cortex Search Service from Snowflake.
func (c *CortexSearchServiceClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("cortex search service name is required"))
	}

	stmt := sqlbuilder.DropIfExists("CORTEX SEARCH SERVICE", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping cortex search service %s: %w", name, err)
	}

	return nil
}

// buildShowCortexSearchServiceByIDSQL builds a SHOW CORTEX SEARCH SERVICES LIKE SQL.
func buildShowCortexSearchServiceByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("CORTEX SEARCH SERVICES", name.Name(), scope)
}

// ShowByID queries SHOW CORTEX SEARCH SERVICES for a specific service within a schema.
func (c *CortexSearchServiceClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.CortexSearchServiceShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("cortex search service name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowCortexSearchServiceByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing cortex search service %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanCortexSearchServiceShowOutput(rows, name.Name())
}

// buildDescribeCortexSearchServiceSQL builds the DESCRIBE CORTEX SEARCH SERVICE SQL.
func buildDescribeCortexSearchServiceSQL(name SchemaObjectIdentifier) string {
	return fmt.Sprintf("DESCRIBE CORTEX SEARCH SERVICE %s", name.FullyQualifiedName())
}

// Describe queries DESCRIBE CORTEX SEARCH SERVICE.
func (c *CortexSearchServiceClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.CortexSearchServiceDescribeOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("cortex search service name is required"))
	}

	rows, err := c.client.Query(ctx, buildDescribeCortexSearchServiceSQL(name))
	if err != nil {
		return nil, fmt.Errorf("describing cortex search service %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanCortexSearchServiceDescribeOutput(rows)
}

// Observe combines ShowByID and Describe into a CortexSearchServiceObservation.
func (c *CortexSearchServiceClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*CortexSearchServiceObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &CortexSearchServiceObservation{Exists: false}, nil
		}

		return nil, err
	}

	obs := &CortexSearchServiceObservation{
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

// scanCortexSearchServiceShowOutput scans SHOW CORTEX SEARCH SERVICES results for a matching row.
func scanCortexSearchServiceShowOutput(rows *sql.Rows, name string) (*v1alpha1.CortexSearchServiceShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.CortexSearchServiceShowOutput, error) {
		return &v1alpha1.CortexSearchServiceShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Warehouse:    m["warehouse"],
			TargetLag:    m["target_lag"],
			Comment:      m["comment"],
			SearchColumn: m["search_column"],
			Definition:   m["definition"],
		}, nil
	})
}

// scanCortexSearchServiceDescribeOutput scans DESCRIBE CORTEX SEARCH SERVICE results.
func scanCortexSearchServiceDescribeOutput(rows *sql.Rows) (*v1alpha1.CortexSearchServiceDescribeOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	result := &v1alpha1.CortexSearchServiceDescribeOutput{}

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

	result.EmbeddingModel = m["embedding_model"]
	result.IndexingState = m["indexing_state"]
	result.ServingState = m["serving_state"]
	result.SourceDataNumRows = m["source_data_num_rows"]

	return result, nil
}
