package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildRAPSignatureClause(t *testing.T) {
	t.Parallel()

	t.Run("SingleArg", func(t *testing.T) {
		t.Parallel()
		args := []RowAccessPolicyArgument{{Name: "user_id", Type: "VARCHAR"}}
		got := buildRAPSignatureClause(args)
		assert.Equal(t, "AS (user_id VARCHAR) RETURNS BOOLEAN", got)
	})

	t.Run("MultipleArgs", func(t *testing.T) {
		t.Parallel()
		args := []RowAccessPolicyArgument{
			{Name: "user_id", Type: "VARCHAR"},
			{Name: "region", Type: "VARCHAR"},
		}
		got := buildRAPSignatureClause(args)
		assert.Equal(t, "AS (user_id VARCHAR, region VARCHAR) RETURNS BOOLEAN", got)
	})
}

func TestBuildCreateRowAccessPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateRowAccessPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "RAP_REGION"),
			Signature: []RowAccessPolicyArgument{{Name: "region", Type: "VARCHAR"}},
			Body:      "current_role() IN ('ADMIN') OR region = current_region()",
		}
		got := buildCreateRowAccessPolicySQL(opts)
		assert.Contains(t, got, `CREATE ROW ACCESS POLICY IF NOT EXISTS "DB"."SCH"."RAP_REGION"`)
		assert.Contains(t, got, "AS (region VARCHAR) RETURNS BOOLEAN")
		assert.Contains(t, got, "-> current_role() IN ('ADMIN') OR region = current_region()")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "restrict by region"
		opts := CreateRowAccessPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []RowAccessPolicyArgument{{Name: "r", Type: "VARCHAR"}},
			Body:      "true",
			Comment:   &comment,
		}
		got := buildCreateRowAccessPolicySQL(opts)
		assert.Contains(t, got, "COMMENT = 'restrict by region'")
	})
}

func TestBuildCreateRowAccessPolicySQL_CreateOrAlter(t *testing.T) {
	t.Parallel()

	opts := CreateRowAccessPolicyOptions{
		Name:             NewSchemaObjectIdentifier("DB", "SCH", "RAP_REGION"),
		Signature:        []RowAccessPolicyArgument{{Name: "region", Type: "VARCHAR"}},
		Body:             "current_role() IN ('ADMIN')",
		UseCreateOrAlter: true,
	}
	got := buildCreateRowAccessPolicySQL(opts)
	assert.Contains(t, got, `CREATE OR ALTER ROW ACCESS POLICY "DB"."SCH"."RAP_REGION"`)
	assert.NotContains(t, got, "IF NOT EXISTS")
	assert.Contains(t, got, "AS (region VARCHAR) RETURNS BOOLEAN")
}

func TestBuildAlterRowAccessPolicyStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		stmts, err := buildAlterRowAccessPolicyStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetBody", func(t *testing.T) {
		t.Parallel()
		body := "current_role() = 'ADMIN'"
		opts := AlterRowAccessPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: &body,
		}
		stmts, err := buildAlterRowAccessPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER ROW ACCESS POLICY "DB"."SCH"."P" SET BODY -> current_role() = 'ADMIN'`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterRowAccessPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		stmts, err := buildAlterRowAccessPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterRowAccessPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("BodyAndComment", func(t *testing.T) {
		t.Parallel()
		body := "true"
		comment := "c"
		opts := AlterRowAccessPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body:    &body,
			Comment: &comment,
		}
		stmts, err := buildAlterRowAccessPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET BODY -> true")
		assert.Contains(t, stmts[1], "SET COMMENT = 'c'")
	})
}

func TestBuildShowRowAccessPolicyByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowRowAccessPolicyByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "RAP"))
	assert.Contains(t, got, "SHOW ROW ACCESS POLICIES LIKE")
	assert.Contains(t, got, "RAP")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateRowAccessPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateRowAccessPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []RowAccessPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:      "true",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateRowAccessPolicyOptions{
			Signature: []RowAccessPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:      "true",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("EmptySignature", func(t *testing.T) {
		t.Parallel()
		opts := CreateRowAccessPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: "true",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("EmptyBody", func(t *testing.T) {
		t.Parallel()
		opts := CreateRowAccessPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []RowAccessPolicyArgument{{Name: "v", Type: "VARCHAR"}},
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterRowAccessPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterRowAccessPolicyOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("BodySet", func(t *testing.T) {
		t.Parallel()
		b := "true"
		opts := AlterRowAccessPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: &b,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterRowAccessPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterRowAccessPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
