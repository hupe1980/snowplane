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

// SCIMIntegrationObservation holds the result of observing a Snowflake SCIM security integration.
type SCIMIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW SECURITY INTEGRATIONS row.
	ShowOutput *v1alpha1.SCIMIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateSCIMIntegrationOptions holds the parameters for creating a SCIM security integration.
type CreateSCIMIntegrationOptions struct {
	Name          AccountObjectIdentifier
	Enabled       *bool
	SCIMClient    string
	RunAsRole     string
	NetworkPolicy *string
	SyncPassword  *bool
	Comment       *string
}

// Validate checks that required fields are populated.
func (o *CreateSCIMIntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.SCIMClient == "" {
		errs = append(errs, errors.New("scim_client is required"))
	}

	if o.RunAsRole == "" {
		errs = append(errs, errors.New("run_as_role is required"))
	}

	return errors.Join(errs...)
}

// AlterSCIMIntegrationOptions holds the parameters for altering a SCIM security integration.
type AlterSCIMIntegrationOptions struct {
	Name          AccountObjectIdentifier
	Enabled       *bool
	NetworkPolicy *string
	SyncPassword  *bool
	Comment       *string
	UnsetFields   []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterSCIMIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.NetworkPolicy != nil ||
		o.SyncPassword != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterSCIMIntegrationOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	return nil
}

// SCIMIntegrationClient provides operations on Snowflake SCIM security integrations.
type SCIMIntegrationClient struct {
	client SQLExecutor
}

// NewSCIMIntegrationClient creates a new SCIMIntegrationClient.
func NewSCIMIntegrationClient(c SQLExecutor) *SCIMIntegrationClient {
	return &SCIMIntegrationClient{client: c}
}

// buildCreateSCIMIntegrationSQL builds the CREATE SECURITY INTEGRATION SQL for SCIM.
func buildCreateSCIMIntegrationSQL(opts CreateSCIMIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = SCIM")
	b.SetString("SCIM_CLIENT", &opts.SCIMClient)
	b.SetString("RUN_AS_ROLE", &opts.RunAsRole)
	b.SetString("NETWORK_POLICY", opts.NetworkPolicy)
	b.SetBool("SYNC_PASSWORD", opts.SyncPassword)
	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	return b.String(), nil
}

// Create creates a SCIM security integration in Snowflake.
func (c *SCIMIntegrationClient) Create(ctx context.Context, opts CreateSCIMIntegrationOptions) error {
	stmt, err := buildCreateSCIMIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create SCIM integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating SCIM integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSCIMIntegrationStatements builds ALTER SECURITY INTEGRATION statements for SCIM.
func buildAlterSCIMIntegrationStatements(opts AlterSCIMIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)
	sc.String("NETWORK_POLICY", opts.NetworkPolicy)
	sc.Bool("SYNC_PASSWORD", opts.SyncPassword)
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a SCIM security integration in Snowflake.
func (c *SCIMIntegrationClient) Alter(ctx context.Context, opts AlterSCIMIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter SCIM integration options: %w", err))
	}

	stmts, err := buildAlterSCIMIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter SCIM integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering SCIM integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a SCIM security integration.
func (c *SCIMIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping SCIM integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a SCIM integration from Snowflake.
func (c *SCIMIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.SCIMIntegrationShowOutput, error) {
	stmt := sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing SCIM integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanSCIMIntegrationShowOutput(rows, name.Name())
}

// scanSCIMIntegrationShowOutput scans SHOW output rows into SCIMIntegrationShowOutput.
func scanSCIMIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.SCIMIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SCIMIntegrationShowOutput, error) {
		return &v1alpha1.SCIMIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed SCIM integration properties.
func (c *SCIMIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing SCIM integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *SCIMIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SCIMIntegrationObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &SCIMIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SCIMIntegrationObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
