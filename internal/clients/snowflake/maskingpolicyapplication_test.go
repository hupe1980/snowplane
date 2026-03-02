package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildSetMaskingPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicPolicy", func(t *testing.T) {
		t.Parallel()
		opts := SetMaskingPolicyOptions{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY"`,
			TableName:  `"MY_DB"."MY_SCHEMA"."MY_TABLE"`,
			ColumnName: "EMAIL",
		}
		got := buildSetMaskingPolicySQL(opts)
		assert.Equal(t, `ALTER TABLE "MY_DB"."MY_SCHEMA"."MY_TABLE" ALTER COLUMN EMAIL SET MASKING POLICY "MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY"`, got)
	})

	t.Run("WithUsingColumns", func(t *testing.T) {
		t.Parallel()
		opts := SetMaskingPolicyOptions{
			PolicyName:   `"MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY"`,
			TableName:    `"MY_DB"."MY_SCHEMA"."MY_TABLE"`,
			ColumnName:   "EMAIL",
			UsingColumns: []string{"EMAIL", "ROLE_NAME"},
		}
		got := buildSetMaskingPolicySQL(opts)
		assert.Equal(t, `ALTER TABLE "MY_DB"."MY_SCHEMA"."MY_TABLE" ALTER COLUMN EMAIL SET MASKING POLICY "MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY" USING (EMAIL, ROLE_NAME)`, got)
	})
}

func TestBuildUnsetMaskingPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := UnsetMaskingPolicyOptions{
			TableName:  `"MY_DB"."MY_SCHEMA"."MY_TABLE"`,
			ColumnName: "EMAIL",
		}
		got := buildUnsetMaskingPolicySQL(opts)
		assert.Equal(t, `ALTER TABLE "MY_DB"."MY_SCHEMA"."MY_TABLE" ALTER COLUMN EMAIL UNSET MASKING POLICY`, got)
	})
}

func TestBuildMaskingPolicyReferencesSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicTable", func(t *testing.T) {
		t.Parallel()
		got := buildMaskingPolicyReferencesSQL(`"MY_DB"."MY_SCHEMA"."MY_TABLE"`)
		expected := `SELECT POLICY_DB, POLICY_SCHEMA, POLICY_NAME, POLICY_KIND, REF_COLUMN_NAME FROM TABLE(SNOWFLAKE.INFORMATION_SCHEMA.POLICY_REFERENCES(REF_ENTITY_NAME => '"MY_DB"."MY_SCHEMA"."MY_TABLE"', REF_ENTITY_DOMAIN => 'TABLE'))`
		assert.Equal(t, expected, got)
	})

	t.Run("EscapesSingleQuotes", func(t *testing.T) {
		t.Parallel()
		got := buildMaskingPolicyReferencesSQL("DB.SCHEMA.TABLE'S")
		assert.Contains(t, got, "TABLE''S")
	})
}

func TestMaskingPolicyApplicationIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		id := MaskingPolicyApplicationIdentifier{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY"`,
			TableName:  `"MY_DB"."MY_SCHEMA"."MY_TABLE"`,
			ColumnName: "EMAIL",
		}
		assert.Equal(t, `MASKING_POLICY "MY_DB"."MY_SCHEMA"."MY_MASKING_POLICY" ON "MY_DB"."MY_SCHEMA"."MY_TABLE".EMAIL`, id.FullyQualifiedName())
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestSetMaskingPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_Basic", func(t *testing.T) {
		t.Parallel()
		opts := &SetMaskingPolicyOptions{
			PolicyName: `"DB"."S"."P"`,
			TableName:  `"DB"."S"."T"`,
			ColumnName: "COL",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_WithUsing", func(t *testing.T) {
		t.Parallel()
		opts := &SetMaskingPolicyOptions{
			PolicyName:   `"DB"."S"."P"`,
			TableName:    `"DB"."S"."T"`,
			ColumnName:   "COL",
			UsingColumns: []string{"COL", "ROLE"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingPolicyName", func(t *testing.T) {
		t.Parallel()
		opts := &SetMaskingPolicyOptions{TableName: `"DB"."S"."T"`, ColumnName: "COL"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingTableName", func(t *testing.T) {
		t.Parallel()
		opts := &SetMaskingPolicyOptions{PolicyName: `"DB"."S"."P"`, ColumnName: "COL"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingColumnName", func(t *testing.T) {
		t.Parallel()
		opts := &SetMaskingPolicyOptions{PolicyName: `"DB"."S"."P"`, TableName: `"DB"."S"."T"`}
		require.Error(t, opts.Validate())
	})
}

func TestUnsetMaskingPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetMaskingPolicyOptions{TableName: `"DB"."S"."T"`, ColumnName: "COL"}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingTableName", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetMaskingPolicyOptions{ColumnName: "COL"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingColumnName", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetMaskingPolicyOptions{TableName: `"DB"."S"."T"`}
		require.Error(t, opts.Validate())
	})
}
