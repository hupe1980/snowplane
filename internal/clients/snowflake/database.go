package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// Shared validation helpers for database option enums.
func validateDataRetention(v *int32) error {
	if v != nil {
		if *v < 0 || *v > 90 {
			return fmt.Errorf("dataRetentionTimeInDays must be 0–90, got %d", *v)
		}
	}

	return nil
}

func validateStorageSerializationPolicy(v *string) error {
	if v != nil {
		switch *v {
		case "COMPATIBLE", "OPTIMIZED":
		default:
			return fmt.Errorf("storageSerializationPolicy must be COMPATIBLE or OPTIMIZED, got %q", *v)
		}
	}

	return nil
}

func validateLogLevel(v *string) error {
	if v != nil {
		switch *v {
		case "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "OFF":
		default:
			return fmt.Errorf("logLevel must be one of TRACE, DEBUG, INFO, WARN, ERROR, FATAL, OFF — got %q", *v)
		}
	}

	return nil
}

func validateMetricLevel(v *string) error {
	if v != nil {
		switch *v {
		case "NONE", "ALL":
		default:
			return fmt.Errorf("metricLevel must be NONE or ALL, got %q", *v)
		}
	}

	return nil
}

func validateTraceLevel(v *string) error {
	if v != nil {
		switch *v {
		case "ALWAYS", "ON_EVENT", "OFF":
		default:
			return fmt.Errorf("traceLevel must be ALWAYS, ON_EVENT, or OFF — got %q", *v)
		}
	}

	return nil
}

func validateMaxDataExtension(v *int32) error {
	if v != nil {
		if *v < 0 || *v > 90 {
			return fmt.Errorf("maxDataExtensionTimeInDays must be 0–90, got %d", *v)
		}
	}

	return nil
}

// DatabaseObservation holds the result of observing a Snowflake database.
type DatabaseObservation struct {
	// Exists indicates whether the database was found.
	Exists bool

	// ShowOutput contains the SHOW DATABASES row.
	ShowOutput *DatabaseShowOutput

	// Parameters contains the database-level parameters from SHOW PARAMETERS.
	Parameters *DatabaseParameters
}

// DatabaseShowOutput contains the fields from SHOW DATABASES.
type DatabaseShowOutput struct {
	CreatedOn     string
	Name          string
	Kind          string // STANDARD or TRANSIENT
	Comment       string
	Owner         string
	RetentionTime int32
}

// DatabaseParameters contains relevant database parameters from SHOW PARAMETERS IN DATABASE.
type DatabaseParameters struct {
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	DefaultDDLCollation        string
	ReplaceInvalidCharacters   *bool
	Catalog                    string
	ExternalVolume             string
	StorageSerializationPolicy string
	LogLevel                   string
	MetricLevel                string
	TraceLevel                 string
}

// CreateDatabaseOptions holds the parameters for creating a database.
type CreateDatabaseOptions struct {
	Name                       AccountObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	Transient                  bool
	Catalog                    *string
	ExternalVolume             *string
	ReplaceInvalidCharacters   *bool
	DefaultDDLCollation        *string
	StorageSerializationPolicy *string
	LogLevel                   *string
	MetricLevel                *string
	TraceLevel                 *string

	// UseCreateOrAlter emits CREATE OR ALTER DATABASE instead of
	// CREATE DATABASE IF NOT EXISTS. Requires Snowflake 2024+ support.
	UseCreateOrAlter bool
}

// Validate checks the CreateDatabaseOptions for validity.
func (o *CreateDatabaseOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("database name is required"))
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

// AlterDatabaseOptions holds the parameters for altering a database.
type AlterDatabaseOptions struct {
	Name                       AccountObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	Catalog                    *string
	ExternalVolume             *string
	ReplaceInvalidCharacters   *bool
	DefaultDDLCollation        *string
	StorageSerializationPolicy *string
	LogLevel                   *string
	MetricLevel                *string
	TraceLevel                 *string

	// UnsetFields lists Snowflake parameter names to revert to their server-side
	// defaults via ALTER DATABASE ... UNSET. Used when a previously-managed spec
	// field is removed (set to nil).
	UnsetFields []string
}

// Validate checks the AlterDatabaseOptions for validity.
func (o *AlterDatabaseOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("database name is required"))
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
func (o *AlterDatabaseOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.DataRetentionTimeInDays != nil ||
		o.MaxDataExtensionTimeInDays != nil ||
		o.Catalog != nil ||
		o.ExternalVolume != nil ||
		o.ReplaceInvalidCharacters != nil ||
		o.DefaultDDLCollation != nil ||
		o.StorageSerializationPolicy != nil ||
		o.LogLevel != nil ||
		o.MetricLevel != nil ||
		o.TraceLevel != nil ||
		len(o.UnsetFields) > 0
}

// DatabaseClient provides operations against Snowflake databases.
type DatabaseClient struct {
	client SQLExecutor
}

// NewDatabaseClient creates a new DatabaseClient backed by the given SQLExecutor.
func NewDatabaseClient(c SQLExecutor) *DatabaseClient {
	return &DatabaseClient{client: c}
}

// buildCreateSQL builds the CREATE DATABASE SQL statement from the given options.
func buildCreateSQL(opts CreateDatabaseOptions) (string, error) {
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
		b.WriteString(" DATABASE ")
	} else {
		b.WriteString(" DATABASE IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetString("COMMENT", opts.Comment)
	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	b.SetInt32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	b.SetString("CATALOG", opts.Catalog)
	b.SetString("EXTERNAL_VOLUME", opts.ExternalVolume)
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

// Create creates a database in Snowflake.
func (d *DatabaseClient) Create(ctx context.Context, opts CreateDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create options: %w", err))
	}

	sql, err := buildCreateSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create database SQL: %w", err))
	}

	if _, err := d.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating database %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStatements builds the ALTER DATABASE SQL statement(s).
// Returns a SET statement and/or an UNSET statement depending on the options.
func buildAlterStatements(opts AlterDatabaseOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	sc.Int32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)
	sc.String("CATALOG", opts.Catalog)
	sc.String("EXTERNAL_VOLUME", opts.ExternalVolume)
	sc.Bool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	sc.String("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	sc.Keyword("STORAGE_SERIALIZATION_POLICY", opts.StorageSerializationPolicy)
	sc.QuotedKeyword("LOG_LEVEL", opts.LogLevel)
	sc.Keyword("METRIC_LEVEL", opts.MetricLevel)
	sc.Keyword("TRACE_LEVEL", opts.TraceLevel)

	return sqlbuilder.BuildAlterStatements("DATABASE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a database in Snowflake. Only changed fields are sent.
// SET and UNSET are executed as separate statements when both are needed.
func (d *DatabaseClient) Alter(ctx context.Context, opts AlterDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter options: %w", err))
	}

	stmts, err := buildAlterStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter database statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := d.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering database %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropSQL builds the DROP DATABASE SQL statement.
func buildDropSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.DropIfExists("DATABASE", name.FullyQualifiedName())
}

// Drop drops a database from Snowflake.
func (d *DatabaseClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("database name is required"))
	}

	if _, err := d.client.Exec(ctx, buildDropSQL(name)); err != nil {
		return fmt.Errorf("dropping database %s: %w", name, err)
	}

	return nil
}

// buildShowByIDSQL builds the SHOW DATABASES LIKE SQL statement.
func buildShowByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("DATABASES", name.Name())
}

// ShowByID queries SHOW DATABASES for a specific database name.
func (d *DatabaseClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*DatabaseShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := d.client.Query(ctx, buildShowByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing database %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDatabaseShowOutput(rows, name.Name())
}

// buildShowParametersSQL builds the SHOW PARAMETERS IN DATABASE SQL statement.
func buildShowParametersSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowParameters("DATABASE", name.FullyQualifiedName())
}

// ShowParameters queries SHOW PARAMETERS IN DATABASE for a specific database.
func (d *DatabaseClient) ShowParameters(ctx context.Context, name AccountObjectIdentifier) (*DatabaseParameters, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := d.client.Query(ctx, buildShowParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing parameters for database %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDatabaseParameters(rows)
}

// Observe combines ShowByID and ShowParameters into a DatabaseObservation.
func (d *DatabaseClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*DatabaseObservation, error) {
	show, err := d.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &DatabaseObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := d.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &DatabaseObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

// scanDatabaseShowOutput scans SHOW DATABASES results for a matching row.
func scanDatabaseShowOutput(rows *sql.Rows, name string) (*DatabaseShowOutput, error) {
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

		return &DatabaseShowOutput{
			CreatedOn:     colMap["created_on"],
			Name:          colMap["name"],
			Kind:          colMap["kind"],
			Comment:       colMap["comment"],
			Owner:         colMap["owner"],
			RetentionTime: rt,
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}

// scanDatabaseParameters parses SHOW PARAMETERS results into DatabaseParameters.
func scanDatabaseParameters(rows *sql.Rows) (*DatabaseParameters, error) {
	params := &DatabaseParameters{}

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
		case "CATALOG":
			params.Catalog = val
		case "EXTERNAL_VOLUME":
			params.ExternalVolume = val
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
