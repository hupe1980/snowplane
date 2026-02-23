package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateDatabaseRoleSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("MY_DB", "DATA_ENGINEER"),
		}
		got := buildCreateDatabaseRoleSQL(opts)
		assert.Equal(t, `CREATE DATABASE ROLE IF NOT EXISTS "MY_DB"."DATA_ENGINEER"`, got)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "Engineering team role"
		opts := CreateDatabaseRoleOptions{
			Name:    NewDatabaseObjectIdentifier("MY_DB", "DATA_ENGINEER"),
			Comment: &comment,
		}
		got := buildCreateDatabaseRoleSQL(opts)
		assert.Equal(t, `CREATE DATABASE ROLE IF NOT EXISTS "MY_DB"."DATA_ENGINEER" COMMENT = 'Engineering team role'`, got)
	})

	t.Run("CommentWithQuotes", func(t *testing.T) {
		t.Parallel()
		comment := "Bob's role"
		opts := CreateDatabaseRoleOptions{
			Name:    NewDatabaseObjectIdentifier("MY_DB", "BOBS_ROLE"),
			Comment: &comment,
		}
		got := buildCreateDatabaseRoleSQL(opts)
		assert.Equal(t, `CREATE DATABASE ROLE IF NOT EXISTS "MY_DB"."BOBS_ROLE" COMMENT = 'Bob''s role'`, got)
	})
}

func TestBuildAlterDatabaseRoleStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
		}
		stmts, err := buildAlterDatabaseRoleStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterDatabaseRoleOptions{
			Name:    NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
			Comment: &comment,
		}
		stmts, err := buildAlterDatabaseRoleStatements(opts)
		require.NoError(t, err)
		assert.Equal(t, []string{
			`ALTER DATABASE ROLE "MY_DB"."MY_ROLE" SET COMMENT = 'updated'`,
		}, stmts)
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name:        NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterDatabaseRoleStatements(opts)
		require.NoError(t, err)
		assert.Equal(t, []string{
			`ALTER DATABASE ROLE "MY_DB"."MY_ROLE" UNSET COMMENT`,
		}, stmts)
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		comment := "new comment"
		opts := AlterDatabaseRoleOptions{
			Name:        NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
			Comment:     &comment,
			UnsetFields: []string{"SOME_OTHER_FIELD"},
		}
		stmts, err := buildAlterDatabaseRoleStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2)
		assert.Equal(t, `ALTER DATABASE ROLE "MY_DB"."MY_ROLE" SET COMMENT = 'new comment'`, stmts[0])
		assert.Equal(t, `ALTER DATABASE ROLE "MY_DB"."MY_ROLE" UNSET SOME_OTHER_FIELD`, stmts[1])
	})
}

func TestBuildDropDatabaseRoleSQL(t *testing.T) {
	t.Parallel()

	name := NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE")
	got := buildDropDatabaseRoleSQL(name)
	assert.Equal(t, `DROP DATABASE ROLE IF EXISTS "MY_DB"."MY_ROLE"`, got)
}

func TestBuildShowDatabaseRoleByIDSQL(t *testing.T) {
	t.Parallel()

	name := NewDatabaseObjectIdentifier("MY_DB", "DATA_ENGINEER")
	got := buildShowDatabaseRoleByIDSQL(name)
	assert.Equal(t, `SHOW DATABASE ROLES LIKE 'DATA\_ENGINEER' IN DATABASE "MY_DB"`, got)
}

func TestCreateDatabaseRoleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("ValidName", func(t *testing.T) {
		t.Parallel()
		opts := CreateDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("", ""),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingDatabaseName", func(t *testing.T) {
		t.Parallel()
		opts := CreateDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("", "MY_ROLE"),
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterDatabaseRoleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("", ""),
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterDatabaseRoleOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name: NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("HasComment", func(t *testing.T) {
		t.Parallel()
		comment := "hello"
		opts := AlterDatabaseRoleOptions{
			Name:    NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
			Comment: &comment,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterDatabaseRoleOptions{
			Name:        NewDatabaseObjectIdentifier("MY_DB", "MY_ROLE"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
