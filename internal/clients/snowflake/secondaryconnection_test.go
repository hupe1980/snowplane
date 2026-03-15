package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateSecondaryConnectionSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecondaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("MY_REPLICA"),
			AsReplicaOf: "myorg.primary_account.MY_CONN",
		}
		got, err := buildCreateSecondaryConnectionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE CONNECTION IF NOT EXISTS "MY_REPLICA"`)
		assert.Contains(t, got, `AS REPLICA OF "myorg"."primary_account"."MY_CONN"`)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecondaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("MY_REPLICA"),
			AsReplicaOf: "myorg.primary_account.MY_CONN",
			Comment:     ptr("replica connection"),
		}
		got, err := buildCreateSecondaryConnectionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `AS REPLICA OF "myorg"."primary_account"."MY_CONN"`)
		assert.Contains(t, got, "COMMENT = 'replica connection'")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := CreateSecondaryConnectionOptions{
				AsReplicaOf: "myorg.primary_account.MY_CONN",
			}
			_, err := buildCreateSecondaryConnectionSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("MissingAsReplicaOf", func(t *testing.T) {
			t.Parallel()
			opts := CreateSecondaryConnectionOptions{
				Name: NewAccountObjectIdentifier("MY_REPLICA"),
			}
			_, err := buildCreateSecondaryConnectionSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "asReplicaOf is required")
		})

		t.Run("InvalidAsReplicaOf_SQLInjection", func(t *testing.T) {
			t.Parallel()
			opts := CreateSecondaryConnectionOptions{
				Name:        NewAccountObjectIdentifier("MY_REPLICA"),
				AsReplicaOf: "myorg.primary_account.MY_CONN; DROP CONNECTION --",
			}
			_, err := buildCreateSecondaryConnectionSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid asReplicaOf identifier")
		})
	})
}

func TestBuildAlterSecondaryConnectionStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecondaryConnectionOptions{
			Name:    NewAccountObjectIdentifier("MY_REPLICA"),
			Comment: ptr("updated comment"),
		}
		stmts, err := buildAlterSecondaryConnectionStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecondaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("MY_REPLICA"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterSecondaryConnectionStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := AlterSecondaryConnectionOptions{}
			_, err := buildAlterSecondaryConnectionStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})
	})
}

func TestSecondaryConnectionAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecondaryConnectionOptions{Name: NewAccountObjectIdentifier("C")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecondaryConnectionOptions{
			Name:    NewAccountObjectIdentifier("C"),
			Comment: ptr("x"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecondaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("C"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestSecondaryConnectionCreateValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecondaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("C"),
			AsReplicaOf: "org.acct.conn",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecondaryConnectionOptions{
			AsReplicaOf: "org.acct.conn",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingAsReplicaOf", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecondaryConnectionOptions{
			Name: NewAccountObjectIdentifier("C"),
		}
		require.Error(t, opts.Validate())
	})
}
