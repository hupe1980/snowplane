package snowflake

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreatePrimaryConnectionSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreatePrimaryConnectionOptions{
			Name: NewAccountObjectIdentifier("MY_CONN"),
		}
		got, err := buildCreatePrimaryConnectionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE CONNECTION IF NOT EXISTS "MY_CONN"`)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := CreatePrimaryConnectionOptions{
			Name:    NewAccountObjectIdentifier("MY_CONN"),
			Comment: ptr("test connection"),
		}
		got, err := buildCreatePrimaryConnectionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE CONNECTION IF NOT EXISTS "MY_CONN"`)
		assert.Contains(t, got, "COMMENT = 'test connection'")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := CreatePrimaryConnectionOptions{}
			_, err := buildCreatePrimaryConnectionSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})
	})
}

func TestBuildAlterPrimaryConnectionStatements(t *testing.T) {
	t.Parallel()

	t.Run("EnableFailover", func(t *testing.T) {
		t.Parallel()
		accounts := []string{"myorg.account1", "myorg.account2"}
		opts := AlterPrimaryConnectionOptions{
			Name:                     NewAccountObjectIdentifier("MY_CONN"),
			EnableFailoverToAccounts: &accounts,
		}
		stmts, err := buildAlterPrimaryConnectionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALTER CONNECTION")
		assert.Contains(t, stmts[0], "ENABLE FAILOVER TO ACCOUNTS")
		assert.Contains(t, stmts[0], `"myorg"."account1"`)
		assert.Contains(t, stmts[0], `"myorg"."account2"`)
	})

	t.Run("DisableFailover", func(t *testing.T) {
		t.Parallel()
		empty := []string{}
		opts := AlterPrimaryConnectionOptions{
			Name:                     NewAccountObjectIdentifier("MY_CONN"),
			EnableFailoverToAccounts: &empty,
		}
		stmts, err := buildAlterPrimaryConnectionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "DISABLE FAILOVER")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterPrimaryConnectionOptions{
			Name:    NewAccountObjectIdentifier("MY_CONN"),
			Comment: ptr("updated comment"),
		}
		stmts, err := buildAlterPrimaryConnectionStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		found := false
		for _, s := range stmts {
			if strings.Contains(s, "COMMENT") && strings.Contains(s, "'updated comment'") {
				found = true
			}
		}
		assert.True(t, found, "expected a SET COMMENT statement")
	})

	t.Run("FailoverAndComment", func(t *testing.T) {
		t.Parallel()
		accounts := []string{"myorg.account1"}
		opts := AlterPrimaryConnectionOptions{
			Name:                     NewAccountObjectIdentifier("MY_CONN"),
			EnableFailoverToAccounts: &accounts,
			Comment:                  ptr("with failover"),
		}
		stmts, err := buildAlterPrimaryConnectionStatements(opts)
		require.NoError(t, err)
		// Should have at least 2 statements: one for failover, one for SET COMMENT
		require.GreaterOrEqual(t, len(stmts), 2)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterPrimaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("MY_CONN"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterPrimaryConnectionStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		found := false
		for _, s := range stmts {
			if strings.Contains(s, "UNSET") {
				found = true
			}
		}
		assert.True(t, found, "expected an UNSET statement")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := AlterPrimaryConnectionOptions{}
			_, err := buildAlterPrimaryConnectionStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("InvalidFailoverAccount_SQLInjection", func(t *testing.T) {
			t.Parallel()
			accounts := []string{"myorg.account1; DROP CONNECTION --"}
			opts := AlterPrimaryConnectionOptions{
				Name:                     NewAccountObjectIdentifier("MY_CONN"),
				EnableFailoverToAccounts: &accounts,
			}
			_, err := buildAlterPrimaryConnectionStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid failover account identifier")
		})
	})
}

func TestPrimaryConnectionAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterPrimaryConnectionOptions{Name: NewAccountObjectIdentifier("C")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterPrimaryConnectionOptions{
			Name:    NewAccountObjectIdentifier("C"),
			Comment: ptr("x"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithFailover", func(t *testing.T) {
		t.Parallel()
		accts := []string{"a.b"}
		opts := AlterPrimaryConnectionOptions{
			Name:                     NewAccountObjectIdentifier("C"),
			EnableFailoverToAccounts: &accts,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterPrimaryConnectionOptions{
			Name:        NewAccountObjectIdentifier("C"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestPrimaryConnectionCreateValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreatePrimaryConnectionOptions{Name: NewAccountObjectIdentifier("C")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreatePrimaryConnectionOptions{}
		require.Error(t, opts.Validate())
	})
}
