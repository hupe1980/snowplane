package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests – Email Notification Integration
// --------------------------------------------------------------------------

func TestBuildCreateEmailNotificationIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateEmailNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_EMAIL_INT"),
			AllowedRecipients: []string{"user@example.com"},
		}
		got, err := buildCreateEmailNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE NOTIFICATION INTEGRATION IF NOT EXISTS "MY_EMAIL_INT"`)
		assert.Contains(t, got, "TYPE = EMAIL")
		assert.Contains(t, got, "ALLOWED_RECIPIENTS = ('user@example.com')")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateEmailNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("FULL_EMAIL"),
			Enabled:           ptr(true),
			AllowedRecipients: []string{"a@test.com", "b@test.com"},
			DefaultRecipients: []string{"c@test.com"},
			DefaultSubject:    ptr("Alert from Snowflake"),
			Comment:           ptr("email notification integration"),
		}
		got, err := buildCreateEmailNotificationIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = EMAIL")
		assert.Contains(t, got, "ALLOWED_RECIPIENTS = ('a@test.com', 'b@test.com')")
		assert.Contains(t, got, "DEFAULT_RECIPIENTS = ('c@test.com')")
		assert.Contains(t, got, "DEFAULT_SUBJECT = 'Alert from Snowflake'")
		assert.Contains(t, got, "ENABLED = TRUE")
		assert.Contains(t, got, "COMMENT = 'email notification integration'")
	})
}

func TestBuildCreateEmailNotificationIntegrationSQL_Validation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateEmailNotificationIntegrationOptions{
			AllowedRecipients: []string{"a@test.com"},
		}
		_, err := buildCreateEmailNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingAllowedRecipients", func(t *testing.T) {
		t.Parallel()
		opts := CreateEmailNotificationIntegrationOptions{
			Name: NewAccountObjectIdentifier("TEST"),
		}
		_, err := buildCreateEmailNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "allowed_recipients is required")
	})

	t.Run("MultipleErrors", func(t *testing.T) {
		t.Parallel()
		opts := CreateEmailNotificationIntegrationOptions{}
		_, err := buildCreateEmailNotificationIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		assert.Contains(t, err.Error(), "allowed_recipients is required")
	})
}

func TestBuildAlterEmailNotificationIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterEmailNotificationIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_EMAIL"),
			Comment: ptr("updated comment"),
		}
		stmts, err := buildAlterEmailNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALTER NOTIFICATION INTEGRATION")
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetAllowedRecipients", func(t *testing.T) {
		t.Parallel()
		recipients := []string{"x@test.com", "y@test.com"}
		opts := AlterEmailNotificationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_EMAIL"),
			AllowedRecipients: &recipients,
		}
		stmts, err := buildAlterEmailNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALLOWED_RECIPIENTS = ('x@test.com', 'y@test.com')")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterEmailNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_EMAIL"),
			UnsetFields: []string{"COMMENT", "DEFAULT_SUBJECT"},
		}
		stmts, err := buildAlterEmailNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET")
		assert.Contains(t, stmts[0], "COMMENT")
		assert.Contains(t, stmts[0], "DEFAULT_SUBJECT")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterEmailNotificationIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_EMAIL"),
			Comment:     ptr("new comment"),
			UnsetFields: []string{"DEFAULT_SUBJECT"},
		}
		stmts, err := buildAlterEmailNotificationIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
	})
}

func TestEmailAlterHasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts AlterEmailNotificationIntegrationOptions
		want bool
	}{
		{"Empty", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T")}, false},
		{"Enabled", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Enabled: ptr(true)}, true},
		{"Comment", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), Comment: ptr("c")}, true},
		{"AllowedRecipients", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), AllowedRecipients: &[]string{"a@b.com"}}, true},
		{"DefaultSubject", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), DefaultSubject: ptr("s")}, true},
		{"UnsetFields", AlterEmailNotificationIntegrationOptions{Name: NewAccountObjectIdentifier("T"), UnsetFields: []string{"COMMENT"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.opts.HasChanges())
		})
	}
}
