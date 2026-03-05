package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SharedDatabaseObservation holds the result of observing a Snowflake shared database.
type SharedDatabaseObservation struct {
	// Exists indicates whether the database was found.
	Exists bool

	// ShowOutput contains the SHOW DATABASES row.
	ShowOutput *v1alpha1.SharedDatabaseShowOutput

	// Parameters contains the database-level parameters from SHOW PARAMETERS.
	Parameters *DatabaseParameters
}

// CreateSharedDatabaseOptions holds the parameters for creating a database from a share.
type CreateSharedDatabaseOptions struct {
	Name      AccountObjectIdentifier
	FromShare string // <provider_account>.<share_name>
}

// Validate checks the create options for validity.
func (o *CreateSharedDatabaseOptions) Validate() error {
	return CollectErrors(
		func() error {
			if !ValidObjectIdentifier(o.Name) {
				return fmt.Errorf("database name is required")
			}
			return nil
		},
		func() error {
			if o.FromShare == "" {
				return fmt.Errorf("fromShare is required")
			}
			parts := strings.Split(o.FromShare, ".")
			if len(parts) < 2 {
				return fmt.Errorf("fromShare must be in format '<provider_account>.<share_name>', got %q", o.FromShare)
			}
			return nil
		},
	)
}

// AlterSharedDatabaseOptions holds the parameters for altering a shared database.
type AlterSharedDatabaseOptions struct {
	Name                       AccountObjectIdentifier
	Comment                    *string
	ExternalVolume             *string
	Catalog                    *string
	DefaultDDLCollation        *string
	ReplaceInvalidCharacters   *bool
	StorageSerializationPolicy *string
	LogLevel                   *string
	TraceLevel                 *string
	UnsetFields                []string
}

// Validate checks the alter options for validity.
func (o *AlterSharedDatabaseOptions) Validate() error {
	return CollectErrors(
		func() error {
			if !ValidObjectIdentifier(o.Name) {
				return fmt.Errorf("database name is required")
			}
			return nil
		},
		func() error { return validateStorageSerializationPolicy(o.StorageSerializationPolicy) },
		func() error { return validateLogLevel(o.LogLevel) },
		func() error { return validateTraceLevel(o.TraceLevel) },
	)
}

// HasChanges returns true if any field in the alter options is set.
func (o *AlterSharedDatabaseOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.ExternalVolume != nil ||
		o.Catalog != nil ||
		o.DefaultDDLCollation != nil ||
		o.ReplaceInvalidCharacters != nil ||
		o.StorageSerializationPolicy != nil ||
		o.LogLevel != nil ||
		o.TraceLevel != nil ||
		len(o.UnsetFields) > 0
}

// SharedDatabaseClient provides operations on Snowflake shared (from share) databases.
type SharedDatabaseClient struct {
	client SQLExecutor
}

// NewSharedDatabaseClient creates a new SharedDatabaseClient.
func NewSharedDatabaseClient(c SQLExecutor) *SharedDatabaseClient {
	return &SharedDatabaseClient{client: c}
}

// buildCreateSharedDatabaseSQL builds the CREATE DATABASE ... FROM SHARE SQL.
func buildCreateSharedDatabaseSQL(opts CreateSharedDatabaseOptions) (string, error) {
	var b sqlbuilder.Builder

	fmt.Fprintf(&b.Builder, "CREATE DATABASE IF NOT EXISTS %s FROM SHARE %s",
		opts.Name.FullyQualifiedName(), opts.FromShare)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a shared database in Snowflake.
func (c *SharedDatabaseClient) Create(ctx context.Context, opts CreateSharedDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create options: %w", err))
	}

	sqlStr, err := buildCreateSharedDatabaseSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create shared database SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sqlStr); err != nil {
		return fmt.Errorf("creating shared database %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSharedDatabaseStatements builds ALTER DATABASE SET/UNSET statements.
func buildAlterSharedDatabaseStatements(opts AlterSharedDatabaseOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.String("EXTERNAL_VOLUME", opts.ExternalVolume)
	sc.String("CATALOG", opts.Catalog)
	sc.String("DEFAULT_DDL_COLLATION", opts.DefaultDDLCollation)
	sc.Bool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	sc.Keyword("STORAGE_SERIALIZATION_POLICY", opts.StorageSerializationPolicy)
	sc.QuotedKeyword("LOG_LEVEL", opts.LogLevel)
	sc.Keyword("TRACE_LEVEL", opts.TraceLevel)

	if err := sc.Err(); err != nil {
		return nil, err
	}

	return sqlbuilder.BuildAlterStatements("DATABASE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a shared database in Snowflake.
func (c *SharedDatabaseClient) Alter(ctx context.Context, opts AlterSharedDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter options: %w", err))
	}

	stmts, err := buildAlterSharedDatabaseStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter shared database SQL: %w", err))
	}

	for _, s := range stmts {
		if _, err := c.client.Exec(ctx, s); err != nil {
			return fmt.Errorf("altering shared database %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a shared database.
func (c *SharedDatabaseClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	sqlStr := sqlbuilder.DropIfExists("DATABASE", name.FullyQualifiedName())
	if _, err := c.client.Exec(ctx, sqlStr); err != nil {
		return fmt.Errorf("dropping shared database %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW DATABASES for a specific database name.
func (c *SharedDatabaseClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.SharedDatabaseShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing shared database %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSharedDatabaseShowOutput(rows, name.Name())
}

// ShowParameters queries SHOW PARAMETERS IN DATABASE for a specific database.
func (c *SharedDatabaseClient) ShowParameters(ctx context.Context, name AccountObjectIdentifier) (*DatabaseParameters, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing parameters for shared database %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDatabaseParameters(rows)
}

// Observe combines ShowByID and ShowParameters into a SharedDatabaseObservation.
func (c *SharedDatabaseClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SharedDatabaseObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SharedDatabaseObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := c.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SharedDatabaseObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

// scanSharedDatabaseShowOutput scans SHOW DATABASES results for a shared database.
func scanSharedDatabaseShowOutput(rows *sql.Rows, name string) (*v1alpha1.SharedDatabaseShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SharedDatabaseShowOutput, error) {
		rt, _ := parseInt32(m["retention_time"])

		return &v1alpha1.SharedDatabaseShowOutput{
			CreatedOn:     m["created_on"],
			Name:          m["name"],
			Kind:          m["kind"],
			Comment:       m["comment"],
			Owner:         m["owner"],
			RetentionTime: rt,
			Origin:        m["origin"],
		}, nil
	})
}
