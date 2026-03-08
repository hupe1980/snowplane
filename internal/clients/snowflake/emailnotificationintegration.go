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

// EmailNotificationIntegrationObservation holds the result of observing an email notification integration.
type EmailNotificationIntegrationObservation struct {
	Exists         bool
	ShowOutput     *v1alpha1.EmailNotificationIntegrationShowOutput
	DescribeOutput map[string]string
}

// CreateEmailNotificationIntegrationOptions holds the parameters for creating an email notification integration.
type CreateEmailNotificationIntegrationOptions struct {
	Name              AccountObjectIdentifier
	Enabled           *bool
	AllowedRecipients []string
	DefaultRecipients []string
	DefaultSubject    *string
	Comment           *string
}

// Validate checks that required fields are populated.
func (o *CreateEmailNotificationIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("notification integration name is required"))
	}

	if len(o.AllowedRecipients) == 0 {
		errs = append(errs, fmt.Errorf("allowed_recipients is required for EMAIL"))
	}

	return errors.Join(errs...)
}

// AlterEmailNotificationIntegrationOptions holds the parameters for altering an email notification integration.
type AlterEmailNotificationIntegrationOptions struct {
	Name              AccountObjectIdentifier
	Enabled           *bool
	AllowedRecipients *[]string
	DefaultRecipients *[]string
	DefaultSubject    *string
	Comment           *string
	UnsetFields       []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterEmailNotificationIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.AllowedRecipients != nil ||
		o.DefaultRecipients != nil ||
		o.DefaultSubject != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterEmailNotificationIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("notification integration name is required")
	}

	return nil
}

// EmailNotificationIntegrationClient provides operations on Snowflake email notification integrations.
type EmailNotificationIntegrationClient struct {
	client SQLExecutor
}

// NewEmailNotificationIntegrationClient creates a new EmailNotificationIntegrationClient.
func NewEmailNotificationIntegrationClient(c SQLExecutor) *EmailNotificationIntegrationClient {
	return &EmailNotificationIntegrationClient{client: c}
}

// buildCreateEmailNotificationIntegrationSQL builds the CREATE SQL for an email notification integration.
func buildCreateEmailNotificationIntegrationSQL(opts CreateEmailNotificationIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = EMAIL")

	if len(opts.AllowedRecipients) > 0 {
		b.WriteString(" ")
		b.WriteString(buildEmailListClause("ALLOWED_RECIPIENTS", opts.AllowedRecipients))
	}

	if len(opts.DefaultRecipients) > 0 {
		b.WriteString(" ")
		b.WriteString(buildEmailListClause("DEFAULT_RECIPIENTS", opts.DefaultRecipients))
	}

	b.SetString("DEFAULT_SUBJECT", opts.DefaultSubject)
	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	return b.String(), nil
}

// Create creates an email notification integration in Snowflake.
func (c *EmailNotificationIntegrationClient) Create(ctx context.Context, opts CreateEmailNotificationIntegrationOptions) error {
	stmt, err := buildCreateEmailNotificationIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create email notification integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating email notification integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterEmailNotificationIntegrationStatements builds ALTER statements for an email notification integration.
func buildAlterEmailNotificationIntegrationStatements(opts AlterEmailNotificationIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.AllowedRecipients != nil {
		sc.UnsafeRaw(buildEmailListClause("ALLOWED_RECIPIENTS", *opts.AllowedRecipients)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.DefaultRecipients != nil {
		sc.UnsafeRaw(buildEmailListClause("DEFAULT_RECIPIENTS", *opts.DefaultRecipients)) //nolint:forbidigo // values escaped via EscapeString
	}

	sc.String("DEFAULT_SUBJECT", opts.DefaultSubject)
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("NOTIFICATION INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an email notification integration in Snowflake.
func (c *EmailNotificationIntegrationClient) Alter(ctx context.Context, opts AlterEmailNotificationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter email notification integration options: %w", err))
	}

	stmts, err := buildAlterEmailNotificationIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter email notification integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering email notification integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an email notification integration.
func (c *EmailNotificationIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping email notification integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves an email notification integration from Snowflake.
func (c *EmailNotificationIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.EmailNotificationIntegrationShowOutput, error) {
	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("NOTIFICATION INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing email notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanEmailNotificationIntegrationShowOutput(rows, name.Name())
}

func scanEmailNotificationIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.EmailNotificationIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.EmailNotificationIntegrationShowOutput, error) {
		return &v1alpha1.EmailNotificationIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed integration properties.
func (c *EmailNotificationIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing email notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *EmailNotificationIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*EmailNotificationIntegrationObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &EmailNotificationIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		return &EmailNotificationIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &EmailNotificationIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}
