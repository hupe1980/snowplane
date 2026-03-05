package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Create SQL
// --------------------------------------------------------------------------

func TestBuildCreateSharedDatabaseSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			Name:      NewAccountObjectIdentifier("SHARED_DB"),
			FromShare: "ab67890.sales_s",
		}
		sqlStr, err := buildCreateSharedDatabaseSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE DATABASE IF NOT EXISTS "SHARED_DB" FROM SHARE ab67890.sales_s`, sqlStr)
	})

	t.Run("WithOrgAccount", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			Name:      NewAccountObjectIdentifier("SHARED_DB"),
			FromShare: "myorg.myaccount.sales_share",
		}
		sqlStr, err := buildCreateSharedDatabaseSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE DATABASE IF NOT EXISTS "SHARED_DB" FROM SHARE myorg.myaccount.sales_share`, sqlStr)
	})
}

func TestCreateSharedDatabaseValidation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			FromShare: "ab67890.sales_s",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingFromShare", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			Name: NewAccountObjectIdentifier("DB"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fromShare is required")
	})

	t.Run("InvalidFromShareFormat", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			Name:      NewAccountObjectIdentifier("DB"),
			FromShare: "just_one_part",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider_account")
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateSharedDatabaseOptions{
			Name:      NewAccountObjectIdentifier("DB"),
			FromShare: "acct.share",
		}
		assert.NoError(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// Alter SQL
// --------------------------------------------------------------------------

func TestBuildAlterSharedDatabaseStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:    NewAccountObjectIdentifier("SHARED_DB"),
			Comment: ptr("shared db comment"),
		}
		stmts, err := buildAlterSharedDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `COMMENT = 'shared db comment'`)
	})

	t.Run("SetExternalVolume", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:           NewAccountObjectIdentifier("SHARED_DB"),
			ExternalVolume: ptr("my_vol"),
		}
		stmts, err := buildAlterSharedDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `EXTERNAL_VOLUME = 'my_vol'`)
	})

	t.Run("SetStorageSerializationPolicy", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:                       NewAccountObjectIdentifier("SHARED_DB"),
			StorageSerializationPolicy: ptr("COMPATIBLE"),
		}
		stmts, err := buildAlterSharedDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `STORAGE_SERIALIZATION_POLICY = COMPATIBLE`)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:        NewAccountObjectIdentifier("SHARED_DB"),
			UnsetFields: []string{"COMMENT", "EXTERNAL_VOLUME"},
		}
		stmts, err := buildAlterSharedDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
		assert.Contains(t, stmts[0], "EXTERNAL_VOLUME")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name: NewAccountObjectIdentifier("SHARED_DB"),
		}
		stmts, err := buildAlterSharedDatabaseStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})
}

func TestSharedDatabaseAlterValidation(t *testing.T) {
	t.Parallel()

	t.Run("InvalidStoragePolicy", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:                       NewAccountObjectIdentifier("DB"),
			StorageSerializationPolicy: ptr("INVALID"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storageSerializationPolicy")
	})

	t.Run("InvalidLogLevel", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:     NewAccountObjectIdentifier("DB"),
			LogLevel: ptr("INVALID"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logLevel")
	})

	t.Run("InvalidTraceLevel", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:       NewAccountObjectIdentifier("DB"),
			TraceLevel: ptr("INVALID"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "traceLevel")
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:    NewAccountObjectIdentifier("DB"),
			Comment: ptr("test"),
		}
		assert.NoError(t, opts.Validate())
	})
}

func TestSharedDatabaseAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{Name: NewAccountObjectIdentifier("DB")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:    NewAccountObjectIdentifier("DB"),
			Comment: ptr("c"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ExternalVolumeSet", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:           NewAccountObjectIdentifier("DB"),
			ExternalVolume: ptr("vol"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CatalogSet", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:    NewAccountObjectIdentifier("DB"),
			Catalog: ptr("cat"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ReplaceInvalidCharsSet", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:                     NewAccountObjectIdentifier("DB"),
			ReplaceInvalidCharacters: ptr(true),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterSharedDatabaseOptions{
			Name:        NewAccountObjectIdentifier("DB"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
