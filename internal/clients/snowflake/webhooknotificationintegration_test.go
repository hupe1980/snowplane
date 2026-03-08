package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests – Webhook Notification Integration
// --------------------------------------------------------------------------

func TestBuildCreateWebhookNotificationIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{
			Name:       NewAccountObjectIdentifier("MY_WEBHOOK_INT"),
			WebhookURL: "https://hooks.example.com/endpoint",
		}
		got, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE NOTIFICATION INTEGRATION IF NOT EXISTS "MY_WEBHOOK_INT"`)
		assert.Contains(t, got, "TYPE = WEBHOOK")
		assert.Contains(t, got, "WEBHOOK_URL = 'https://hooks.example.com/endpoint'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{
			Name:                NewAccountObjectIdentifier("FULL_WEBHOOK"),
			Enabled:             ptr(true),
			WebhookURL:          "https://hooks.example.com/full",
			WebhookSecret:       ptr("my-secret-123"),
			WebhookBodyTemplate: ptr(`{"text": "Alert: SNOWFLAKE_WEBHOOK_MESSAGE"}`),
			WebhookHeaders: map[string]string{
				"Authorization": "Bearer token123",
				"Content-Type":  "application/json",
			},
			Comment: ptr("webhook integration"),
		}
		got, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = WEBHOOK")
		assert.Contains(t, got, "WEBHOOK_URL = 'https://hooks.example.com/full'")
		assert.Contains(t, got, "WEBHOOK_SECRET = 'my-secret-123'")
		assert.Contains(t, got, "WEBHOOK_BODY_TEMPLATE")
		assert.Contains(t, got, "WEBHOOK_HEADER_Authorization")
		assert.Contains(t, got, "WEBHOOK_HEADER_Content-Type")
		assert.Contains(t, got, "ENABLED = TRUE")
		assert.Contains(t, got, "COMMENT = 'webhook integration'")
	})

	t.Run("HeadersSorted", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{
			Name:       NewAccountObjectIdentifier("SORTED"),
			WebhookURL: "https://hooks.example.com/sorted",
			WebhookHeaders: map[string]string{
				"Z-Header": "z-val",
				"A-Header": "a-val",
			},
		}
		got, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		// A-Header should appear before Z-Header due to sorting.
		idxA := len(got) // fallback
		idxZ := len(got)
		for i := 0; i < len(got); i++ {
			if got[i:i+1] == "A" && i+8 < len(got) && got[i:i+8] == "A-Header" {
				idxA = i
			}
			if got[i:i+1] == "Z" && i+8 < len(got) && got[i:i+8] == "Z-Header" {
				idxZ = i
			}
		}
		assert.Less(t, idxA, idxZ, "A-Header should precede Z-Header")
	})
}

func TestBuildCreateWebhookNotificationIntegrationSQL_Validation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{
			WebhookURL: "https://hooks.example.com",
		}
		_, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingWebhookURL", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("TEST"),
		}
		_, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "webhook_url is required")
	})

	t.Run("MultipleErrors", func(t *testing.T) {
		t.Parallel()
		opts := CreateWebhookNotificationIntegrationOptions{}
		_, err := buildCreateWebhookNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		assert.Contains(t, err.Error(), "webhook_url is required")
	})
}

func TestBuildAlterWebhookNotificationIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterWebhookNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_WH"),
			Comment: ptr("updated comment"),
		}
		stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetWebhookURL", func(t *testing.T) {
		t.Parallel()
		opts := AlterWebhookNotificationIntegrationOptions{
			Name:       NewAccountObjectIdentifier("MY_WH"),
			WebhookURL: ptr("https://new-hooks.example.com/endpoint"),
		}
		stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "WEBHOOK_URL = 'https://new-hooks.example.com/endpoint'")
	})

	t.Run("SetHeaders", func(t *testing.T) {
		t.Parallel()
		opts := AlterWebhookNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("MY_WH"),
			WebhookHeaders: map[string]string{
				"X-Custom": "value",
			},
		}
		stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "WEBHOOK_HEADER_X-Custom = 'value'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterWebhookNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_WH"),
			UnsetFields: []string{"COMMENT", "WEBHOOK_SECRET"},
		}
		stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET")
		assert.Contains(t, stmts[0], "COMMENT")
		assert.Contains(t, stmts[0], "WEBHOOK_SECRET")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterWebhookNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_WH"),
			WebhookURL:  ptr("https://hooks.example.com/new"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterWebhookNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
	})
}

func TestWebhookAlterHasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts AlterWebhookNotificationIntegrationOptions
		want bool
	}{
		{"Empty", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T")}, false},
		{"Enabled", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Enabled: ptr(true)}, true},
		{"Comment", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Comment: ptr("c")}, true},
		{"WebhookURL", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), WebhookURL: ptr("url")}, true},
		{"WebhookSecret", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), WebhookSecret: ptr("s")}, true},
		{"WebhookBody", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), WebhookBodyTemplate: ptr("t")}, true},
		{"Headers", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), WebhookHeaders: map[string]string{"k": "v"}}, true},
		{"UnsetFields", AlterWebhookNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), UnsetFields: []string{"COMMENT"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.opts.HasChanges())
		})
	}
}
