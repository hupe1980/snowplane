package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SecondaryDatabaseObservation holds the result of observing a Snowflake secondary database.
type SecondaryDatabaseObservation struct {
	// Exists indicates whether the database was found.
	Exists bool

	// ShowOutput contains the SHOW DATABASES row.
	ShowOutput *v1alpha1.SecondaryDatabaseShowOutput

	// Parameters contains the database-level parameters from SHOW PARAMETERS.
	Parameters *DatabaseParameters
}

// CreateSecondaryDatabaseOptions holds the parameters for creating a secondary (replica) database.
type CreateSecondaryDatabaseOptions struct {
	Name                    AccountObjectIdentifier
	AsReplicaOf             string // org.account.db_name
	DataRetentionTimeInDays *int32
}

// Validate checks the create options for validity.
func (o *CreateSecondaryDatabaseOptions) Validate() error {
	return CollectErrors(
		func() error {
			if !ValidObjectIdentifier(o.Name) {
				return fmt.Errorf("database name is required")
			}
			return nil
		},
		func() error {
			if o.AsReplicaOf == "" {
				return fmt.Errorf("asReplicaOf is required")
			}
			parts := strings.Split(o.AsReplicaOf, ".")
			if len(parts) != 3 {
				return fmt.Errorf("asReplicaOf must be in format 'org.account.db_name', got %q", o.AsReplicaOf)
			}
			return nil
		},
		func() error { return validateDataRetention(o.DataRetentionTimeInDays) },
	)
}

// AlterSecondaryDatabaseOptions holds the parameters for altering a secondary database.
type AlterSecondaryDatabaseOptions struct {
	Name                       AccountObjectIdentifier
	Comment                    *string
	DataRetentionTimeInDays    *int32
	MaxDataExtensionTimeInDays *int32
	UnsetFields                []string
}

// Validate checks the alter options for validity.
func (o *AlterSecondaryDatabaseOptions) Validate() error {
	return CollectErrors(
		func() error {
			if !ValidObjectIdentifier(o.Name) {
				return fmt.Errorf("database name is required")
			}
			return nil
		},
		func() error { return validateDataRetention(o.DataRetentionTimeInDays) },
		func() error { return validateMaxDataExtension(o.MaxDataExtensionTimeInDays) },
	)
}

// HasChanges returns true if any field in the alter options is set.
func (o *AlterSecondaryDatabaseOptions) HasChanges() bool {
	return o.Comment != nil ||
		o.DataRetentionTimeInDays != nil ||
		o.MaxDataExtensionTimeInDays != nil ||
		len(o.UnsetFields) > 0
}

// SecondaryDatabaseClient provides operations on Snowflake secondary (replica) databases.
type SecondaryDatabaseClient struct {
	client SQLExecutor
}

// NewSecondaryDatabaseClient creates a new SecondaryDatabaseClient.
func NewSecondaryDatabaseClient(c SQLExecutor) *SecondaryDatabaseClient {
	return &SecondaryDatabaseClient{client: c}
}

// buildCreateSecondaryDatabaseSQL builds the CREATE DATABASE ... AS REPLICA OF SQL.
func buildCreateSecondaryDatabaseSQL(opts CreateSecondaryDatabaseOptions) (string, error) {
	var b sqlbuilder.Builder

	fmt.Fprintf(&b.Builder, "CREATE DATABASE IF NOT EXISTS %s AS REPLICA OF %s",
		opts.Name.FullyQualifiedName(), opts.AsReplicaOf)

	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a secondary (replica) database in Snowflake.
func (c *SecondaryDatabaseClient) Create(ctx context.Context, opts CreateSecondaryDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create options: %w", err))
	}

	sql, err := buildCreateSecondaryDatabaseSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create secondary database SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating secondary database %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSecondaryDatabaseStatements builds ALTER DATABASE SET/UNSET statements.
func buildAlterSecondaryDatabaseStatements(opts AlterSecondaryDatabaseOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", opts.DataRetentionTimeInDays)
	sc.Int32("MAX_DATA_EXTENSION_TIME_IN_DAYS", opts.MaxDataExtensionTimeInDays)

	if err := sc.Err(); err != nil {
		return nil, err
	}

	return sqlbuilder.BuildAlterStatements("DATABASE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a secondary database in Snowflake.
func (c *SecondaryDatabaseClient) Alter(ctx context.Context, opts AlterSecondaryDatabaseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter options: %w", err))
	}

	stmts, err := buildAlterSecondaryDatabaseStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter secondary database SQL: %w", err))
	}

	for _, s := range stmts {
		if _, err := c.client.Exec(ctx, s); err != nil {
			return fmt.Errorf("altering secondary database %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildRefreshSQL builds the ALTER DATABASE ... REFRESH SQL statement.
func buildRefreshSQL(name AccountObjectIdentifier) string {
	return fmt.Sprintf("ALTER DATABASE %s REFRESH", name.FullyQualifiedName())
}

// Refresh triggers a replication refresh of the secondary database.
func (c *SecondaryDatabaseClient) Refresh(ctx context.Context, name AccountObjectIdentifier) error {
	if _, err := c.client.Exec(ctx, buildRefreshSQL(name)); err != nil {
		return fmt.Errorf("refreshing secondary database %s: %w", name, err)
	}

	return nil
}

// Drop drops a secondary database.
func (c *SecondaryDatabaseClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	sql := sqlbuilder.DropIfExists("DATABASE", name.FullyQualifiedName())
	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("dropping secondary database %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW DATABASES for a specific database name.
func (c *SecondaryDatabaseClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.SecondaryDatabaseShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing secondary database %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSecondaryDatabaseShowOutput(rows, name.Name())
}

// ShowParameters queries SHOW PARAMETERS IN DATABASE for a specific database.
func (c *SecondaryDatabaseClient) ShowParameters(ctx context.Context, name AccountObjectIdentifier) (*DatabaseParameters, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("database name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing parameters for secondary database %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDatabaseParameters(rows)
}

// Observe combines ShowByID and ShowParameters into a SecondaryDatabaseObservation.
func (c *SecondaryDatabaseClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SecondaryDatabaseObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SecondaryDatabaseObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := c.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SecondaryDatabaseObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

// scanSecondaryDatabaseShowOutput scans SHOW DATABASES results for a secondary database.
func scanSecondaryDatabaseShowOutput(rows *sql.Rows, name string) (*v1alpha1.SecondaryDatabaseShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SecondaryDatabaseShowOutput, error) {
		rt, _ := parseInt32(m["retention_time"])

		return &v1alpha1.SecondaryDatabaseShowOutput{
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
