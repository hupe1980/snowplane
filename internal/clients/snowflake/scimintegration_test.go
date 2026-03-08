package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateSCIMIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:       NewAccountObjectIdentifier("MY_SCIM_INT"),
			SCIMClient: "OKTA",
			RunAsRole:  "OKTA_PROVISIONER",
		}
		got, err := buildCreateSCIMIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECURITY INTEGRATION IF NOT EXISTS "MY_SCIM_INT"`)
		assert.Contains(t, got, "TYPE = SCIM")
		assert.Contains(t, got, "SCIM_CLIENT = 'OKTA'")
		assert.Contains(t, got, "RUN_AS_ROLE = 'OKTA_PROVISIONER'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:          NewAccountObjectIdentifier("FULL_SCIM"),
			SCIMClient:    "AZURE",
			RunAsRole:     "AAD_PROVISIONER",
			Enabled:       ptr(true),
			NetworkPolicy: ptr("MY_NET_POLICY"),
			SyncPassword:  ptr(false),
			Comment:       ptr("my scim integration"),
		}
		got, err := buildCreateSCIMIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = SCIM")
		assert.Contains(t, got, "SCIM_CLIENT = 'AZURE'")
		assert.Contains(t, got, "RUN_AS_ROLE = 'AAD_PROVISIONER'")
		assert.Contains(t, got, "NETWORK_POLICY = 'MY_NET_POLICY'")
		assert.Contains(t, got, "SYNC_PASSWORD = FALSE")
		assert.Contains(t, got, "ENABLED = TRUE")
		assert.Contains(t, got, "COMMENT = 'my scim integration'")
	})

	t.Run("WithGenericClient", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:       NewAccountObjectIdentifier("GENERIC_SCIM"),
			SCIMClient: "GENERIC",
			RunAsRole:  "GENERIC_SCIM_PROVISIONER",
		}
		got, err := buildCreateSCIMIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "SCIM_CLIENT = 'GENERIC'")
		assert.Contains(t, got, "RUN_AS_ROLE = 'GENERIC_SCIM_PROVISIONER'")
	})
}

func TestBuildCreateSCIMIntegrationSQL_Validation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			SCIMClient: "OKTA",
			RunAsRole:  "OKTA_PROVISIONER",
		}
		_, err := buildCreateSCIMIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingSCIMClient", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:      NewAccountObjectIdentifier("TEST"),
			RunAsRole: "OKTA_PROVISIONER",
		}
		_, err := buildCreateSCIMIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scim_client is required")
	})

	t.Run("MissingRunAsRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:       NewAccountObjectIdentifier("TEST"),
			SCIMClient: "OKTA",
		}
		_, err := buildCreateSCIMIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "run_as_role is required")
	})

	t.Run("MultipleErrors", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{}
		_, err := buildCreateSCIMIntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		assert.Contains(t, err.Error(), "scim_client is required")
		assert.Contains(t, err.Error(), "run_as_role is required")
	})
}

// --------------------------------------------------------------------------
// Alter SQL generation tests
// --------------------------------------------------------------------------

func TestBuildAlterSCIMIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_SCIM_INT"),
			Comment: ptr("updated comment"),
		}
		got, err := buildAlterSCIMIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], `ALTER SECURITY INTEGRATION "MY_SCIM_INT" SET`)
		assert.Contains(t, got[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetMultipleFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{
			Name:          NewAccountObjectIdentifier("MY_SCIM_INT"),
			Enabled:       ptr(false),
			NetworkPolicy: ptr("NEW_POLICY"),
			SyncPassword:  ptr(true),
			Comment:       ptr("new comment"),
		}
		got, err := buildAlterSCIMIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "ENABLED = FALSE")
		assert.Contains(t, got[0], "NETWORK_POLICY = 'NEW_POLICY'")
		assert.Contains(t, got[0], "SYNC_PASSWORD = TRUE")
		assert.Contains(t, got[0], "COMMENT = 'new comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_SCIM_INT"),
			UnsetFields: []string{"COMMENT", "NETWORK_POLICY"},
		}
		got, err := buildAlterSCIMIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "UNSET")
		assert.Contains(t, got[0], "COMMENT")
		assert.Contains(t, got[0], "NETWORK_POLICY")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_SCIM_INT"),
			Enabled:     ptr(true),
			UnsetFields: []string{"COMMENT"},
		}
		got, err := buildAlterSCIMIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Contains(t, got[0], "SET")
		assert.Contains(t, got[0], "ENABLED = TRUE")
		assert.Contains(t, got[1], "UNSET")
		assert.Contains(t, got[1], "COMMENT")
	})

	t.Run("ValidationFailsMissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{
			Comment: ptr("test"),
		}
		_, err := buildAlterSCIMIntegrationStatements(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

// --------------------------------------------------------------------------
// HasChanges tests
// --------------------------------------------------------------------------

func TestSCIMAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithEnabled", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), Enabled: ptr(true)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), NetworkPolicy: ptr("POL")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithSyncPassword", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), SyncPassword: ptr(false)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), Comment: ptr("hello")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

// --------------------------------------------------------------------------
// Validate tests
// --------------------------------------------------------------------------

func TestSCIMCreateValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSCIMIntegrationOptions{
			Name:       NewAccountObjectIdentifier("TEST"),
			SCIMClient: "OKTA",
			RunAsRole:  "OKTA_PROVISIONER",
		}
		assert.NoError(t, opts.Validate())
	})
}

func TestSCIMAlterValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{Name: NewAccountObjectIdentifier("TEST")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSCIMIntegrationOptions{}
		assert.Error(t, opts.Validate())
	})
}
