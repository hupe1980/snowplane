package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildGrantRoleSQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole_ToRole", func(t *testing.T) {
		t.Parallel()
		opts := GrantRoleOptions{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			ToRole:         "SYSADMIN",
		}
		got := buildGrantRoleSQL(opts)
		assert.Equal(t, `GRANT ROLE "ANALYST" TO ROLE "SYSADMIN"`, got)
	})

	t.Run("AccountRole_ToUser", func(t *testing.T) {
		t.Parallel()
		opts := GrantRoleOptions{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			ToUser:         "john",
		}
		got := buildGrantRoleSQL(opts)
		assert.Equal(t, `GRANT ROLE "ANALYST" TO USER "john"`, got)
	})

	t.Run("DatabaseRole_ToRole", func(t *testing.T) {
		t.Parallel()
		opts := GrantRoleOptions{
			RoleName:       "MY_DB.READER",
			IsDatabaseRole: true,
			ToRole:         "SYSADMIN",
		}
		got := buildGrantRoleSQL(opts)
		assert.Equal(t, `GRANT DATABASE ROLE "MY_DB"."READER" TO ROLE "SYSADMIN"`, got)
	})

	t.Run("DatabaseRole_ToDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := GrantRoleOptions{
			RoleName:       "MY_DB.READER",
			IsDatabaseRole: true,
			ToDatabaseRole: "MY_DB.WRITER",
		}
		got := buildGrantRoleSQL(opts)
		assert.Equal(t, `GRANT DATABASE ROLE "MY_DB"."READER" TO DATABASE ROLE "MY_DB"."WRITER"`, got)
	})
}

func TestBuildRevokeRoleSQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole_FromRole", func(t *testing.T) {
		t.Parallel()
		opts := RevokeRoleOptions{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			FromRole:       "SYSADMIN",
		}
		got := buildRevokeRoleSQL(opts)
		assert.Equal(t, `REVOKE ROLE "ANALYST" FROM ROLE "SYSADMIN"`, got)
	})

	t.Run("AccountRole_FromUser", func(t *testing.T) {
		t.Parallel()
		opts := RevokeRoleOptions{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			FromUser:       "john",
		}
		got := buildRevokeRoleSQL(opts)
		assert.Equal(t, `REVOKE ROLE "ANALYST" FROM USER "john"`, got)
	})

	t.Run("DatabaseRole_FromRole", func(t *testing.T) {
		t.Parallel()
		opts := RevokeRoleOptions{
			RoleName:       "MY_DB.READER",
			IsDatabaseRole: true,
			FromRole:       "SYSADMIN",
		}
		got := buildRevokeRoleSQL(opts)
		assert.Equal(t, `REVOKE DATABASE ROLE "MY_DB"."READER" FROM ROLE "SYSADMIN"`, got)
	})

	t.Run("DatabaseRole_FromDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := RevokeRoleOptions{
			RoleName:         "MY_DB.READER",
			IsDatabaseRole:   true,
			FromDatabaseRole: "MY_DB.WRITER",
		}
		got := buildRevokeRoleSQL(opts)
		assert.Equal(t, `REVOKE DATABASE ROLE "MY_DB"."READER" FROM DATABASE ROLE "MY_DB"."WRITER"`, got)
	})
}

// --------------------------------------------------------------------------
// GrantRoleOptions validation tests
// --------------------------------------------------------------------------

func TestGrantRoleOptionsValidation(t *testing.T) {
	t.Parallel()

	t.Run("Valid_ToRole", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "ANALYST", ToRole: "SYSADMIN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_ToUser", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "ANALYST", ToUser: "john"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_ToDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "MY_DB.READER", IsDatabaseRole: true, ToDatabaseRole: "MY_DB.WRITER"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Error_EmptyRoleName", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "", ToRole: "SYSADMIN"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "roleName is required")
	})

	t.Run("Error_WhitespaceRoleName", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "   ", ToRole: "SYSADMIN"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "roleName is required")
	})

	t.Run("Error_NoTarget", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "ANALYST"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one")
	})

	t.Run("Error_MultipleTargets", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "ANALYST", ToRole: "SYSADMIN", ToUser: "john"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one")
	})

	t.Run("Error_ToDatabaseRole_NotDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "ANALYST", IsDatabaseRole: false, ToDatabaseRole: "MY_DB.WRITER"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "toDatabaseRole is only valid for database role")
	})

	t.Run("Error_ToUser_DatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &GrantRoleOptions{RoleName: "MY_DB.READER", IsDatabaseRole: true, ToUser: "john"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "toUser is not valid for database role")
	})
}

// --------------------------------------------------------------------------
// RevokeRoleOptions validation tests
// --------------------------------------------------------------------------

func TestRevokeRoleOptionsValidation(t *testing.T) {
	t.Parallel()

	t.Run("Valid_FromRole", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "ANALYST", FromRole: "SYSADMIN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_FromUser", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "ANALYST", FromUser: "john"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Error_EmptyRoleName", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "", FromRole: "SYSADMIN"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "roleName is required")
	})

	t.Run("Error_NoTarget", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "ANALYST"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one")
	})

	t.Run("Error_MultipleTargets", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "ANALYST", FromRole: "SYSADMIN", FromUser: "john"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exactly one")
	})

	t.Run("Error_FromDatabaseRole_NotDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "ANALYST", IsDatabaseRole: false, FromDatabaseRole: "MY_DB.WRITER"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fromDatabaseRole is only valid for database role")
	})

	t.Run("Error_FromUser_DatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "MY_DB.READER", IsDatabaseRole: true, FromUser: "john"}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fromUser is not valid for database role")
	})

	t.Run("Valid_FromDatabaseRole_IsDatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := &RevokeRoleOptions{RoleName: "MY_DB.READER", IsDatabaseRole: true, FromDatabaseRole: "MY_DB.WRITER"}
		require.NoError(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// RoleAssignmentIdentifier tests
// --------------------------------------------------------------------------

func TestRoleAssignmentIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole_ToRole", func(t *testing.T) {
		t.Parallel()
		id := RoleAssignmentIdentifier{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			GrantedTo:      "ROLE",
			GranteeName:    "SYSADMIN",
		}
		assert.Equal(t, `GRANT ROLE "ANALYST" TO ROLE "SYSADMIN"`, id.FullyQualifiedName())
	})

	t.Run("AccountRole_ToUser", func(t *testing.T) {
		t.Parallel()
		id := RoleAssignmentIdentifier{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			GrantedTo:      "USER",
			GranteeName:    "john",
		}
		assert.Equal(t, `GRANT ROLE "ANALYST" TO USER "john"`, id.FullyQualifiedName())
	})

	t.Run("DatabaseRole_ToRole", func(t *testing.T) {
		t.Parallel()
		id := RoleAssignmentIdentifier{
			RoleName:       "MY_DB.READER",
			IsDatabaseRole: true,
			GrantedTo:      "ROLE",
			GranteeName:    "SYSADMIN",
		}
		assert.Equal(t, `GRANT DATABASE ROLE "MY_DB"."READER" TO ROLE "SYSADMIN"`, id.FullyQualifiedName())
	})

	t.Run("DatabaseRole_ToDatabaseRole", func(t *testing.T) {
		t.Parallel()
		id := RoleAssignmentIdentifier{
			RoleName:       "MY_DB.READER",
			IsDatabaseRole: true,
			GrantedTo:      "DATABASE_ROLE",
			GranteeName:    "MY_DB.WRITER",
		}
		assert.Equal(t, `GRANT DATABASE ROLE "MY_DB"."READER" TO DATABASE_ROLE "MY_DB"."WRITER"`, id.FullyQualifiedName())
	})

	t.Run("String_delegates_to_FQN", func(t *testing.T) {
		t.Parallel()
		id := RoleAssignmentIdentifier{
			RoleName:       "ANALYST",
			IsDatabaseRole: false,
			GrantedTo:      "ROLE",
			GranteeName:    "SYSADMIN",
		}
		assert.Equal(t, id.FullyQualifiedName(), id.String())
	})
}

// --------------------------------------------------------------------------
// matchesRoleAssignmentGrantee tests
// --------------------------------------------------------------------------

func TestMatchesRoleAssignmentGrantee(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		actual   string
		expected string
		want     bool
	}{
		{"ExactMatch", "SYSADMIN", "SYSADMIN", true},
		{"CaseInsensitive", "sysadmin", "SYSADMIN", true},
		{"Mismatch", "ANALYST", "SYSADMIN", false},
		{"FQN_vs_Unqualified", "MY_ROLE", "MY_DB.MY_ROLE", true},
		{"FQN_vs_FQN", "MY_DB.MY_ROLE", "MY_DB.MY_ROLE", true},
		{"FQN_vs_WrongUnqualified", "OTHER_ROLE", "MY_DB.MY_ROLE", false},
		{"Unqualified_vs_FQN", "MY_DB.MY_ROLE", "MY_ROLE", true},
		{"Unqualified_vs_FQN_Wrong", "MY_DB.MY_ROLE", "OTHER_ROLE", false},
		{"Empty_both", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchesRoleAssignmentGrantee(tt.actual, tt.expected)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --------------------------------------------------------------------------
// quoteRoleName and quoteTarget tests
// --------------------------------------------------------------------------

func TestQuoteRoleName(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole", func(t *testing.T) {
		t.Parallel()
		got := quoteRoleName("ANALYST", false)
		assert.Equal(t, `"ANALYST"`, got)
	})

	t.Run("DatabaseRole", func(t *testing.T) {
		t.Parallel()
		got := quoteRoleName("MY_DB.READER", true)
		assert.Equal(t, `"MY_DB"."READER"`, got)
	})
}

func TestQuoteTarget(t *testing.T) {
	t.Parallel()

	t.Run("ROLE", func(t *testing.T) {
		t.Parallel()
		got := quoteTarget("ROLE", "SYSADMIN")
		assert.Equal(t, `"SYSADMIN"`, got)
	})

	t.Run("USER", func(t *testing.T) {
		t.Parallel()
		got := quoteTarget("USER", "john")
		assert.Equal(t, `"john"`, got)
	})

	t.Run("DATABASE_ROLE", func(t *testing.T) {
		t.Parallel()
		got := quoteTarget("DATABASE_ROLE", "MY_DB.WRITER")
		assert.Equal(t, `"MY_DB"."WRITER"`, got)
	})
}
