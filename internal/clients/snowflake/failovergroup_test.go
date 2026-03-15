package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// CREATE SQL tests
// --------------------------------------------------------------------------

func TestBuildCreateFailoverGroupSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateFailoverGroupOptions{
			Name:            NewAccountObjectIdentifier("MY_FG"),
			ObjectTypes:     []string{"DATABASES", "ROLES"},
			AllowedAccounts: []string{"MYORG.ACCOUNT2"},
		}
		sql, err := buildCreateFailoverGroupSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "CREATE FAILOVER GROUP \"MY_FG\"")
		assert.Contains(t, sql, "OBJECT_TYPES = DATABASES, ROLES")
		assert.Contains(t, sql, "ALLOWED_ACCOUNTS = MYORG.ACCOUNT2")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		ignoreEdition := true
		replSchedule := "10 MINUTE"
		errInteg := "MY_NOTIFICATION"
		comment := "DR group"
		opts := CreateFailoverGroupOptions{
			Name:                    NewAccountObjectIdentifier("FG_FULL"),
			ObjectTypes:             []string{"DATABASES", "SHARES", "INTEGRATIONS"},
			AllowedAccounts:         []string{"MYORG.ACCT1", "MYORG.ACCT2"},
			AllowedDatabases:        []string{"DB1", "DB2"},
			AllowedShares:           []string{"SHARE1"},
			AllowedIntegrationTypes: []string{"SECURITY INTEGRATIONS"},
			IgnoreEditionCheck:      &ignoreEdition,
			ReplicationSchedule:     &replSchedule,
			ErrorIntegration:        &errInteg,
			Comment:                 &comment,
		}
		sql, err := buildCreateFailoverGroupSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "ALLOWED_DATABASES = \"DB1\", \"DB2\"")
		assert.Contains(t, sql, "ALLOWED_SHARES = \"SHARE1\"")
		assert.Contains(t, sql, "ALLOWED_INTEGRATION_TYPES = SECURITY INTEGRATIONS")
		assert.Contains(t, sql, "ALLOWED_ACCOUNTS = MYORG.ACCT1, MYORG.ACCT2")
		assert.Contains(t, sql, "IGNORE EDITION CHECK")
		assert.Contains(t, sql, "REPLICATION_SCHEDULE = '10 MINUTE'")
		assert.Contains(t, sql, "ERROR_INTEGRATION = \"MY_NOTIFICATION\"")
		assert.Contains(t, sql, "COMMENT = 'DR group'")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("EmptyName", func(t *testing.T) {
			t.Parallel()
			opts := CreateFailoverGroupOptions{
				ObjectTypes:     []string{"DATABASES"},
				AllowedAccounts: []string{"MYORG.ACCT1"},
			}
			_, err := buildCreateFailoverGroupSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("NoObjectTypes", func(t *testing.T) {
			t.Parallel()
			opts := CreateFailoverGroupOptions{
				Name:            NewAccountObjectIdentifier("FG"),
				AllowedAccounts: []string{"MYORG.ACCT1"},
			}
			_, err := buildCreateFailoverGroupSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "at least one object type")
		})

		t.Run("NoAllowedAccounts", func(t *testing.T) {
			t.Parallel()
			opts := CreateFailoverGroupOptions{
				Name:        NewAccountObjectIdentifier("FG"),
				ObjectTypes: []string{"DATABASES"},
			}
			_, err := buildCreateFailoverGroupSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "at least one allowed account")
		})

		t.Run("InvalidObjectType", func(t *testing.T) {
			t.Parallel()
			opts := CreateFailoverGroupOptions{
				Name:            NewAccountObjectIdentifier("FG"),
				ObjectTypes:     []string{"INVALID_TYPE"},
				AllowedAccounts: []string{"MYORG.ACCT1"},
			}
			_, err := buildCreateFailoverGroupSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid object type")
		})

		t.Run("InvalidIntegrationType", func(t *testing.T) {
			t.Parallel()
			opts := CreateFailoverGroupOptions{
				Name:                    NewAccountObjectIdentifier("FG"),
				ObjectTypes:             []string{"INTEGRATIONS"},
				AllowedAccounts:         []string{"MYORG.ACCT1"},
				AllowedIntegrationTypes: []string{"INVALID_INTEG"},
			}
			_, err := buildCreateFailoverGroupSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid integration type")
		})
	})
}

// --------------------------------------------------------------------------
// ALTER SQL tests
// --------------------------------------------------------------------------

func TestBuildAlterFailoverGroupStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		c := "new comment"
		opts := AlterFailoverGroupOptions{
			Name:    NewAccountObjectIdentifier("MY_FG"),
			Comment: &c,
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'new comment'")
	})

	t.Run("SetObjectTypes", func(t *testing.T) {
		t.Parallel()
		ots := []string{"DATABASES", "ROLES", "USERS"}
		opts := AlterFailoverGroupOptions{
			Name:        NewAccountObjectIdentifier("MY_FG"),
			ObjectTypes: &ots,
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "OBJECT_TYPES = DATABASES, ROLES, USERS")
	})

	t.Run("SetAllowedDatabases", func(t *testing.T) {
		t.Parallel()
		dbs := []string{"DB1", "DB2"}
		opts := AlterFailoverGroupOptions{
			Name:             NewAccountObjectIdentifier("MY_FG"),
			AllowedDatabases: &dbs,
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALLOWED_DATABASES = \"DB1\", \"DB2\"")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterFailoverGroupOptions{
			Name:        NewAccountObjectIdentifier("MY_FG"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterFailoverGroupOptions{
			Name: NewAccountObjectIdentifier("MY_FG"),
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("MultipleChanges", func(t *testing.T) {
		t.Parallel()
		ots := []string{"DATABASES"}
		accts := []string{"MYORG.ACCT1"}
		comment := "updated"
		opts := AlterFailoverGroupOptions{
			Name:            NewAccountObjectIdentifier("MY_FG"),
			ObjectTypes:     &ots,
			AllowedAccounts: &accts,
			Comment:         &comment,
		}
		stmts, err := buildAlterFailoverGroupStatements(opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(stmts), 3) // OBJECT_TYPES + ALLOWED_ACCOUNTS + COMMENT
	})
}

// --------------------------------------------------------------------------
// HasChanges tests
// --------------------------------------------------------------------------

func TestFailoverGroupAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := &AlterFailoverGroupOptions{Name: NewAccountObjectIdentifier("FG")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "test"
		opts := &AlterFailoverGroupOptions{Name: NewAccountObjectIdentifier("FG"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := &AlterFailoverGroupOptions{Name: NewAccountObjectIdentifier("FG"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithObjectTypes", func(t *testing.T) {
		t.Parallel()
		ots := []string{"DATABASES"}
		opts := &AlterFailoverGroupOptions{Name: NewAccountObjectIdentifier("FG"), ObjectTypes: &ots}
		assert.True(t, opts.HasChanges())
	})
}

// --------------------------------------------------------------------------
// Validate tests
// --------------------------------------------------------------------------

func TestFailoverGroupCreateValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := &CreateFailoverGroupOptions{
			Name:            NewAccountObjectIdentifier("FG"),
			ObjectTypes:     []string{"DATABASES"},
			AllowedAccounts: []string{"MYORG.ACCT1"},
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("AllObjectTypes", func(t *testing.T) {
		t.Parallel()
		opts := &CreateFailoverGroupOptions{
			Name: NewAccountObjectIdentifier("FG"),
			ObjectTypes: []string{
				"ACCOUNT PARAMETERS", "DATABASES", "INTEGRATIONS",
				"NETWORK POLICIES", "RESOURCE MONITORS", "ROLES",
				"SHARES", "USERS", "WAREHOUSES",
			},
			AllowedAccounts: []string{"MYORG.ACCT1"},
		}
		assert.NoError(t, opts.Validate())
	})
}
