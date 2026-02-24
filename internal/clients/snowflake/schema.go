package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SchemaObservation holds the result of observing a Snowflake schema.
type SchemaObservation struct {
	// Exists indicates whether the schema was found.
	Exists bool

	// ShowOutput contains the SHOW SCHEMAS row.
	ShowOutput *SchemaShowOutput

	// Parameters contains the schema-level parameters from SHOW PARAMETERS.
	Parameters *SchemaParameters
}

// SchemaShowOutput contains the fields from SHOW SCHEMAS.
type SchemaShowOutput struct {
	CreatedOn     string
	Name          string
	DatabaseName  string
	Kind          string // STANDARD or TRANSIENT
	Comment       string
	Owner         string
	RetentionTime int32
	Options       string // Contains "MANAGED ACCESS" if managed access is enabled
}

// IsManagedAccess returns true if the schema has managed access enabled.
func (o *SchemaShowOutput) IsManagedAccess() bool {
	return strings.Contains(o.Options, "MANAGED ACCESS")
}

// SchemaParameters contains relevant schema parameters from SHOW PARAMETERS IN SCHEMA.
type SchemaParameters struct {
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	DefaultDDLCollation        string
	ReplaceInvalidCharacters   *bool
	StorageSerializationPolicy string
	LogLevel                   string
	MetricLevel                string
	TraceLevel                 string
}

// CreateSchemaOptions holds the parameters for creating a schema.
type CreateSchemaOptions struct {
	Name                       DatabaseObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	Transient                  bool
	ManagedAccess              bool
	DefaultDDLCollation        *string
	ReplaceInvalidCharacters   *bool
	StorageSerializationPolicy *string
	LogLevel                   *string
	MetricLevel                *string
	TraceLevel                 *string

	// UseCreateOrAlter emits CREATE OR ALTER SCHEMA instead of
	// CREATE SCHEMA IF NOT EXISTS. Requires Snowflake support.
	UseCreateOrAlter bool
}

// Validate checks the CreateSchemaOptions for validity.
func (o *CreateSchemaOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("schema name is required"))
	}

	if err := validateDataRetention(o.DataRetentionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateMaxDataExtension(o.MaxDataExtensionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateStorageSerializationPolicy(o.StorageSerializationPolicy); err != nil {
		errs = append(errs, err)
	}

	if err := validateLogLevel(o.LogLevel); err != nil {
		errs = append(errs, err)
	}

	if err := validateMetricLevel(o.MetricLevel); err != nil {
		errs = append(errs, err)
	}

	if err := validateTraceLevel(o.TraceLevel); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// AlterSchemaOptions holds the parameters for altering a schema.
type AlterSchemaOptions struct {
	Name                       DatabaseObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	DefaultDDLCollation        *string
	ReplaceInvalidCharacters   *bool
	StorageSerializationPolicy *string
	LogLevel                   *string
	MetricLevel                *string
	TraceLevel                 *string

	// SetManagedAccess toggles managed access mode on an existing schema.
	// Snowflake requires a separate ALTER SCHEMA ... ENABLE/DISABLE MANAGED ACCESS
	// statement (not a SET/UNSET parameter).
	SetManagedAccess *bool

	// UnsetFields lists Snowflake parameter names to revert to their server-side
	// defaults via ALTER SCHEMA ... UNSET. Used when a previously-managed spec
	// field is removed (set to nil).
	UnsetFields []string
}

// Validate checks the AlterSchemaOptions for validity.
func (o *AlterSchemaOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("schema name is required"))
	}

	if err := validateDataRetention(o.DataRetentionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateMaxDataExtension(o.MaxDataExtensionTimeInDays); err != nil {
		errs = append(errs, err)
	}

	if err := validateStorageSerializationPolicy(o.StorageSerializationPolicy); err != nil {
		errs = append(errs, err)
	}

	if err := validateLogLevel(o.LogLevel); err != nil {
		errs = append(errs, err)
	}

	if err := validateMetricLevel(o.MetricLevel); err != nil {
		errs = append(errs, err)
	}

	if err := validateTraceLevel(o.TraceLevel); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterSchemaOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.DataRetentionTimeInDays != nil ||
		o.MaxDataExtensionTimeInDays != nil ||
		o.DefaultDDLCollation != nil ||
		o.ReplaceInvalidCharacters != nil ||
		o.StorageSerializationPolicy != nil ||
		o.LogLevel != nil ||
		o.MetricLevel != nil ||
		o.TraceLevel != nil ||
		o.SetManagedAccess != nil ||
		len(o.UnsetFields) > 0
}

// SchemaClient provides operations against Snowflake schemas.
type SchemaClient struct {
	client SQLExecutor
}

// NewSchemaClient creates a new SchemaClient backed by the given SQLExecutor.
func NewSchemaClient(c SQLExecutor) *SchemaClient {
	return &SchemaClient{client: c}
}

// buildCreateSchemaSQL builds the CREATE SCHEMA SQL statement.
func buildCreateSchemaSQL(opts CreateSchemaOptions) (string, error) {
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
		b.WriteString(" SCHEMA ")
	} else {
		b.WriteString(" SCHEMA IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())

	if opts.ManagedAccess {
		b.WriteString(" WITH MANAGED ACCESS")
	}

	b.SetString("COMMENT", opts.Comment)
	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	b.SetInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	b.SetBool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	b.SetString("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	b.SetKeyword("STORAGE_SERIALIZATION_POLICY", opts.StorageSerializationPolicy)
	b.SetQuotedKeyword("LOG_LEVEL", opts.LogLevel)
	b.SetKeyword("METRIC_LEVEL", opts.MetricLevel)
	b.SetKeyword("TRACE_LEVEL", opts.TraceLevel)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a schema in Snowflake.
func (s *SchemaClient) Create(ctx context.Context, opts CreateSchemaOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create schema options: %w", err))
	}

	sql, err := buildCreateSchemaSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create schema SQL: %w", err))
	}

	if _, err := s.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating schema %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSchemaStatements builds the ALTER SCHEMA SQL statement(s).
// Returns a slice of separate statements: MANAGED ACCESS toggle, SET, and UNSET.
func buildAlterSchemaStatements(opts AlterSchemaOptions) ([]string, error) {
	var statements []string

	// Handle MANAGED ACCESS toggle as a separate statement.
	if opts.SetManagedAccess != nil {
		action := "DISABLE"
		if *opts.SetManagedAccess {
			action = "ENABLE"
		}

		statements = append(statements, fmt.Sprintf("ALTER SCHEMA %s %s MANAGED ACCESS",
			opts.Name.FullyQualifiedName(), action))
	}

	// Build SET clause for all other parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	sc.Int32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	sc.Bool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	sc.String("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	sc.Keyword("STORAGE_SERIALIZATION_POLICY", opts.StorageSerializationPolicy)
	sc.QuotedKeyword("LOG_LEVEL", opts.LogLevel)
	sc.Keyword("METRIC_LEVEL", opts.MetricLevel)
	sc.Keyword("TRACE_LEVEL", opts.TraceLevel)

	alterStmts, err := sqlbuilder.BuildAlterStatements("SCHEMA", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a schema in Snowflake. Only changed fields are sent.
// MANAGED ACCESS, SET, and UNSET changes are executed as separate statements.
func (s *SchemaClient) Alter(ctx context.Context, opts AlterSchemaOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter schema options: %w", err))
	}

	stmts, err := buildAlterSchemaStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter schema statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := s.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering schema %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropSchemaSQL builds the DROP SCHEMA SQL statement.
func buildDropSchemaSQL(name DatabaseObjectIdentifier) string {
	return sqlbuilder.DropIfExists("SCHEMA", name.FullyQualifiedName())
}

// Drop drops a schema from Snowflake.
func (s *SchemaClient) Drop(ctx context.Context, name DatabaseObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("schema name is required"))
	}

	if _, err := s.client.Exec(ctx, buildDropSchemaSQL(name)); err != nil {
		return fmt.Errorf("dropping schema %s: %w", name, err)
	}

	return nil
}

// buildShowSchemaByIDSQL builds the SHOW SCHEMAS LIKE SQL statement scoped to a database.
func buildShowSchemaByIDSQL(name DatabaseObjectIdentifier) string {
	return sqlbuilder.ShowLikeIn("SCHEMAS", name.Name(), "DATABASE "+sqlbuilder.QuoteIdentifier(name.DatabaseName()))
}

// ShowByID queries SHOW SCHEMAS for a specific schema name within a database.
func (s *SchemaClient) ShowByID(ctx context.Context, name DatabaseObjectIdentifier) (*SchemaShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("schema name is required"))
	}

	rows, err := s.client.Query(ctx, buildShowSchemaByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing schema %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSchemaShowOutput(rows, name.Name())
}

// buildShowSchemaParametersSQL builds the SHOW PARAMETERS IN SCHEMA SQL statement.
func buildShowSchemaParametersSQL(name DatabaseObjectIdentifier) string {
	return sqlbuilder.ShowParameters("SCHEMA", name.FullyQualifiedName())
}

// ShowParameters queries SHOW PARAMETERS IN SCHEMA for a specific schema.
func (s *SchemaClient) ShowParameters(ctx context.Context, name DatabaseObjectIdentifier) (*SchemaParameters, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("schema name is required"))
	}

	rows, err := s.client.Query(ctx, buildShowSchemaParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing parameters for schema %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSchemaParameters(rows)
}

// Observe combines ShowByID and ShowParameters into a SchemaObservation.
func (s *SchemaClient) Observe(ctx context.Context, name DatabaseObjectIdentifier) (*SchemaObservation, error) {
	show, err := s.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SchemaObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := s.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SchemaObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

// scanSchemaShowOutput scans SHOW SCHEMAS results for a matching row.
func scanSchemaShowOutput(rows *sql.Rows, name string) (*SchemaShowOutput, error) {
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

		return &SchemaShowOutput{
			CreatedOn:     colMap["created_on"],
			Name:          colMap["name"],
			DatabaseName:  colMap["database_name"],
			Kind:          colMap["kind"],
			Comment:       colMap["comment"],
			Owner:         colMap["owner"],
			RetentionTime: rt,
			Options:       colMap["options"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}

// scanSchemaParameters parses SHOW PARAMETERS results into SchemaParameters.
func scanSchemaParameters(rows *sql.Rows) (*SchemaParameters, error) {
	params := &SchemaParameters{}

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
			return nil, fmt.Errorf("scanning parameter row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		key := strings.ToUpper(colMap["key"])
		val := colMap["value"]

		switch key {
		case "DATA_RETENTION_TIME_IN_DAYS":
			if v, ok := parseInt32(val); ok {
				params.DataRetentionTimeInDays = &v
			}
		case "MAX_DATA_EXTENSION_TIME_IN_DAYS":
			if v, ok := parseInt32(val); ok {
				params.MaxDataExtensionTimeInDays = &v
			}
		case "DEFAULT_DDL_COLLATION":
			params.DefaultDDLCollation = val
		case "REPLACE_INVALID_CHARACTERS":
			v := strings.EqualFold(val, "true")
			params.ReplaceInvalidCharacters = &v
		case "STORAGE_SERIALIZATION_POLICY":
			params.StorageSerializationPolicy = val
		case "LOG_LEVEL":
			params.LogLevel = val
		case "METRIC_LEVEL":
			params.MetricLevel = val
		case "TRACE_LEVEL":
			params.TraceLevel = val
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parameter rows: %w", err)
	}

	return params, nil
}
