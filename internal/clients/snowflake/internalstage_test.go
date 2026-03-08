package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateInternalStageSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			Comment: ptr("my stage comment"),
		}

		got, err := buildCreateInternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE STAGE IF NOT EXISTS "DB"."SCHEMA"."MY_STAGE"`)
		assert.Contains(t, got, "COMMENT")
	})

	t.Run("WithEncryption", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			Encryption: &InternalStageEncryptionOptions{Type: "SNOWFLAKE_FULL"},
		}

		got, err := buildCreateInternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `ENCRYPTION = (TYPE = 'SNOWFLAKE_FULL')`)
	})

	t.Run("WithDirectory", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			Directory: &InternalStageDirectoryCreateOptions{
				Enable:          true,
				RefreshOnCreate: ptr(true),
			},
		}

		got, err := buildCreateInternalStageSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `DIRECTORY = (ENABLE = TRUE`)
		assert.Contains(t, got, `REFRESH_ON_CREATE = TRUE`)
	})
}

func TestCreateInternalStageOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("InvalidEncryption", func(t *testing.T) {
		t.Parallel()

		opts := CreateInternalStageOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE"),
			Encryption: &InternalStageEncryptionOptions{Type: "AWS_CSE"},
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "encryption type")
	})
}

func TestBuildAlterInternalStageStatements(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterInternalStageOptions{Name: id}
		stmts, err := buildAlterInternalStageStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterInternalStageOptions{
			Name:    id,
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterInternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[len(stmts)-1], "SET")
		assert.Contains(t, stmts[len(stmts)-1], "COMMENT = 'updated'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterInternalStageOptions{
			Name:        id,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterInternalStageStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterInternalStageOptions_HasChanges(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterInternalStageOptions{Name: id}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterInternalStageOptions{Name: id, Comment: ptr("x")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterInternalStageOptions{Name: id, UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterInternalStageOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterInternalStageOptions{Name: NewSchemaObjectIdentifier("DB", "SCHEMA", "MY_STAGE")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterInternalStageOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
