package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateTagSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicTag", func(t *testing.T) {
		t.Parallel()
		opts := CreateTagOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_TAG"),
		}
		got := buildCreateTagSQL(opts)
		assert.Equal(t, `CREATE OR ALTER TAG "DB"."SCH"."MY_TAG"`, got)
	})

	t.Run("WithAllowedValues", func(t *testing.T) {
		t.Parallel()
		opts := CreateTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "ENV_TAG"),
			AllowedValues: []string{"dev", "staging", "prod"},
		}
		got := buildCreateTagSQL(opts)
		assert.Contains(t, got, "ALLOWED_VALUES 'dev', 'staging', 'prod'")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "environment tag"
		opts := CreateTagOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Comment: &comment,
		}
		got := buildCreateTagSQL(opts)
		assert.Contains(t, got, "COMMENT = 'environment tag'")
	})

	t.Run("AllowedValuesWithQuotes", func(t *testing.T) {
		t.Parallel()
		opts := CreateTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "T"),
			AllowedValues: []string{"it's", "fine"},
		}
		got := buildCreateTagSQL(opts)
		assert.Contains(t, got, "ALLOWED_VALUES 'it''s', 'fine'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		comment := "full tag"
		opts := CreateTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "FULL_TAG"),
			AllowedValues: []string{"a", "b"},
			Comment:       &comment,
		}
		got := buildCreateTagSQL(opts)
		assert.Contains(t, got, "CREATE OR ALTER TAG")
		assert.Contains(t, got, "ALLOWED_VALUES 'a', 'b'")
		assert.Contains(t, got, "COMMENT = 'full tag'")
	})
}

func TestBuildAlterTagStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "T"),
		}
		stmts, err := buildAlterTagStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("UnsetAllowedValues", func(t *testing.T) {
		t.Parallel()
		empty := []string{}
		opts := AlterTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "T"),
			AllowedValues: &empty,
		}
		stmts, err := buildAlterTagStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET ALLOWED_VALUES")
	})

	t.Run("SetAllowedValues", func(t *testing.T) {
		t.Parallel()
		vals := []string{"x", "y"}
		opts := AlterTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "T"),
			AllowedValues: &vals,
		}
		stmts, err := buildAlterTagStatements(opts)
		require.NoError(t, err)
		// First unset, then set (idempotent)
		assert.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "UNSET ALLOWED_VALUES")
		assert.Contains(t, stmts[1], "SET ALLOWED_VALUES 'x', 'y'")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterTagOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Comment: &comment,
		}
		stmts, err := buildAlterTagStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterTagStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestBuildDropTagSQL(t *testing.T) {
	t.Parallel()

	got := buildDropTagSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_TAG"))
	assert.Equal(t, `DROP TAG IF EXISTS "DB"."SCH"."MY_TAG"`, got)
}

func TestBuildShowTagByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowTagByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_TAG"))
	assert.Contains(t, got, "SHOW TAGS LIKE")
	assert.Contains(t, got, "MY\\_TAG")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateTagOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateTagOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "T")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateTagOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterTagOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "T")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterTagOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "T")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("AllowedValuesSet", func(t *testing.T) {
		t.Parallel()
		vals := []string{"a"}
		opts := AlterTagOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "T"),
			AllowedValues: &vals,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterTagOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterTagOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
