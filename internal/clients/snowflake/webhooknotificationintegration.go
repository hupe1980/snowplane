package snowflake

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// WebhookNotificationIntegrationObservation holds the result of observing a webhook notification integration.
type WebhookNotificationIntegrationObservation struct {
	Exists         bool
	ShowOutput     *v1alpha1.WebhookNotificationIntegrationShowOutput
	DescribeOutput map[string]string
}

// CreateWebhookNotificationIntegrationOptions holds the parameters for creating a webhook notification integration.
type CreateWebhookNotificationIntegrationOptions struct {
	Name                AccountObjectIdentifier
	Enabled             *bool
	WebhookURL          string
	WebhookSecret       *string
	WebhookBodyTemplate *string
	WebhookHeaders      map[string]string
	Comment             *string
}

// Validate checks that required fields are populated.
func (o *CreateWebhookNotificationIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("notification integration name is required"))
	}

	if o.WebhookURL == "" {
		errs = append(errs, fmt.Errorf("webhook_url is required for WEBHOOK"))
	}

	return errors.Join(errs...)
}

// AlterWebhookNotificationIntegrationOptions holds the parameters for altering a webhook notification integration.
type AlterWebhookNotificationIntegrationOptions struct {
	Name                AccountObjectIdentifier
	Enabled             *bool
	WebhookURL          *string
	WebhookSecret       *string
	WebhookBodyTemplate *string
	WebhookHeaders      map[string]string
	Comment             *string
	UnsetFields         []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterWebhookNotificationIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.WebhookURL != nil ||
		o.WebhookSecret != nil ||
		o.WebhookBodyTemplate != nil ||
		len(o.WebhookHeaders) > 0 ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterWebhookNotificationIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("notification integration name is required")
	}

	return nil
}

// WebhookNotificationIntegrationClient provides operations on Snowflake webhook notification integrations.
type WebhookNotificationIntegrationClient struct {
	client SQLExecutor
}

// NewWebhookNotificationIntegrationClient creates a new WebhookNotificationIntegrationClient.
func NewWebhookNotificationIntegrationClient(c SQLExecutor) *WebhookNotificationIntegrationClient {
	return &WebhookNotificationIntegrationClient{client: c}
}

// buildCreateWebhookNotificationIntegrationSQL builds the CREATE SQL for a webhook notification integration.
func buildCreateWebhookNotificationIntegrationSQL(opts CreateWebhookNotificationIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = WEBHOOK")

	url := opts.WebhookURL
	b.SetString("WEBHOOK_URL", &url)
	b.SetString("WEBHOOK_SECRET", opts.WebhookSecret)
	b.SetString("WEBHOOK_BODY_TEMPLATE", opts.WebhookBodyTemplate)
	setWebhookHeaders(&b, opts.WebhookHeaders)
	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	return b.String(), nil
}

// Create creates a webhook notification integration in Snowflake.
func (c *WebhookNotificationIntegrationClient) Create(ctx context.Context, opts CreateWebhookNotificationIntegrationOptions) error {
	stmt, err := buildCreateWebhookNotificationIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create webhook notification integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating webhook notification integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterWebhookNotificationIntegrationStatements builds ALTER statements for a webhook notification integration.
func buildAlterWebhookNotificationIntegrationStatements(opts AlterWebhookNotificationIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)
	sc.String("WEBHOOK_URL", opts.WebhookURL)
	sc.String("WEBHOOK_SECRET", opts.WebhookSecret)
	sc.String("WEBHOOK_BODY_TEMPLATE", opts.WebhookBodyTemplate)

	if len(opts.WebhookHeaders) > 0 {
		keys := make([]string, 0, len(opts.WebhookHeaders))
		for k := range opts.WebhookHeaders {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			v := opts.WebhookHeaders[k]
			sc.String("WEBHOOK_HEADER_"+k, &v)
		}
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("NOTIFICATION INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a webhook notification integration in Snowflake.
func (c *WebhookNotificationIntegrationClient) Alter(ctx context.Context, opts AlterWebhookNotificationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter webhook notification integration options: %w", err))
	}

	stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter webhook notification integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering webhook notification integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a webhook notification integration.
func (c *WebhookNotificationIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping webhook notification integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a webhook notification integration from Snowflake.
func (c *WebhookNotificationIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.WebhookNotificationIntegrationShowOutput, error) {
	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("NOTIFICATION INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing webhook notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return ScanShowOutput(rows, name.Name(), func(m map[string]string) (*v1alpha1.WebhookNotificationIntegrationShowOutput, error) {
		return &v1alpha1.WebhookNotificationIntegrationShowOutput{
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
func (c *WebhookNotificationIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing webhook notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *WebhookNotificationIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*WebhookNotificationIntegrationObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &WebhookNotificationIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		return &WebhookNotificationIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &WebhookNotificationIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}
