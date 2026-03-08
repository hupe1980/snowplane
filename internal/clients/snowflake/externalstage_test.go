package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateExternalStageSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:     "s3://bucket/path/",
			Comment: ptr("test"),
		}

		got, err := buildCreateExternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE STAGE IF NOT EXISTS "DB"."SCHEMA"."MY_STAGE"`)
		assert.Contains(t, got, "URL = 's3://bucket/path/'")
		assert.Contains(t, got, "COMMENT")
	})

	t.Run("WithStorageIntegration", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name:               NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:                "s3://bucket/path/",
			StorageIntegration: ptr("MY_INT"),
		}

		got, err := buildCreateExternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `STORAGE_INTEGRATION = "MY_INT"`)
	})

	t.Run("WithEncryption", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:        "s3://bucket/path/",
			Encryption: &ExternalStageEncryptionOptions{Type: "AWS_SSE_S3"},
		}

		got, err := buildCreateExternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `ENCRYPTION = (TYPE = 'AWS_SSE_S3')`)
	})

	t.Run("WithDirectory", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:  "s3://bucket/path/",
			Directory: &ExternalStageDirectoryCreateOptions{
				Enable:                  true,
				AutoRefresh:             ptr(true),
				RefreshOnCreate:         ptr(true),
				NotificationIntegration: ptr("MY_NI"),
			},
		}

		got, err := buildCreateExternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "DIRECTORY = (ENABLE = TRUE")
		assert.Contains(t, got, "AUTO_REFRESH = TRUE")
		assert.Contains(t, got, "REFRESH_ON_CREATE = TRUE")
		assert.Contains(t, got, `NOTIFICATION_INTEGRATION = "MY_NI"`)
	})
}

func TestCreateExternalStageOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:  "s3://bucket/path/",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			URL: "s3://bucket/path/",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingURL", func(t *testing.T) {
		t.Parallel()

		opts := CreateExternalStageOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			URL:  "",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "url is required")
	})
}

func TestBuildAlterExternalStageStatements(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalStageOptions{Name: id}
		stmts, err := buildAlterExternalStageStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalStageOptions{
			Name:    id,
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterExternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[len(stmts)-1], "COMMENT = 'updated'")
	})

	t.Run("SetStorageIntegration", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalStageOptions{
			Name:               id,
			StorageIntegration: ptr("NEW_INT"),
		}
		stmts, err := buildAlterExternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[len(stmts)-1], `STORAGE_INTEGRATION = "NEW_INT"`)
	})

	t.Run("DirectoryUpdate", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalStageOptions{
			Name: id,
			Directory: &ExternalStageDirectoryCreateOptions{
				Enable: true,
			},
		}
		stmts, err := buildAlterExternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "DIRECTORY = (ENABLE = TRUE")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterExternalStageOptions{
			Name:        id,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterExternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterExternalStageOptions_HasChanges(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{Name: id}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{Name: id, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithStorageIntegration", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{Name: id, StorageIntegration: ptr("INT")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{Name: id, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterExternalStageOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalStageOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
