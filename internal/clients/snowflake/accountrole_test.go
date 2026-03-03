package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateAccountRoleSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateAccountRoleOptions{
			Name: NewAccountObjectIdentifier("DATA_ENGINEER"),
		}
		got, err := buildCreateAccountRoleSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE ROLE IF NOT EXISTS "DATA_ENGINEER"`, got)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "Engineering team role"
		opts := CreateAccountRoleOptions{
			Name:    NewAccountObjectIdentifier("DATA_ENGINEER"),
			Comment: &comment,
		}
		got, err := buildCreateAccountRoleSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE ROLE IF NOT EXISTS "DATA_ENGINEER" COMMENT = 'Engineering team role'`, got)
	})

	t.Run("CommentWithQuotes", func(t *testing.T) {
		t.Parallel()
		comment := "Bob's role"
		opts := CreateAccountRoleOptions{
			Name:    NewAccountObjectIdentifier("BOBS_ROLE"),
			Comment: &comment,
		}
		got, err := buildCreateAccountRoleSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE ROLE IF NOT EXISTS "BOBS_ROLE" COMMENT = 'Bob''s role'`, got)
	})
}

func TestBuildAlterAccountRoleStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{
			Name: NewAccountObjectIdentifier("MY_ROLE"),
		}
		stmts, err := buildAlterAccountRoleStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated comment"
		opts := AlterAccountRoleOptions{
			Name:    NewAccountObjectIdentifier("MY_ROLE"),
			Comment: &comment,
		}
		stmts, err := buildAlterAccountRoleStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER ROLE "MY_ROLE" SET COMMENT = 'updated comment'`, stmts[0])
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{
			Name:        NewAccountObjectIdentifier("MY_ROLE"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterAccountRoleStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER ROLE "MY_ROLE" UNSET COMMENT`, stmts[0])
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		comment := "new"
		opts := AlterAccountRoleOptions{
			Name:        NewAccountObjectIdentifier("MY_ROLE"),
			Comment:     &comment,
			UnsetFields: []string{"SOMETHING"},
		}
		stmts, err := buildAlterAccountRoleStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2)
		assert.Equal(t, `ALTER ROLE "MY_ROLE" SET COMMENT = 'new'`, stmts[0])
		assert.Equal(t, `ALTER ROLE "MY_ROLE" UNSET SOMETHING`, stmts[1])
	})
}

func TestBuildDropAccountRoleSQL(t *testing.T) {
	t.Parallel()

	got := buildDropAccountRoleSQL(NewAccountObjectIdentifier("OLD_ROLE"))
	assert.Equal(t, `DROP ROLE IF EXISTS "OLD_ROLE"`, got)
}

func TestBuildShowAccountRoleByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowAccountRoleByIDSQL(NewAccountObjectIdentifier("MY_ROLE"))
	assert.Equal(t, `SHOW ROLES LIKE 'MY\_ROLE'`, got)
}

func TestBuildShowAccountRoleByIDSQL_SpecialChars(t *testing.T) {
	t.Parallel()

	got := buildShowAccountRoleByIDSQL(NewAccountObjectIdentifier("MY%ROLE"))
	assert.Equal(t, `SHOW ROLES LIKE 'MY\%ROLE'`, got)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateAccountRoleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateAccountRoleOptions{Name: NewAccountObjectIdentifier("MY_ROLE")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateAccountRoleOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterAccountRoleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{Name: NewAccountObjectIdentifier("MY_ROLE")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterAccountRoleOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{Name: NewAccountObjectIdentifier("R")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterAccountRoleOptions{Name: NewAccountObjectIdentifier("R"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAccountRoleOptions{Name: NewAccountObjectIdentifier("R"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}
