package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateStorageIntegrationGCSSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationGCSOptions{
			Name:                    NewAccountObjectIdentifier("MY_GCS_INT"),
			StorageAllowedLocations: []string{"gcs://mybucket/"},
		}

		got, err := buildCreateStorageIntegrationGCSSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE STORAGE INTEGRATION IF NOT EXISTS "MY_GCS_INT"`)
		assert.Contains(t, got, "TYPE = 'EXTERNAL_STAGE'")
		assert.Contains(t, got, "STORAGE_PROVIDER = 'GCS'")
		assert.Contains(t, got, "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationGCSOptions{
			Name:                    NewAccountObjectIdentifier("MY_GCS_INT"),
			StorageAllowedLocations: []string{"gcs://mybucket/"},
			StorageBlockedLocations: []string{"gcs://mybucket/sensitive/"},
			Enabled:                 ptr(false),
			Comment:                 ptr("test"),
		}

		got, err := buildCreateStorageIntegrationGCSSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ENABLED = FALSE")
		assert.Contains(t, got, "STORAGE_BLOCKED_LOCATIONS")
		assert.Contains(t, got, "COMMENT = 'test'")
	})
}

func TestCreateStorageIntegrationGCSOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationGCSOptions{
			Name:                    NewAccountObjectIdentifier("MY_GCS_INT"),
			StorageAllowedLocations: []string{"gcs://mybucket/"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationGCSOptions{
			StorageAllowedLocations: []string{"gcs://mybucket/"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingLocations", func(t *testing.T) {
		t.Parallel()

		opts := CreateStorageIntegrationGCSOptions{
			Name: NewAccountObjectIdentifier("MY_GCS_INT"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage allowed location")
	})
}

func TestBuildAlterStorageIntegrationGCSStatements(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_GCS_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationGCSOptions{Name: name}
		stmts, err := buildAlterStorageIntegrationGCSStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationGCSOptions{
			Name:    name,
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterStorageIntegrationGCSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UpdateLocations", func(t *testing.T) {
		t.Parallel()

		locs := []string{"gcs://a/", "gcs://b/"}
		opts := AlterStorageIntegrationGCSOptions{
			Name:                    name,
			StorageAllowedLocations: &locs,
		}
		stmts, err := buildAlterStorageIntegrationGCSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "STORAGE_ALLOWED_LOCATIONS")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterStorageIntegrationGCSOptions{
			Name:        name,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterStorageIntegrationGCSStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterStorageIntegrationGCSOptions_HasChanges(t *testing.T) {
	t.Parallel()

	name := NewAccountObjectIdentifier("MY_GCS_INT")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationGCSOptions{Name: name}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationGCSOptions{Name: name, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationGCSOptions{Name: name, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterStorageIntegrationGCSOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationGCSOptions{Name: NewAccountObjectIdentifier("MY_GCS_INT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterStorageIntegrationGCSOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
