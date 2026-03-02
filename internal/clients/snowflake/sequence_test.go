package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateSequenceSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateSequenceOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
		}
		got := buildCreateSequenceSQL(opts)
		assert.Contains(t, got, `CREATE SEQUENCE IF NOT EXISTS "DB"."SCH"."MY_SEQ"`)
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		start := int64(100)
		increment := int64(5)
		ordering := "ORDER"
		comment := "my sequence"
		opts := CreateSequenceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			Start:     &start,
			Increment: &increment,
			Ordering:  &ordering,
			Comment:   &comment,
		}
		got := buildCreateSequenceSQL(opts)
		assert.Contains(t, got, `CREATE SEQUENCE IF NOT EXISTS "DB"."SCH"."MY_SEQ"`)
		assert.Contains(t, got, "START = 100")
		assert.Contains(t, got, "INCREMENT = 5")
		assert.Contains(t, got, "ORDER")
		assert.Contains(t, got, "COMMENT = 'my sequence'")
	})

	t.Run("CreateOrAlter", func(t *testing.T) {
		t.Parallel()
		opts := CreateSequenceOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			UseCreateOrAlter: true,
		}
		got := buildCreateSequenceSQL(opts)
		assert.Contains(t, got, `CREATE OR ALTER SEQUENCE "DB"."SCH"."MY_SEQ"`)
		assert.NotContains(t, got, "IF NOT EXISTS")
	})

	t.Run("WithNoorder", func(t *testing.T) {
		t.Parallel()
		ordering := "NOORDER"
		opts := CreateSequenceOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			Ordering: &ordering,
		}
		got := buildCreateSequenceSQL(opts)
		assert.Contains(t, got, "NOORDER")
	})
}

func TestCreateSequenceOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSequenceOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "SEQ"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSequenceOptions{}
		require.Error(t, opts.Validate())
	})
}

func TestBuildAlterSequenceStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetIncrement", func(t *testing.T) {
		t.Parallel()
		inc := int64(10)
		opts := AlterSequenceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			Increment: &inc,
		}
		stmts, err := buildAlterSequenceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER SEQUENCE "DB"."SCH"."MY_SEQ" SET`)
		assert.Contains(t, stmts[0], "INCREMENT BY = 10")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "new comment"
		opts := AlterSequenceOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			Comment: &comment,
		}
		stmts, err := buildAlterSequenceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'new comment'")
	})

	t.Run("SetOrdering", func(t *testing.T) {
		t.Parallel()
		ordering := "ORDER"
		opts := AlterSequenceOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			Ordering: &ordering,
		}
		stmts, err := buildAlterSequenceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ORDER")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSequenceOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MY_SEQ"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterSequenceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestAlterSequenceOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterSequenceOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "SEQ"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSequenceOptions{}
		require.Error(t, opts.Validate())
	})
}

func TestAlterSequenceOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterSequenceOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "SEQ"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("IncrementChange", func(t *testing.T) {
		t.Parallel()
		inc := int64(5)
		opts := AlterSequenceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SEQ"),
			Increment: &inc,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetChange", func(t *testing.T) {
		t.Parallel()
		opts := AlterSequenceOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "SEQ"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
