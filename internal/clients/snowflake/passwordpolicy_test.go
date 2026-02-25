package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreatePasswordPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicMinimal", func(t *testing.T) {
		t.Parallel()
		opts := CreatePasswordPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_POLICY"),
		}
		got := buildCreatePasswordPolicySQL(opts)
		assert.Contains(t, got, `CREATE PASSWORD POLICY IF NOT EXISTS "DB"."SCH"."MY_POLICY"`)
	})

	t.Run("WithAllFields", func(t *testing.T) {
		t.Parallel()
		comment := "test password policy"
		opts := CreatePasswordPolicyOptions{
			Name:                    NewSchemaObjectIdentifier("DB", "SCH", "FULL_POLICY"),
			PasswordMinLength:       int32Ptr(10),
			PasswordMaxLength:       int32Ptr(256),
			PasswordMinUpperCase:    int32Ptr(2),
			PasswordMinLowerCase:    int32Ptr(2),
			PasswordMinNumeric:      int32Ptr(1),
			PasswordMinSpecial:      int32Ptr(1),
			PasswordMinAgeDays:      int32Ptr(1),
			PasswordMaxAgeDays:      int32Ptr(90),
			PasswordMaxRetries:      int32Ptr(5),
			PasswordLockoutTimeMins: int32Ptr(30),
			PasswordHistory:         int32Ptr(10),
			Comment:                 &comment,
		}
		got := buildCreatePasswordPolicySQL(opts)
		assert.Contains(t, got, "PASSWORD_MIN_LENGTH = 10")
		assert.Contains(t, got, "PASSWORD_MAX_LENGTH = 256")
		assert.Contains(t, got, "PASSWORD_MIN_UPPER_CASE_CHARS = 2")
		assert.Contains(t, got, "PASSWORD_MIN_LOWER_CASE_CHARS = 2")
		assert.Contains(t, got, "PASSWORD_MIN_NUMERIC_CHARS = 1")
		assert.Contains(t, got, "PASSWORD_MIN_SPECIAL_CHARS = 1")
		assert.Contains(t, got, "PASSWORD_MIN_AGE_DAYS = 1")
		assert.Contains(t, got, "PASSWORD_MAX_AGE_DAYS = 90")
		assert.Contains(t, got, "PASSWORD_MAX_RETRIES = 5")
		assert.Contains(t, got, "PASSWORD_LOCKOUT_TIME_MINS = 30")
		assert.Contains(t, got, "PASSWORD_HISTORY = 10")
		assert.Contains(t, got, "COMMENT = 'test password policy'")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "my policy"
		opts := CreatePasswordPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		got := buildCreatePasswordPolicySQL(opts)
		assert.Contains(t, got, "COMMENT = 'my policy'")
	})
}

func TestBuildAlterPasswordPolicyStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		stmts, err := buildAlterPasswordPolicyStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetMinLength", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "P"),
			PasswordMinLength: int32Ptr(12),
		}
		stmts, err := buildAlterPasswordPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "PASSWORD_MIN_LENGTH = 12")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterPasswordPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		stmts, err := buildAlterPasswordPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterPasswordPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("SetMultipleFields", func(t *testing.T) {
		t.Parallel()
		comment := "c"
		opts := AlterPasswordPolicyOptions{
			Name:               NewSchemaObjectIdentifier("DB", "SCH", "P"),
			PasswordMinLength:  int32Ptr(10),
			PasswordMaxRetries: int32Ptr(3),
			Comment:            &comment,
		}
		stmts, err := buildAlterPasswordPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[0], "PASSWORD_MIN_LENGTH = 10")
		assert.Contains(t, stmts[0], "PASSWORD_MAX_RETRIES = 3")
		assert.Contains(t, stmts[0], "COMMENT = 'c'")
	})
}

func TestBuildShowPasswordPolicyByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowPasswordPolicyByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_POLICY"))
	assert.Contains(t, got, "SHOW PASSWORD POLICIES LIKE")
	assert.Contains(t, got, "MY\\_POLICY")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestCreatePasswordPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreatePasswordPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreatePasswordPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterPasswordPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterPasswordPolicyOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithMinLength", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), PasswordMinLength: int32Ptr(10)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterPasswordPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterPasswordPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}
