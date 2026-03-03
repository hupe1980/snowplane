package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateAuthenticationPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicMinimal", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_AUTH_POLICY"),
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE AUTHENTICATION POLICY IF NOT EXISTS "DB"."SCH"."MY_AUTH_POLICY"`)
	})

	t.Run("WithAuthenticationMethods", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name:                  NewSchemaObjectIdentifier("DB", "SCH", "P"),
			AuthenticationMethods: []string{"PASSWORD", "SAML"},
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "AUTHENTICATION_METHODS = (PASSWORD, SAML)")
	})

	t.Run("WithClientTypes", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ClientTypes: []string{"SNOWFLAKE_UI", "DRIVERS"},
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "CLIENT_TYPES = (SNOWFLAKE_UI, DRIVERS)")
	})

	t.Run("WithSecurityIntegrations", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name:                 NewSchemaObjectIdentifier("DB", "SCH", "P"),
			SecurityIntegrations: []string{"MY_INT1", "MY_INT2"},
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "SECURITY_INTEGRATIONS = ('MY_INT1', 'MY_INT2')")
	})

	t.Run("WithMfaSubPolicy", func(t *testing.T) {
		t.Parallel()
		enf := "REQUIRED"
		opts := CreateAuthenticationPolicyOptions{
			Name:                        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			MfaAllowedMethods:           []string{"TOTP"},
			MfaEnforceMfaOnExternalAuth: &enf,
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "MFA_AUTHENTICATION_METHODS = (TOTP)")
		assert.Contains(t, got, "ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION = REQUIRED")
	})

	t.Run("WithPatSubPolicy", func(t *testing.T) {
		t.Parallel()
		networkEval := "REQUIRED"
		opts := CreateAuthenticationPolicyOptions{
			Name:                       NewSchemaObjectIdentifier("DB", "SCH", "P"),
			PatDefaultExpiryInDays:     ptr(int32(30)),
			PatMaxExpiryInDays:         ptr(int32(90)),
			PatNetworkPolicyEvaluation: &networkEval,
			PatRequireRoleRestriction:  ptr(true),
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "PAT_DEFAULT_EXPIRY_IN_DAYS = 30")
		assert.Contains(t, got, "PAT_MAX_EXPIRY_IN_DAYS = 90")
		assert.Contains(t, got, "PAT_NETWORK_POLICY_EVALUATION = REQUIRED")
		assert.Contains(t, got, "PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS = TRUE")
	})

	t.Run("WithWorkloadIdentitySubPolicy", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name:                                NewSchemaObjectIdentifier("DB", "SCH", "P"),
			WorkloadIdentityAllowedProviders:    []string{"AWS", "AZURE"},
			WorkloadIdentityAllowedAwsAccounts:  []string{"123456789012"},
			WorkloadIdentityAllowedAzureIssuers: []string{"https://sts.windows.net/tenant"},
			WorkloadIdentityAllowedOidcIssuers:  []string{"https://accounts.google.com"},
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "WORKLOAD_IDENTITY_ALLOWED_PROVIDERS = (AWS, AZURE)")
		assert.Contains(t, got, "WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS = ('123456789012')")
		assert.Contains(t, got, "WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS = ('https://sts.windows.net/tenant')")
		assert.Contains(t, got, "WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS = ('https://accounts.google.com')")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "my auth policy"
		opts := CreateAuthenticationPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'my auth policy'")
	})

	t.Run("WithMfaEnrollment", func(t *testing.T) {
		t.Parallel()
		enrollment := "REQUIRED"
		opts := CreateAuthenticationPolicyOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			MfaEnrollment: &enrollment,
		}
		got, err := buildCreateAuthenticationPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "MFA_ENROLLMENT = REQUIRED")
	})
}

func TestBuildCreateAuthenticationPolicySQL_CreateOrAlter(t *testing.T) {
	t.Parallel()

	opts := CreateAuthenticationPolicyOptions{
		Name:                  NewSchemaObjectIdentifier("DB", "SCH", "MY_AUTH_POLICY"),
		AuthenticationMethods: []string{"PASSWORD"},
		UseCreateOrAlter:      true,
	}
	got, err := buildCreateAuthenticationPolicySQL(opts)
	require.NoError(t, err)
	assert.Contains(t, got, `CREATE OR ALTER AUTHENTICATION POLICY "DB"."SCH"."MY_AUTH_POLICY"`)
	assert.NotContains(t, got, "IF NOT EXISTS")
	assert.Contains(t, got, "AUTHENTICATION_METHODS = (PASSWORD)")
}

func TestBuildAlterAuthenticationPolicyStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetAuthenticationMethods", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:                  NewSchemaObjectIdentifier("DB", "SCH", "P"),
			AuthenticationMethods: []string{"PASSWORD", "SAML"},
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "AUTHENTICATION_METHODS = (PASSWORD, SAML)")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterAuthenticationPolicyOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &comment,
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("SetPatFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:                      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			PatDefaultExpiryInDays:    ptr(int32(30)),
			PatRequireRoleRestriction: ptr(true),
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "PAT_DEFAULT_EXPIRY_IN_DAYS = 30")
		assert.Contains(t, stmts[0], "PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS = TRUE")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:                  NewSchemaObjectIdentifier("DB", "SCH", "P"),
			AuthenticationMethods: []string{"PASSWORD"},
			UnsetFields:           []string{"COMMENT"},
		}
		stmts, err := buildAlterAuthenticationPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[0], "AUTHENTICATION_METHODS = (PASSWORD)")
		assert.Contains(t, stmts[1], "UNSET COMMENT")
	})
}

func TestBuildShowAuthenticationPolicyByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowAuthenticationPolicyByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_POLICY"))
	assert.Contains(t, got, "SHOW AUTHENTICATION POLICIES LIKE")
	assert.Contains(t, got, "MY\\_POLICY")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestCreateAuthenticationPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateAuthenticationPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterAuthenticationPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterAuthenticationPolicyOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithAuthenticationMethods", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:                  NewSchemaObjectIdentifier("DB", "SCH", "P"),
			AuthenticationMethods: []string{"PASSWORD"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterAuthenticationPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithMfaEnrollment", func(t *testing.T) {
		t.Parallel()
		e := "REQUIRED"
		opts := AlterAuthenticationPolicyOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), MfaEnrollment: &e}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithWorkloadIdentityProviders", func(t *testing.T) {
		t.Parallel()
		opts := AlterAuthenticationPolicyOptions{
			Name:                             NewSchemaObjectIdentifier("DB", "SCH", "P"),
			WorkloadIdentityAllowedProviders: []string{"AWS"},
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestBuildKeywordListClause(t *testing.T) {
	t.Parallel()

	t.Run("ValidKeywords", func(t *testing.T) {
		t.Parallel()
		got, err := sqlbuilder.BuildKeywordListClause("AUTH_METHODS", []string{"PASSWORD", "SAML"})
		require.NoError(t, err)
		assert.Equal(t, "AUTH_METHODS = (PASSWORD, SAML)", got)
	})

	t.Run("InjectionAttemptReturnsError", func(t *testing.T) {
		t.Parallel()
		_, err := sqlbuilder.BuildKeywordListClause("AUTH_METHODS", []string{"SAML); DROP TABLE x;--"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid keyword value character")
	})
}

func TestBuildEscapedListClause(t *testing.T) {
	t.Parallel()

	t.Run("ValidValues", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("INTEGRATIONS", []string{"INT1", "INT2"})
		assert.Equal(t, "INTEGRATIONS = ('INT1', 'INT2')", got)
	})

	t.Run("EscapesSingleQuotes", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("INTEGRATIONS", []string{"INT'1"})
		assert.Equal(t, "INTEGRATIONS = ('INT''1')", got)
	})
}
