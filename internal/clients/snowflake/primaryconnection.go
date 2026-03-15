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

// PrimaryConnectionObservation holds the result of observing a Snowflake primary connection.
type PrimaryConnectionObservation struct {
	Exists     bool
	ShowOutput *v1alpha1.PrimaryConnectionShowOutput
}

// CreatePrimaryConnectionOptions holds the parameters for CREATE CONNECTION.
type CreatePrimaryConnectionOptions struct {
	Name    AccountObjectIdentifier
	Comment *string
}

// Validate checks that required fields are populated.
func (o *CreatePrimaryConnectionOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	return nil
}

// AlterPrimaryConnectionOptions holds the parameters for ALTER CONNECTION.
type AlterPrimaryConnectionOptions struct {
	Name                     AccountObjectIdentifier
	EnableFailoverToAccounts *[]string
	Comment                  *string
	UnsetFields              []string
}

// HasChanges returns true if there are any SET or UNSET operations.
func (o *AlterPrimaryConnectionOptions) HasChanges() bool {
	return o.EnableFailoverToAccounts != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity.
func (o *AlterPrimaryConnectionOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	if o.EnableFailoverToAccounts != nil {
		for _, a := range *o.EnableFailoverToAccounts {
			if err := sqlbuilder.ValidateIdentifierParts(a); err != nil {
				return fmt.Errorf("invalid failover account identifier %q: %w", a, err)
			}
		}
	}

	return nil
}

// PrimaryConnectionClient provides operations on Snowflake primary connections.
type PrimaryConnectionClient struct {
	client SQLExecutor
}

// NewPrimaryConnectionClient creates a new client.
func NewPrimaryConnectionClient(c SQLExecutor) *PrimaryConnectionClient {
	return &PrimaryConnectionClient{client: c}
}

func buildCreatePrimaryConnectionSQL(opts CreatePrimaryConnectionOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "CONNECTION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a primary connection in Snowflake.
func (c *PrimaryConnectionClient) Create(ctx context.Context, opts CreatePrimaryConnectionOptions) error {
	stmt, err := buildCreatePrimaryConnectionSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create primary connection SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating primary connection %s: %w", opts.Name, err)
	}

	return nil
}

func buildAlterPrimaryConnectionStatements(opts AlterPrimaryConnectionOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())
	var stmts []string

	// ENABLE FAILOVER TO ACCOUNTS is a special ALTER syntax, not a SET clause.
	if opts.EnableFailoverToAccounts != nil {
		if len(*opts.EnableFailoverToAccounts) > 0 {
			quoted := make([]string, len(*opts.EnableFailoverToAccounts))
			for i, a := range *opts.EnableFailoverToAccounts {
				quoted[i] = sqlbuilder.QuoteIdentifierParts(a)
			}

			stmts = append(stmts, fmt.Sprintf("ALTER CONNECTION %s ENABLE FAILOVER TO ACCOUNTS %s", fqn, strings.Join(quoted, ", ")))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER CONNECTION %s DISABLE FAILOVER", fqn))
		}
	}

	// SET/UNSET for COMMENT.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)

	setUnsetStmts, err := sqlbuilder.BuildAlterStatements("CONNECTION", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, setUnsetStmts...)

	return stmts, nil
}

// Alter alters a primary connection in Snowflake.
func (c *PrimaryConnectionClient) Alter(ctx context.Context, opts AlterPrimaryConnectionOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter primary connection options: %w", err))
	}

	stmts, err := buildAlterPrimaryConnectionStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter primary connection statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering primary connection %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a primary connection.
func (c *PrimaryConnectionClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("CONNECTION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping primary connection %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a primary connection.
func (c *PrimaryConnectionClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.PrimaryConnectionShowOutput, error) {
	stmt := sqlbuilder.ShowLike("CONNECTIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing primary connection %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPrimaryConnectionShowOutput(rows, name.Name())
}

func scanPrimaryConnectionShowOutput(rows *sql.Rows, name string) (*v1alpha1.PrimaryConnectionShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.PrimaryConnectionShowOutput, error) {
		return &v1alpha1.PrimaryConnectionShowOutput{
			CreatedOn:         m["created_on"],
			Name:              m["name"],
			SnowflakeRegion:   m["snowflake_region"],
			AccountName:       m["account_name"],
			OrganizationName:  m["organization_name"],
			ConnectionURL:     m["connection_url"],
			IsPrimary:         strings.EqualFold(m["is_primary"], "true"),
			FailoverAllowedTo: m["failover_allowed_to_accounts"],
			Comment:           m["comment"],
		}, nil
	})
}

// Observe combines ShowByID into a single observation.
func (c *PrimaryConnectionClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*PrimaryConnectionObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &PrimaryConnectionObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &PrimaryConnectionObservation{
		Exists:     true,
		ShowOutput: showOutput,
	}, nil
}
