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

// SecondaryConnectionObservation holds the result of observing a Snowflake secondary connection.
type SecondaryConnectionObservation struct {
	Exists     bool
	ShowOutput *v1alpha1.SecondaryConnectionShowOutput
}

// CreateSecondaryConnectionOptions holds the parameters for CREATE CONNECTION ... AS REPLICA OF.
type CreateSecondaryConnectionOptions struct {
	Name        AccountObjectIdentifier
	AsReplicaOf string
	Comment     *string
}

// Validate checks that required fields are populated.
func (o *CreateSecondaryConnectionOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	if o.AsReplicaOf == "" {
		return errors.New("asReplicaOf is required")
	}

	if err := sqlbuilder.ValidateIdentifierParts(o.AsReplicaOf); err != nil {
		return fmt.Errorf("invalid asReplicaOf identifier: %w", err)
	}

	return nil
}

// AlterSecondaryConnectionOptions holds the parameters for ALTER CONNECTION.
type AlterSecondaryConnectionOptions struct {
	Name        AccountObjectIdentifier
	Comment     *string
	UnsetFields []string
}

// HasChanges returns true if there are any SET or UNSET operations.
func (o *AlterSecondaryConnectionOptions) HasChanges() bool {
	return o.Comment != nil || len(o.UnsetFields) > 0
}

// Validate checks validity.
func (o *AlterSecondaryConnectionOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	return nil
}

// SecondaryConnectionClient provides operations on Snowflake secondary connections.
type SecondaryConnectionClient struct {
	client SQLExecutor
}

// NewSecondaryConnectionClient creates a new client.
func NewSecondaryConnectionClient(c SQLExecutor) *SecondaryConnectionClient {
	return &SecondaryConnectionClient{client: c}
}

func buildCreateSecondaryConnectionSQL(opts CreateSecondaryConnectionOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())
	replicaOf := sqlbuilder.QuoteIdentifierParts(opts.AsReplicaOf)
	stmt := fmt.Sprintf("CREATE CONNECTION IF NOT EXISTS %s AS REPLICA OF %s", fqn, replicaOf)

	if opts.Comment != nil {
		stmt += fmt.Sprintf(" COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment))
	}

	return stmt, nil
}

// Create creates a secondary connection in Snowflake.
func (c *SecondaryConnectionClient) Create(ctx context.Context, opts CreateSecondaryConnectionOptions) error {
	stmt, err := buildCreateSecondaryConnectionSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create secondary connection SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating secondary connection %s: %w", opts.Name, err)
	}

	return nil
}

func buildAlterSecondaryConnectionStatements(opts AlterSecondaryConnectionOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("CONNECTION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a secondary connection in Snowflake.
func (c *SecondaryConnectionClient) Alter(ctx context.Context, opts AlterSecondaryConnectionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter secondary connection options: %w", err))
	}

	stmts, err := buildAlterSecondaryConnectionStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter secondary connection statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering secondary connection %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a secondary connection.
func (c *SecondaryConnectionClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("CONNECTION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping secondary connection %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a secondary connection.
func (c *SecondaryConnectionClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.SecondaryConnectionShowOutput, error) {
	stmt := sqlbuilder.ShowLike("CONNECTIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing secondary connection %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanSecondaryConnectionShowOutput(rows, name.Name())
}

func scanSecondaryConnectionShowOutput(rows *sql.Rows, name string) (*v1alpha1.SecondaryConnectionShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SecondaryConnectionShowOutput, error) {
		return &v1alpha1.SecondaryConnectionShowOutput{
			CreatedOn:        m["created_on"],
			Name:             m["name"],
			SnowflakeRegion:  m["snowflake_region"],
			AccountName:      m["account_name"],
			OrganizationName: m["organization_name"],
			ConnectionURL:    m["connection_url"],
			IsPrimary:        strings.EqualFold(m["is_primary"], "true"),
			PrimaryName:      m["primary"],
			Comment:          m["comment"],
		}, nil
	})
}

// Observe combines ShowByID into a single observation.
func (c *SecondaryConnectionClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SecondaryConnectionObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &SecondaryConnectionObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &SecondaryConnectionObservation{
		Exists:     true,
		ShowOutput: showOutput,
	}, nil
}
