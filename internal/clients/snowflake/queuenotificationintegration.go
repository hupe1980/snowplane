package snowflake

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// QueueNotificationIntegrationObservation holds the result of observing a queue notification integration.
type QueueNotificationIntegrationObservation struct {
	Exists         bool
	ShowOutput     *v1alpha1.QueueNotificationIntegrationShowOutput
	DescribeOutput map[string]string
}

// CreateQueueNotificationIntegrationOptions holds the parameters for creating a queue notification integration.
type CreateQueueNotificationIntegrationOptions struct {
	Name                        AccountObjectIdentifier
	Enabled                     *bool
	NotificationProvider        string
	Direction                   string
	AWSSNSTopicARN              *string
	AWSSNSRoleARN               *string
	AWSSQSArn                   *string
	AWSSQSRoleARN               *string
	GCPPubSubTopicName          *string
	GCPPubSubSubscriptionName   *string
	AzureStorageQueuePrimaryURI *string
	AzureTenantID               *string
	AzureEventGridTopicEndpoint *string
	Comment                     *string
}

// Validate checks that required fields are populated.
func (o *CreateQueueNotificationIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("notification integration name is required"))
	}

	if o.NotificationProvider == "" {
		errs = append(errs, fmt.Errorf("notification_provider is required for QUEUE"))
	}

	return errors.Join(errs...)
}

// AlterQueueNotificationIntegrationOptions holds the parameters for altering a queue notification integration.
type AlterQueueNotificationIntegrationOptions struct {
	Name                        AccountObjectIdentifier
	Enabled                     *bool
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
	Comment                     *string
	UnsetFields                 []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterQueueNotificationIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
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
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterQueueNotificationIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("notification integration name is required")
	}

	return nil
}

// QueueNotificationIntegrationClient provides operations on Snowflake queue notification integrations.
type QueueNotificationIntegrationClient struct {
	client SQLExecutor
}

// NewQueueNotificationIntegrationClient creates a new QueueNotificationIntegrationClient.
func NewQueueNotificationIntegrationClient(c SQLExecutor) *QueueNotificationIntegrationClient {
	return &QueueNotificationIntegrationClient{client: c}
}

// buildCreateQueueNotificationIntegrationSQL builds the CREATE SQL for a queue notification integration.
func buildCreateQueueNotificationIntegrationSQL(opts CreateQueueNotificationIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = QUEUE")
	fmt.Fprintf(&b.Builder, " NOTIFICATION_PROVIDER = %s", opts.NotificationProvider)

	if opts.Direction != "" {
		fmt.Fprintf(&b.Builder, " DIRECTION = %s", opts.Direction)
	}

	b.SetString("AWS_SNS_TOPIC_ARN", opts.AWSSNSTopicARN)
	b.SetString("AWS_SNS_ROLE_ARN", opts.AWSSNSRoleARN)
	b.SetString("AWS_SQS_ARN", opts.AWSSQSArn)
	b.SetString("AWS_SQS_ROLE_ARN", opts.AWSSQSRoleARN)
	b.SetString("GCP_PUBSUB_TOPIC_NAME", opts.GCPPubSubTopicName)
	b.SetString("GCP_PUBSUB_SUBSCRIPTION_NAME", opts.GCPPubSubSubscriptionName)
	b.SetString("AZURE_STORAGE_QUEUE_PRIMARY_URI", opts.AzureStorageQueuePrimaryURI)
	b.SetString("AZURE_TENANT_ID", opts.AzureTenantID)
	b.SetString("AZURE_EVENT_GRID_TOPIC_ENDPOINT", opts.AzureEventGridTopicEndpoint)
	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	return b.String(), nil
}

// Create creates a queue notification integration in Snowflake.
func (c *QueueNotificationIntegrationClient) Create(ctx context.Context, opts CreateQueueNotificationIntegrationOptions) error {
	stmt, err := buildCreateQueueNotificationIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create queue notification integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating queue notification integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterQueueNotificationIntegrationStatements builds ALTER statements for a queue notification integration.
func buildAlterQueueNotificationIntegrationStatements(opts AlterQueueNotificationIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.NotificationProvider != nil {
		sc.UnsafeRaw(fmt.Sprintf("NOTIFICATION_PROVIDER = %s", *opts.NotificationProvider)) //nolint:forbidigo // Snowflake keyword validated by CRD enum
	}

	if opts.Direction != nil {
		sc.UnsafeRaw(fmt.Sprintf("DIRECTION = %s", *opts.Direction)) //nolint:forbidigo // Snowflake keyword validated by CRD enum
	}

	sc.String("AWS_SNS_TOPIC_ARN", opts.AWSSNSTopicARN)
	sc.String("AWS_SNS_ROLE_ARN", opts.AWSSNSRoleARN)
	sc.String("AWS_SQS_ARN", opts.AWSSQSArn)
	sc.String("AWS_SQS_ROLE_ARN", opts.AWSSQSRoleARN)
	sc.String("GCP_PUBSUB_TOPIC_NAME", opts.GCPPubSubTopicName)
	sc.String("GCP_PUBSUB_SUBSCRIPTION_NAME", opts.GCPPubSubSubscriptionName)
	sc.String("AZURE_STORAGE_QUEUE_PRIMARY_URI", opts.AzureStorageQueuePrimaryURI)
	sc.String("AZURE_TENANT_ID", opts.AzureTenantID)
	sc.String("AZURE_EVENT_GRID_TOPIC_ENDPOINT", opts.AzureEventGridTopicEndpoint)
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("NOTIFICATION INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a queue notification integration in Snowflake.
func (c *QueueNotificationIntegrationClient) Alter(ctx context.Context, opts AlterQueueNotificationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter queue notification integration options: %w", err))
	}

	stmts, err := buildAlterQueueNotificationIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter queue notification integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering queue notification integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a queue notification integration.
func (c *QueueNotificationIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("NOTIFICATION INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping queue notification integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a queue notification integration from Snowflake.
func (c *QueueNotificationIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.QueueNotificationIntegrationShowOutput, error) {
	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("NOTIFICATION INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing queue notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return ScanShowOutput(rows, name.Name(), func(m map[string]string) (*v1alpha1.QueueNotificationIntegrationShowOutput, error) {
		return &v1alpha1.QueueNotificationIntegrationShowOutput{
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
func (c *QueueNotificationIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing queue notification integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *QueueNotificationIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*QueueNotificationIntegrationObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &QueueNotificationIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		return &QueueNotificationIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &QueueNotificationIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}
