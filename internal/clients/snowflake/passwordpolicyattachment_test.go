package snowflake

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildSetPasswordPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		opts := SetPasswordPolicyOptions{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_POLICY"`,
			TargetType: "ACCOUNT",
		}
		got := buildSetPasswordPolicySQL(opts)
		assert.Equal(t, `ALTER ACCOUNT SET PASSWORD POLICY "MY_DB"."MY_SCHEMA"."MY_POLICY"`, got)
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		opts := SetPasswordPolicyOptions{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_POLICY"`,
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		got := buildSetPasswordPolicySQL(opts)
		assert.Equal(t, `ALTER USER JOHN_DOE SET PASSWORD POLICY "MY_DB"."MY_SCHEMA"."MY_POLICY"`, got)
	})
}

func TestBuildUnsetPasswordPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		opts := UnsetPasswordPolicyOptions{
			TargetType: "ACCOUNT",
		}
		got := buildUnsetPasswordPolicySQL(opts)
		assert.Equal(t, `ALTER ACCOUNT UNSET PASSWORD POLICY`, got)
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		opts := UnsetPasswordPolicyOptions{
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		got := buildUnsetPasswordPolicySQL(opts)
		assert.Equal(t, `ALTER USER JOHN_DOE UNSET PASSWORD POLICY`, got)
	})
}

func TestBuildPolicyReferencesSQL(t *testing.T) {
	t.Parallel()

	t.Run("UserDomain", func(t *testing.T) {
		t.Parallel()
		got := buildPolicyReferencesSQL("JOHN_DOE", "USER")
		assert.Equal(t, `SELECT POLICY_DB, POLICY_SCHEMA, POLICY_NAME, POLICY_KIND FROM TABLE(SNOWFLAKE.INFORMATION_SCHEMA.POLICY_REFERENCES(REF_ENTITY_NAME => 'JOHN_DOE', REF_ENTITY_DOMAIN => 'USER'))`, got)
	})

	t.Run("AccountDomain", func(t *testing.T) {
		t.Parallel()
		got := buildPolicyReferencesSQL("", "ACCOUNT")
		assert.Equal(t, `SELECT POLICY_DB, POLICY_SCHEMA, POLICY_NAME, POLICY_KIND FROM TABLE(SNOWFLAKE.INFORMATION_SCHEMA.POLICY_REFERENCES(REF_ENTITY_NAME => '', REF_ENTITY_DOMAIN => 'ACCOUNT'))`, got)
	})

	t.Run("EscapesSingleQuotes", func(t *testing.T) {
		t.Parallel()
		got := buildPolicyReferencesSQL("O'BRIEN", "USER")
		assert.Contains(t, got, "O''BRIEN")
	})
}

func TestPasswordPolicyAttachmentIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	t.Run("AccountTarget", func(t *testing.T) {
		t.Parallel()
		id := PasswordPolicyAttachmentIdentifier{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_POLICY"`,
			TargetType: "ACCOUNT",
		}
		assert.Equal(t, `PASSWORD_POLICY "MY_DB"."MY_SCHEMA"."MY_POLICY" ON ACCOUNT`, id.FullyQualifiedName())
	})

	t.Run("UserTarget", func(t *testing.T) {
		t.Parallel()
		id := PasswordPolicyAttachmentIdentifier{
			PolicyName: `"MY_DB"."MY_SCHEMA"."MY_POLICY"`,
			TargetType: "USER",
			TargetName: "JOHN_DOE",
		}
		assert.Equal(t, `PASSWORD_POLICY "MY_DB"."MY_SCHEMA"."MY_POLICY" ON USER JOHN_DOE`, id.FullyQualifiedName())
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestSetPasswordPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_Account", func(t *testing.T) {
		t.Parallel()
		opts := &SetPasswordPolicyOptions{PolicyName: `"DB"."S"."P"`, TargetType: "ACCOUNT"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_User", func(t *testing.T) {
		t.Parallel()
		opts := &SetPasswordPolicyOptions{PolicyName: `"DB"."S"."P"`, TargetType: "USER", TargetName: "JOHN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingPolicyName", func(t *testing.T) {
		t.Parallel()
		opts := &SetPasswordPolicyOptions{TargetType: "ACCOUNT"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingTargetType", func(t *testing.T) {
		t.Parallel()
		opts := &SetPasswordPolicyOptions{PolicyName: `"DB"."S"."P"`}
		require.Error(t, opts.Validate())
	})

	t.Run("UserWithoutName", func(t *testing.T) {
		t.Parallel()
		opts := &SetPasswordPolicyOptions{PolicyName: `"DB"."S"."P"`, TargetType: "USER"}
		require.Error(t, opts.Validate())
	})
}

func TestUnsetPasswordPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_Account", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetPasswordPolicyOptions{TargetType: "ACCOUNT"}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_User", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetPasswordPolicyOptions{TargetType: "USER", TargetName: "JOHN"}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingTargetType", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetPasswordPolicyOptions{}
		require.Error(t, opts.Validate())
	})

	t.Run("UserWithoutName", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetPasswordPolicyOptions{TargetType: "USER"}
		require.Error(t, opts.Validate())
	})
}

func TestIsSQLCompilationError(t *testing.T) {
	t.Parallel()

	t.Run("MatchingError", func(t *testing.T) {
		t.Parallel()
		assert.True(t, IsSQLCompilationError(fmt.Errorf("SQL compilation error: something went wrong")))
	})

	t.Run("NonMatchingError", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsSQLCompilationError(assert.AnError))
	})

	t.Run("NilError", func(t *testing.T) {
		t.Parallel()
		assert.False(t, IsSQLCompilationError(nil))
	})
}
