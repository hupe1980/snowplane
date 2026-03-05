package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Create SQL
// --------------------------------------------------------------------------

func TestBuildCreateSecondaryDatabaseSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name:        NewAccountObjectIdentifier("MY_REPLICA"),
			AsReplicaOf: "myorg.myaccount.MY_PRIMARY",
		}
		sql, err := buildCreateSecondaryDatabaseSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE DATABASE IF NOT EXISTS "MY_REPLICA" AS REPLICA OF myorg.myaccount.MY_PRIMARY`, sql)
	})

	t.Run("WithRetentionDays", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name:                    NewAccountObjectIdentifier("MY_REPLICA"),
			AsReplicaOf:             "myorg.myaccount.MY_PRIMARY",
			DataRetentionTimeInDays: ptr[int32](7),
		}
		sql, err := buildCreateSecondaryDatabaseSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `AS REPLICA OF myorg.myaccount.MY_PRIMARY`)
		assert.Contains(t, sql, `DATA_RETENTION_TIME_IN_DAYS = 7`)
	})
}

func TestCreateSecondaryDatabaseValidation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			AsReplicaOf: "myorg.myaccount.MY_PRIMARY",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingAsReplicaOf", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name: NewAccountObjectIdentifier("DB"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "asReplicaOf is required")
	})

	t.Run("InvalidAsReplicaOfFormat", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name:        NewAccountObjectIdentifier("DB"),
			AsReplicaOf: "just_one_part",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "org.account.db_name")
	})

	t.Run("InvalidRetention", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name:                    NewAccountObjectIdentifier("DB"),
			AsReplicaOf:             "org.acct.db",
			DataRetentionTimeInDays: ptr[int32](100),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Retention")
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateSecondaryDatabaseOptions{
			Name:        NewAccountObjectIdentifier("DB"),
			AsReplicaOf: "org.acct.db",
		}
		assert.NoError(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// Alter SQL
// --------------------------------------------------------------------------

func TestBuildAlterSecondaryDatabaseStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name:    NewAccountObjectIdentifier("MY_REPLICA"),
			Comment: ptr("replica comment"),
		}
		stmts, err := buildAlterSecondaryDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `COMMENT = 'replica comment'`)
	})

	t.Run("SetRetention", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name:                    NewAccountObjectIdentifier("MY_REPLICA"),
			DataRetentionTimeInDays: ptr[int32](14),
		}
		stmts, err := buildAlterSecondaryDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `DATA_RETENTION_TIME_IN_DAYS = 14`)
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name:        NewAccountObjectIdentifier("MY_REPLICA"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterSecondaryDatabaseStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name: NewAccountObjectIdentifier("MY_REPLICA"),
		}
		stmts, err := buildAlterSecondaryDatabaseStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})
}

func TestSecondaryDatabaseAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{Name: NewAccountObjectIdentifier("DB")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name:    NewAccountObjectIdentifier("DB"),
			Comment: ptr("c"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterSecondaryDatabaseOptions{
			Name:        NewAccountObjectIdentifier("DB"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}

// --------------------------------------------------------------------------
// Refresh SQL
// --------------------------------------------------------------------------

func TestBuildRefreshSQL(t *testing.T) {
	t.Parallel()

	sql := buildRefreshSQL(NewAccountObjectIdentifier("MY_REPLICA"))
	assert.Equal(t, `ALTER DATABASE "MY_REPLICA" REFRESH`, sql)
}
