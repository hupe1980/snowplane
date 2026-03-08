package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateStorageIntegrationAzureSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			Name:                    NewAccountObjectIdentifier("MY_AZURE_INT"),
			AzureTenantID:           "my-tenant-id",
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/container/"},
		}

		got, err := buildCreateStorageIntegrationAzureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE STORAGE INTEGRATION IF NOT EXISTS "MY_AZURE_INT"`)
		assert.Contains(t, got, "TYPE = 'EXTERNAL_STAGE'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'AZURE'")
		assert.Contains(t, got, "AZURE_TENANT_ID = 'my-tenant-id'")
		assert.Contains(t, got, "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			Name:                    NewAccountObjectIdentifier("MY_AZURE_INT"),
			AzureTenantID:           "my-tenant-id",
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/container/"},
			StorageBlockedLocations: []string{"azure://myaccount.blob.core.windows.net/container/sensitive/"},
			Enabled:                 ptr(false),
			Comment:                 ptr("test"),
		}

		got, err := buildCreateStorageIntegrationAzureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ENABLED = FALSE")
		assert.Contains(t, got, "STORAGE_BLOCKED_LOCATIONS")
		assert.Contains(t, got, "COMMENT = 'test'")
	})
}

func TestCreateStorageIntegrationAzureOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			Name:                    NewAccountObjectIdentifier("MY_AZURE_INT"),
			AzureTenantID:           "my-tenant-id",
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/container/"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			AzureTenantID:           "my-tenant-id",
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/container/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingTenantID", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			Name:                    NewAccountObjectIdentifier("MY_AZURE_INT"),
			AzureTenantID:           "",
			StorageAllowedLocations: []string{"azure://myaccount.blob.core.windows.net/container/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "azureTenantID")
	})

	t.Run("MissingLocations", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationAzureOptions{
			Name:          NewAccountObjectIdentifier("MY_AZURE_INT"),
			AzureTenantID: "my-tenant-id",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage allowed location")
	})
}

func TestBuildAlterStorageIntegrationAzureStatements(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_AZURE_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAzureOptions{Name: name}
		stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAzureOptions{
			Name:    name,
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UpdateLocations", func(t *testing.T) {
		t.Parallel()

		locs := []string{"azure://a/", "azure://b/"}
		opts := AlterStorageIntegrationAzureOptions{
			Name:                    name,
			StorageAllowedLocations: &locs,
		}
		stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("SetTenantID", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAzureOptions{
			Name:          name,
			AzureTenantID: ptr("new-tenant"),
		}
		stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "AZURE_TENANT_ID = 'new-tenant'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationAzureOptions{
			Name:        name,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterStorageIntegrationAzureOptions_HasChanges(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_AZURE_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAzureOptions{Name: name}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAzureOptions{Name: name, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAzureOptions{Name: name, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterStorageIntegrationAzureOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAzureOptions{Name: NewAccountObjectIdentifier("MY_AZURE_INT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationAzureOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
