package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildSignatureClause(t *testing.T) {
	t.Parallel()

	t.Run("SingleArg", func(t *testing.T) {
		t.Parallel()
		args := []MaskingPolicyArgument{{Name: "val", Type: "VARCHAR"}}
		got := buildSignatureClause(args)
		assert.Equal(t, "AS (val VARCHAR) RETURNS VARCHAR", got)
	})

	t.Run("MultipleArgs", func(t *testing.T) {
		t.Parallel()
		args := []MaskingPolicyArgument{
			{Name: "val", Type: "VARCHAR"},
			{Name: "role", Type: "VARCHAR"},
		}
		got := buildSignatureClause(args)
		assert.Equal(t, "AS (val VARCHAR, role VARCHAR) RETURNS VARCHAR", got)
	})

	t.Run("NumberType", func(t *testing.T) {
		t.Parallel()
		args := []MaskingPolicyArgument{{Name: "num", Type: "NUMBER"}}
		got := buildSignatureClause(args)
		assert.Equal(t, "AS (num NUMBER) RETURNS NUMBER", got)
	})
}

func TestBuildCreateMaskingPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MASK_EMAIL"),
			Signature: []MaskingPolicyArgument{{Name: "val", Type: "VARCHAR"}},
			Body:      "CASE WHEN current_role() IN ('ADMIN') THEN val ELSE '***' END",
		}
		got := buildCreateMaskingPolicySQL(opts)
		assert.Contains(t, got, `CREATE MASKING POLICY IF NOT EXISTS "DB"."SCH"."MASK_EMAIL"`)
		assert.Contains(t, got, "AS (val VARCHAR) RETURNS VARCHAR")
		assert.Contains(t, got, "-> CASE WHEN current_role() IN ('ADMIN') THEN val ELSE '***' END")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "masks emails"
		opts := CreateMaskingPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:      "v",
			Comment:   &comment,
		}
		got := buildCreateMaskingPolicySQL(opts)
		assert.Contains(t, got, "COMMENT = 'masks emails'")
	})

	t.Run("WithExemptOtherPolicies", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name:                NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature:           []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:                "v",
			ExemptOtherPolicies: boolPtr(true),
		}
		got := buildCreateMaskingPolicySQL(opts)
		assert.Contains(t, got, "EXEMPT_OTHER_POLICIES = TRUE")
	})

	t.Run("ExemptOtherPoliciesFalse", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name:                NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature:           []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:                "v",
			ExemptOtherPolicies: boolPtr(false),
		}
		got := buildCreateMaskingPolicySQL(opts)
		assert.NotContains(t, got, "EXEMPT_OTHER_POLICIES")
	})
}

func TestBuildAlterMaskingPolicyStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		stmts, err := buildAlterMaskingPolicyStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetBody", func(t *testing.T) {
		t.Parallel()
		body := "CASE WHEN true THEN val ELSE '***' END"
		opts := AlterMaskingPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: &body,
		}
		stmts, err := buildAlterMaskingPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER MASKING POLICY "DB"."SCH"."P" SET BODY -> CASE WHEN true THEN val ELSE '***' END`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterMaskingPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		stmts, err := buildAlterMaskingPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterMaskingPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("BodyAndComment", func(t *testing.T) {
		t.Parallel()
		body := "val"
		comment := "c"
		opts := AlterMaskingPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body:    &body,
			Comment: &comment,
		}
		stmts, err := buildAlterMaskingPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET BODY -> val")
		assert.Contains(t, stmts[1], "SET COMMENT = 'c'")
	})
}

func TestBuildShowMaskingPolicyByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowMaskingPolicyByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MASK"))
	assert.Contains(t, got, "SHOW MASKING POLICIES LIKE")
	assert.Contains(t, got, "MASK")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateMaskingPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:      "v",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Signature: []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
			Body:      "v",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("EmptySignature", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: "v",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("EmptyBody", func(t *testing.T) {
		t.Parallel()
		opts := CreateMaskingPolicyOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Signature: []MaskingPolicyArgument{{Name: "v", Type: "VARCHAR"}},
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterMaskingPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterMaskingPolicyOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("BodySet", func(t *testing.T) {
		t.Parallel()
		b := "val"
		opts := AlterMaskingPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Body: &b,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterMaskingPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterMaskingPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
