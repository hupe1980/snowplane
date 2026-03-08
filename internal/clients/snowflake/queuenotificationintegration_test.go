package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests – Queue Notification Integration
// --------------------------------------------------------------------------

func TestBuildCreateQueueNotificationIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("AWSSNS", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_QUEUE_INT"),
			NotificationProvider: "AWS_SNS",
			Direction:            "OUTBOUND",
			AWSSNSTopicARN:       ptr("arn:aws:sns:us-east-1:123456789:my-topic"),
			AWSSNSRoleARN:        ptr("arn:aws:iam::123456789:role/my-role"),
		}
		got, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE NOTIFICATION INTEGRATION IF NOT EXISTS "MY_QUEUE_INT"`)
		assert.Contains(t, got, "TYPE = QUEUE")
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = AWS_SNS")
		assert.Contains(t, got, "DIRECTION = OUTBOUND")
		assert.Contains(t, got, "AWS_SNS_TOPIC_ARN = 'arn:aws:sns:us-east-1:123456789:my-topic'")
		assert.Contains(t, got, "AWS_SNS_ROLE_ARN = 'arn:aws:iam::123456789:role/my-role'")
	})

	t.Run("GCPPubSub", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("GCP_INT"),
			NotificationProvider: "GCP_PUBSUB",
			GCPPubSubTopicName:   ptr("projects/myproject/topics/mytopic"),
		}
		got, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = GCP_PUBSUB")
		assert.Contains(t, got, "GCP_PUBSUB_TOPIC_NAME = 'projects/myproject/topics/mytopic'")
	})

	t.Run("AzureStorageQueue", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			Name:                        NewAccountObjectIdentifier("AZURE_INT"),
			NotificationProvider:        "AZURE_STORAGE_QUEUE",
			AzureStorageQueuePrimaryURI: ptr("https://myaccount.queue.core.windows.net/myqueue"),
			AzureTenantID:               ptr("00000000-0000-0000-0000-000000000000"),
		}
		got, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = AZURE_STORAGE_QUEUE")
		assert.Contains(t, got, "AZURE_STORAGE_QUEUE_PRIMARY_URI = 'https://myaccount.queue.core.windows.net/myqueue'")
		assert.Contains(t, got, "AZURE_TENANT_ID = '00000000-0000-0000-0000-000000000000'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("FULL_QUEUE"),
			Enabled:              ptr(false),
			NotificationProvider: "AWS_SQS",
			Direction:            "INBOUND",
			AWSSQSArn:            ptr("arn:aws:sqs:us-east-1:123456789:my-queue"),
			AWSSQSRoleARN:        ptr("arn:aws:iam::123456789:role/sqs-role"),
			Comment:              ptr("queue int"),
		}
		got, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "NOTIFICATION_PROVIDER = AWS_SQS")
		assert.Contains(t, got, "DIRECTION = INBOUND")
		assert.Contains(t, got, "ENABLED = FALSE")
		assert.Contains(t, got, "COMMENT = 'queue int'")
	})
}

func TestBuildCreateQueueNotificationIntegrationSQL_Validation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			NotificationProvider: "AWS_SNS",
		}
		_, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingProvider", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("TEST"),
		}
		_, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notification_provider is required")
	})

	t.Run("MultipleErrors", func(t *testing.T) {
		t.Parallel()
		opts := CreateQueueNotificationIntegrationOptions{}
		_, err := buildCreateQueueNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		assert.Contains(t, err.Error(), "notification_provider is required")
	})
}

func TestBuildAlterQueueNotificationIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterQueueNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_QUEUE"),
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterQueueNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("SetProviderAndDirection", func(t *testing.T) {
		t.Parallel()
		opts := AlterQueueNotificationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_QUEUE"),
			NotificationProvider: ptr("AWS_SNS"),
			Direction:            ptr("OUTBOUND"),
		}
		stmts, err := buildAlterQueueNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "NOTIFICATION_PROVIDER = AWS_SNS")
		assert.Contains(t, stmts[0], "DIRECTION = OUTBOUND")
	})

	t.Run("SetAWSFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterQueueNotificationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_QUEUE"),
			AWSSNSTopicARN: ptr("arn:aws:sns:us-east-1:123456789:my-topic"),
			AWSSNSRoleARN:  ptr("arn:aws:iam::123456789:role/my-role"),
		}
		stmts, err := buildAlterQueueNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "AWS_SNS_TOPIC_ARN")
		assert.Contains(t, stmts[0], "AWS_SNS_ROLE_ARN")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterQueueNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_QUEUE"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterQueueNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET")
		assert.Contains(t, stmts[0], "COMMENT")
	})
}

func TestQueueAlterHasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts AlterQueueNotificationIntegrationOptions
		want bool
	}{
		{"Empty", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T")}, false},
		{"Enabled", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Enabled: ptr(true)}, true},
		{"Comment", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Comment: ptr("c")}, true},
		{"Provider", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), NotificationProvider: ptr("AWS_SNS")}, true},
		{"Direction", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Direction: ptr("OUTBOUND")}, true},
		{"AWSSNS", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), AWSSNSTopicARN: ptr("arn")}, true},
		{"GCP", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), GCPPubSubTopicName: ptr("topic")}, true},
		{"Azure", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), AzureTenantID: ptr("id")}, true},
		{"UnsetFields", AlterQueueNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), UnsetFields: []string{"COMMENT"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.opts.HasChanges())
		})
	}
}
