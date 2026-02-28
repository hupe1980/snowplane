package snowflake

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findIndex returns the index of substr in s, or -1 if not found.
func findIndex(s, substr string) int {
	return strings.Index(s, substr)
}

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateNotificationIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("EmailBasic", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := CreateNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_EMAIL_NI"),
			Type:              "EMAIL",
			Enabled:           &enabled,
			AllowedRecipients: []string{"admin@example.com", "ops@example.com"},
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, `CREATE NOTIFICATION INTEGRATION IF NOT EXISTS "MY_EMAIL_NI"`)
		assert.Contains(t, got, "TYPE = EMAIL")
		assert.Contains(t, got, "ALLOWED_RECIPIENTS = ('admin@example.com', 'ops@example.com')")
		assert.Contains(t, got, "ENABLED = TRUE")
	})

	t.Run("EmailWithDefaults", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_EMAIL"),
			Type:              "EMAIL",
			AllowedRecipients: []string{"admin@example.com"},
			DefaultRecipients: []string{"default@example.com"},
			DefaultSubject:    strPtr("Alert Notification"),
			Comment:           strPtr("test email integration"),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "ALLOWED_RECIPIENTS = ('admin@example.com')")
		assert.Contains(t, got, "DEFAULT_RECIPIENTS = ('default@example.com')")
		assert.Contains(t, got, "DEFAULT_SUBJECT = 'Alert Notification'")
		assert.Contains(t, got, "COMMENT = 'test email integration'")
	})

	t.Run("QueueAWSSNS", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_SNS"),
			Type:                 "QUEUE",
			NotificationProvider: strPtr("AWS_SNS"),
			Direction:            strPtr("OUTBOUND"),
			AWSSNSTopicARN:       strPtr("arn:aws:sns:us-east-1:123456789012:my-topic"),
			AWSSNSRoleARN:        strPtr("arn:aws:iam::123456789012:role/sns-role"),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "TYPE = QUEUE")
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = AWS_SNS")
		assert.Contains(t, got, "DIRECTION = OUTBOUND")
		assert.Contains(t, got, "AWS_SNS_TOPIC_ARN = 'arn:aws:sns:us-east-1:123456789012:my-topic'")
		assert.Contains(t, got, "AWS_SNS_ROLE_ARN = 'arn:aws:iam::123456789012:role/sns-role'")
	})

	t.Run("QueueGCPPubSub", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:                      NewAccountObjectIdentifier("MY_PUBSUB"),
			Type:                      "QUEUE",
			NotificationProvider:      strPtr("GCP_PUBSUB"),
			Direction:                 strPtr("OUTBOUND"),
			GCPPubSubTopicName:        strPtr("projects/myproj/topics/mytopic"),
			GCPPubSubSubscriptionName: strPtr("projects/myproj/subscriptions/mysub"),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = GCP_PUBSUB")
		assert.Contains(t, got, "GCP_PUBSUB_TOPIC_NAME = 'projects/myproj/topics/mytopic'")
		assert.Contains(t, got, "GCP_PUBSUB_SUBSCRIPTION_NAME = 'projects/myproj/subscriptions/mysub'")
	})

	t.Run("QueueAzure", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:                        NewAccountObjectIdentifier("MY_AZURE"),
			Type:                        "QUEUE",
			NotificationProvider:        strPtr("AZURE_STORAGE_QUEUE"),
			Direction:                   strPtr("OUTBOUND"),
			AzureStorageQueuePrimaryURI: strPtr("https://myaccount.queue.core.windows.net/myqueue"),
			AzureTenantID:               strPtr("tenant-id-123"),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = AZURE_STORAGE_QUEUE")
		assert.Contains(t, got, "AZURE_STORAGE_QUEUE_PRIMARY_URI = 'https://myaccount.queue.core.windows.net/myqueue'")
		assert.Contains(t, got, "AZURE_TENANT_ID = 'tenant-id-123'")
	})

	t.Run("WebhookBasic", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:       NewAccountObjectIdentifier("MY_WEBHOOK"),
			Type:       "WEBHOOK",
			WebhookURL: strPtr("https://hooks.example.com/alert"),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "TYPE = WEBHOOK")
		assert.Contains(t, got, "WEBHOOK_URL = 'https://hooks.example.com/alert'")
	})

	t.Run("WebhookWithSecret", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:                NewAccountObjectIdentifier("MY_WEBHOOK"),
			Type:                "WEBHOOK",
			WebhookURL:          strPtr("https://hooks.example.com/alert"),
			WebhookSecret:       strPtr("my-secret"),
			WebhookBodyTemplate: strPtr(`{"text": "$BODY"}`),
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		assert.Contains(t, got, "WEBHOOK_SECRET = 'my-secret'")
		assert.Contains(t, got, `WEBHOOK_BODY_TEMPLATE = '{"text": "$BODY"}'`)
	})

	t.Run("WebhookHeadersSorted", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:       NewAccountObjectIdentifier("MY_WEBHOOK"),
			Type:       "WEBHOOK",
			WebhookURL: strPtr("https://hooks.example.com"),
			WebhookHeaders: map[string]string{
				"X-Zebra": "z",
				"X-Alpha": "a",
			},
		}
		got := buildCreateNotificationIntegrationSQL(opts)
		// Headers must be sorted by key for deterministic SQL.
		alphaIdx := findIndex(got, "WEBHOOK_HEADER_X-Alpha")
		zebraIdx := findIndex(got, "WEBHOOK_HEADER_X-Zebra")
		assert.Greater(t, zebraIdx, alphaIdx, "headers should be sorted alphabetically")
		assert.Contains(t, got, "WEBHOOK_HEADER_X-Alpha = 'a'")
		assert.Contains(t, got, "WEBHOOK_HEADER_X-Zebra = 'z'")
	})
}

func TestBuildAlterNotificationIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := AlterNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_NI"),
			Type:    "EMAIL",
			Enabled: &enabled,
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ENABLED = TRUE")
	})

	t.Run("SetEmailAllowedRecipients", func(t *testing.T) {
		t.Parallel()
		list := []string{"a@b.com", "c@d.com"}
		opts := AlterNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_NI"),
			Type:              "EMAIL",
			AllowedRecipients: &list,
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALLOWED_RECIPIENTS = ('a@b.com', 'c@d.com')")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_NI"),
			Type:    "EMAIL",
			Comment: strPtr("updated comment"),
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_NI"),
			Type:        "EMAIL",
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("MY_NI"),
			Type: "EMAIL",
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Empty(t, stmts)
	})

	t.Run("SetWebhookHeaders", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_NI"),
			Type:           "WEBHOOK",
			WebhookURL:     strPtr("https://example.com/hook"),
			WebhookHeaders: map[string]string{"Authorization": "Bearer x", "Content-Type": "application/json"},
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "WEBHOOK_URL = 'https://example.com/hook'")
		// Headers should be sorted by key.
		authIdx := findIndex(stmts[0], "WEBHOOK_HEADER_Authorization")
		ctIdx := findIndex(stmts[0], "WEBHOOK_HEADER_Content-Type")
		assert.Greater(t, ctIdx, authIdx, "headers should be sorted alphabetically")
		assert.Contains(t, stmts[0], "WEBHOOK_HEADER_Authorization = 'Bearer x'")
		assert.Contains(t, stmts[0], "WEBHOOK_HEADER_Content-Type = 'application/json'")
	})

	t.Run("SetQueueARN", func(t *testing.T) {
		t.Parallel()
		provider := "AWS_SNS"
		arn := "arn:aws:sns:us-east-1:123456789:my-topic"
		role := "arn:aws:iam::123456789:role/my-role"
		opts := AlterNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_NI"),
			Type:                 "QUEUE",
			NotificationProvider: &provider,
			AWSSNSTopicARN:       &arn,
			AWSSNSRoleARN:        &role,
		}
		stmts, err := buildAlterNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "NOTIFICATION_PROVIDER = AWS_SNS")
		assert.Contains(t, stmts[0], "AWS_SNS_TOPIC_ARN = '"+arn+"'")
		assert.Contains(t, stmts[0], "AWS_SNS_ROLE_ARN = '"+role+"'")
	})
}

func TestCreateNotificationIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_EMAIL", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("NI"),
			Type:              "EMAIL",
			AllowedRecipients: []string{"a@b.com"},
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("Invalid_NoName", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Type:              "EMAIL",
			AllowedRecipients: []string{"a@b.com"},
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("Invalid_NoType", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("Invalid_BadType", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
			Type: "INVALID",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("Invalid_EMAIL_NoRecipients", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
			Type: "EMAIL",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("Invalid_QUEUE_NoProvider", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
			Type: "QUEUE",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("Invalid_WEBHOOK_NoURL", func(t *testing.T) {
		t.Parallel()
		opts := CreateNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
			Type: "WEBHOOK",
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterNotificationIntegrationOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("NI"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := AlterNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("NI"),
			Enabled: &enabled,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("Comment", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("NI"),
			Comment: strPtr("c"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("NI"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WebhookHeaders", func(t *testing.T) {
		t.Parallel()
		opts := AlterNotificationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("NI"),
			WebhookHeaders: map[string]string{"X-Custom": "val"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("NotificationProvider", func(t *testing.T) {
		t.Parallel()
		provider := "AWS_SNS"
		opts := AlterNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("NI"),
			NotificationProvider: &provider,
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestBuildShowNotificationIntegrationByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowNotificationIntegrationByIDSQL(NewAccountObjectIdentifier("MY_NI"))
	assert.Contains(t, got, "SHOW NOTIFICATION INTEGRATIONS")
	assert.Contains(t, got, "MY\\_NI")
}
