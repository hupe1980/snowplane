package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// NotificationIntegrationObservation holds the result of observing a Snowflake notification integration.
type NotificationIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW NOTIFICATION INTEGRATIONS row.
	ShowOutput *NotificationIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// NotificationIntegrationShowOutput contains the fields from SHOW NOTIFICATION INTEGRATIONS.
type NotificationIntegrationShowOutput struct {
	CreatedOn string
	Name      string
	Type      string
	Category  string
	Enabled   bool
	Comment   string
}

// CreateNotificationIntegrationOptions holds the parameters for creating a notification integration.
type CreateNotificationIntegrationOptions struct {
	Name    AccountObjectIdentifier
	Type    string // EMAIL, QUEUE, WEBHOOK
	Enabled *bool

	// EMAIL config.
	AllowedRecipients []string
	DefaultRecipients []string
	DefaultSubject    *string

	// QUEUE config.
	NotificationProvider        *string
	Direction                   *string
	AWSSNSTopicARN              *string
	AWSSNSRoleARN               *string
	AWSSQSArn                   *string
	AWSSQSRoleARN               *string
	GCPPubSubTopicName          *string
	GCPPubSubSubscriptionName   *string
	AzureStorageQueuePrimaryURI *string
	AzureTenantID               *string
	AzureEventGridTopicEndpoint *string

	// WEBHOOK config.
	WebhookURL          *string
	WebhookSecret       *string
	WebhookBodyTemplate *string
	WebhookHeaders      map[string]string

	Comment *string
}

// validNotificationIntegrationTypes is the allowlist of notification integration types.
var validNotificationIntegrationTypes = map[string]bool{
	"EMAIL":   true,
	"QUEUE":   true,
	"WEBHOOK": true,
}

// Validate checks the CreateNotificationIntegrationOptions for validity.
func (o *CreateNotificationIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("notification integration name is required"))
	}

	if o.Type == "" {
		errs = append(errs, fmt.Errorf("notification integration type is required"))
	} else if !validNotificationIntegrationTypes[o.Type] {
		errs = append(errs, fmt.Errorf("invalid notification integration type %q", o.Type))
	}

	switch o.Type {
	case "EMAIL":
		if len(o.AllowedRecipients) == 0 {
			errs = append(errs, fmt.Errorf("allowed_recipients is required for EMAIL"))
		}
	case "QUEUE":
		if o.NotificationProvider == nil || *o.NotificationProvider == "" {
			errs = append(errs, fmt.Errorf("notification_provider is required for QUEUE"))
		}
	case "WEBHOOK":
		if o.WebhookURL == nil || *o.WebhookURL == "" {
			errs = append(errs, fmt.Errorf("webhook_url is required for WEBHOOK"))
		}
	}

	return errors.Join(errs...)
}

// AlterNotificationIntegrationOptions holds the parameters for altering a notification integration.
type AlterNotificationIntegrationOptions struct {
	Name    AccountObjectIdentifier
	Type    string
	Enabled *bool

	// EMAIL config (alterable fields).
	AllowedRecipients *[]string
	DefaultRecipients *[]string
	DefaultSubject    *string

	// WEBHOOK config (alterable fields).
	WebhookURL          *string
	WebhookSecret       *string
	WebhookBodyTemplate *string
	WebhookHeaders      map[string]string

	// QUEUE config (alterable fields).
	NotificationProvider        *string
	Direction                   *string
	AWSSNSTopicARN              *string
	AWSSNSRoleARN               *string
	AWSSQSArn                   *string
	AWSSQSRoleARN               *string
	GCPPubSubTopicName          *string
	GCPPubSubSubscriptionName   *string
	AzureStorageQueuePrimaryURI *string
	AzureTenantID               *string
	AzureEventGridTopicEndpoint *string

	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterNotificationIntegrationOptions for validity.
func (o *AlterNotificationIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("notification integration name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterNotificationIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.Comment != nil ||
		o.AllowedRecipients != nil ||
		o.DefaultRecipients != nil ||
		o.DefaultSubject != nil ||
		o.WebhookURL != nil ||
		o.WebhookSecret != nil ||
		o.WebhookBodyTemplate != nil ||
		len(o.WebhookHeaders) > 0 ||
		o.NotificationProvider != nil ||
		o.Direction != nil ||
		o.AWSSNSTopicARN != nil ||
		o.AWSSNSRoleARN != nil ||
		o.AWSSQSArn != nil ||
		o.AWSSQSRoleARN != nil ||
		o.GCPPubSubTopicName != nil ||
		o.GCPPubSubSubscriptionName != nil ||
		o.AzureStorageQueuePrimaryURI != nil ||
		o.AzureTenantID != nil ||
		o.AzureEventGridTopicEndpoint != nil ||
		len(o.UnsetFields) > 0
}

// NotificationIntegrationClient provides operations against Snowflake notification integrations.
type NotificationIntegrationClient struct {
	client SQLExecutor
}

// NewNotificationIntegrationClient creates a new NotificationIntegrationClient.
func NewNotificationIntegrationClient(c SQLExecutor) *NotificationIntegrationClient {
	return &NotificationIntegrationClient{client: c}
}

// buildCreateNotificationIntegrationSQL builds the CREATE NOTIFICATION INTEGRATION SQL statement.
func buildCreateNotificationIntegrationSQL(opts CreateNotificationIntegrationOptions) string {
	var b sqlbuilder.Builder

	b.WriteString("CREATE NOTIFICATION INTEGRATION IF NOT EXISTS ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.Name.Name()))
	fmt.Fprintf(&b.Builder, " TYPE = %s", opts.Type)

	switch opts.Type {
	case "EMAIL":
		if len(opts.AllowedRecipients) > 0 {
			b.WriteString(" ")
			b.WriteString(buildEmailListClause("ALLOWED_RECIPIENTS", opts.AllowedRecipients))
		}

		if len(opts.DefaultRecipients) > 0 {
			b.WriteString(" ")
			b.WriteString(buildEmailListClause("DEFAULT_RECIPIENTS", opts.DefaultRecipients))
		}

		b.SetString("DEFAULT_SUBJECT", opts.DefaultSubject)

	case "QUEUE":
		if opts.NotificationProvider != nil {
			fmt.Fprintf(&b.Builder, " NOTIFICATION_PROVIDER = %s", *opts.NotificationProvider)
		}

		if opts.Direction != nil {
			fmt.Fprintf(&b.Builder, " DIRECTION = %s", *opts.Direction)
		}

		if opts.AWSSNSTopicARN != nil {
			b.SetString("AWS_SNS_TOPIC_ARN", opts.AWSSNSTopicARN)
		}

		if opts.AWSSNSRoleARN != nil {
			b.SetString("AWS_SNS_ROLE_ARN", opts.AWSSNSRoleARN)
		}

		if opts.AWSSQSArn != nil {
			b.SetString("AWS_SQS_ARN", opts.AWSSQSArn)
		}

		if opts.AWSSQSRoleARN != nil {
			b.SetString("AWS_SQS_ROLE_ARN", opts.AWSSQSRoleARN)
		}

		if opts.GCPPubSubTopicName != nil {
			b.SetString("GCP_PUBSUB_TOPIC_NAME", opts.GCPPubSubTopicName)
		}

		if opts.GCPPubSubSubscriptionName != nil {
			b.SetString("GCP_PUBSUB_SUBSCRIPTION_NAME", opts.GCPPubSubSubscriptionName)
		}

		if opts.AzureStorageQueuePrimaryURI != nil {
			b.SetString("AZURE_STORAGE_QUEUE_PRIMARY_URI", opts.AzureStorageQueuePrimaryURI)
		}

		if opts.AzureTenantID != nil {
			b.SetString("AZURE_TENANT_ID", opts.AzureTenantID)
		}

		if opts.AzureEventGridTopicEndpoint != nil {
			b.SetString("AZURE_EVENT_GRID_TOPIC_ENDPOINT", opts.AzureEventGridTopicEndpoint)
		}

	case "WEBHOOK":
		if opts.WebhookURL != nil {
			b.SetString("WEBHOOK_URL", opts.WebhookURL)
		}

		if opts.WebhookSecret != nil {
			b.SetString("WEBHOOK_SECRET", opts.WebhookSecret)
		}

		if opts.WebhookBodyTemplate != nil {
			b.SetString("WEBHOOK_BODY_TEMPLATE", opts.WebhookBodyTemplate)
		}

		setWebhookHeaders(&b, opts.WebhookHeaders)
	}

	if opts.Enabled != nil {
		b.SetBool("ENABLED", opts.Enabled)
	}

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// buildEmailListClause formats an email list for Snowflake SQL, e.g. ('a@b.com', 'c@d.com').
func buildEmailListClause(keyword string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
	}

	return fmt.Sprintf("%s = (%s)", keyword, strings.Join(quoted, ", "))
}

// setWebhookHeaders writes sorted WEBHOOK_HEADER_<key> clauses to the builder.
func setWebhookHeaders(b *sqlbuilder.Builder, headers map[string]string) {
	if len(headers) == 0 {
		return
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		v := headers[k]
		b.SetString("WEBHOOK_HEADER_"+k, &v)
	}
}

// Create creates a notification integration in Snowflake.
func (ni *NotificationIntegrationClient) Create(ctx context.Context, opts CreateNotificationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create notification integration options: %w", err))
	}

	if _, err := ni.client.Exec(ctx, buildCreateNotificationIntegrationSQL(opts)); err != nil {
		return fmt.Errorf("creating notification integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterNotificationIntegrationStatements builds ALTER NOTIFICATION INTEGRATION statements.
func buildAlterNotificationIntegrationStatements(opts AlterNotificationIntegrationOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.Enabled != nil {
		sc.Bool("ENABLED", opts.Enabled)
	}

	switch opts.Type {
	case "EMAIL":
		if opts.AllowedRecipients != nil {
			sc.UnsafeRaw(buildEmailListClause("ALLOWED_RECIPIENTS", *opts.AllowedRecipients))
		}

		if opts.DefaultRecipients != nil {
			sc.UnsafeRaw(buildEmailListClause("DEFAULT_RECIPIENTS", *opts.DefaultRecipients))
		}

		if opts.DefaultSubject != nil {
			sc.String("DEFAULT_SUBJECT", opts.DefaultSubject)
		}

	case "WEBHOOK":
		if opts.WebhookURL != nil {
			sc.String("WEBHOOK_URL", opts.WebhookURL)
		}

		if opts.WebhookSecret != nil {
			sc.String("WEBHOOK_SECRET", opts.WebhookSecret)
		}

		if opts.WebhookBodyTemplate != nil {
			sc.String("WEBHOOK_BODY_TEMPLATE", opts.WebhookBodyTemplate)
		}

		// Sorted header keys for deterministic SQL.
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

	case "QUEUE":
		if opts.NotificationProvider != nil {
			sc.UnsafeRaw(fmt.Sprintf("NOTIFICATION_PROVIDER = %s", *opts.NotificationProvider))
		}

		if opts.Direction != nil {
			sc.UnsafeRaw(fmt.Sprintf("DIRECTION = %s", *opts.Direction))
		}

		if opts.AWSSNSTopicARN != nil {
			sc.String("AWS_SNS_TOPIC_ARN", opts.AWSSNSTopicARN)
		}

		if opts.AWSSNSRoleARN != nil {
			sc.String("AWS_SNS_ROLE_ARN", opts.AWSSNSRoleARN)
		}

		if opts.AWSSQSArn != nil {
			sc.String("AWS_SQS_ARN", opts.AWSSQSArn)
		}

		if opts.AWSSQSRoleARN != nil {
			sc.String("AWS_SQS_ROLE_ARN", opts.AWSSQSRoleARN)
		}

		if opts.GCPPubSubTopicName != nil {
			sc.String("GCP_PUBSUB_TOPIC_NAME", opts.GCPPubSubTopicName)
		}

		if opts.GCPPubSubSubscriptionName != nil {
			sc.String("GCP_PUBSUB_SUBSCRIPTION_NAME", opts.GCPPubSubSubscriptionName)
		}

		if opts.AzureStorageQueuePrimaryURI != nil {
			sc.String("AZURE_STORAGE_QUEUE_PRIMARY_URI", opts.AzureStorageQueuePrimaryURI)
		}

		if opts.AzureTenantID != nil {
			sc.String("AZURE_TENANT_ID", opts.AzureTenantID)
		}

		if opts.AzureEventGridTopicEndpoint != nil {
			sc.String("AZURE_EVENT_GRID_TOPIC_ENDPOINT", opts.AzureEventGridTopicEndpoint)
		}
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("NOTIFICATION INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a notification integration in Snowflake.
func (ni *NotificationIntegrationClient) Alter(ctx context.Context, opts AlterNotificationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter notification integration options: %w", err))
	}

	stmts, err := buildAlterNotificationIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter notification integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := ni.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering notification integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a notification integration from Snowflake.
func (ni *NotificationIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("notification integration name is required"))
	}

	stmt := sqlbuilder.DropIfExists("NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := ni.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping notification integration %s: %w", name, err)
	}

	return nil
}

// buildShowNotificationIntegrationByIDSQL builds the SHOW SQL for a specific notification integration.
func buildShowNotificationIntegrationByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("NOTIFICATION INTEGRATIONS", name.Name())
}

// ShowByID queries SHOW NOTIFICATION INTEGRATIONS for a specific integration.
func (ni *NotificationIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*NotificationIntegrationShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("notification integration name is required"))
	}

	rows, err := ni.client.Query(ctx, buildShowNotificationIntegrationByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing notification integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanNotificationIntegrationShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs of properties.
func (ni *NotificationIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("notification integration name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := ni.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing notification integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a NotificationIntegrationObservation.
func (ni *NotificationIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*NotificationIntegrationObservation, error) {
	show, err := ni.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &NotificationIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := ni.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &NotificationIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &NotificationIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanNotificationIntegrationShowOutput scans SHOW NOTIFICATION INTEGRATIONS results.
func scanNotificationIntegrationShowOutput(rows *sql.Rows, name string) (*NotificationIntegrationShowOutput, error) {
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

		return &NotificationIntegrationShowOutput{
			CreatedOn: colMap["created_on"],
			Name:      colMap["name"],
			Type:      colMap["type"],
			Category:  colMap["category"],
			Enabled:   strings.EqualFold(colMap["enabled"], "true"),
			Comment:   colMap["comment"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
