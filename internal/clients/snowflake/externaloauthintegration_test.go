package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateExternalOAuthIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalOAuthIntegrationOptions{
			Name:                          NewAccountObjectIdentifier("MY_OAUTH"),
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://dev-123.okta.com",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
		}
		got, err := buildCreateExternalOAuthIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECURITY INTEGRATION IF NOT EXISTS "MY_OAUTH"`)
		assert.Contains(t, got, "TYPE = EXTERNAL_OAUTH")
		assert.Contains(t, got, "EXTERNAL_OAUTH_TYPE = OKTA")
		assert.Contains(t, got, "EXTERNAL_OAUTH_ISSUER = 'https://dev-123.okta.com'")
		assert.Contains(t, got, "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'sub'")
		assert.Contains(t, got, "EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'LOGIN_NAME'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateExternalOAuthIntegrationOptions{
			Name:                          NewAccountObjectIdentifier("FULL_OAUTH"),
			ExternalOAuthType:             "AZURE",
			Issuer:                        "https://sts.windows.net/tenant-id/",
			TokenUserMappingClaim:         "upn",
			SnowflakeUserMappingAttribute: "EMAIL_ADDRESS",
			JWSKeysURL:                    ptr("https://login.microsoftonline.com/common/discovery/keys"),
			AudienceList:                  []string{"https://analysis.windows.net/powerbi/connector/Snowflake"},
			AllowedRoles:                  []string{"ANALYST", "DATA_ENG"},
			BlockedRoles:                  []string{"ACCOUNTADMIN", "SECURITYADMIN"},
			AnyRoleMode:                   ptr("DISABLE"),
			ScopeDelimiter:                ptr(" "),
			NetworkPolicy:                 ptr("my_policy"),
			Enabled:                       ptr(true),
			Comment:                       ptr("azure oauth"),
		}
		got, err := buildCreateExternalOAuthIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "EXTERNAL_OAUTH_TYPE = AZURE")
		assert.Contains(t, got, "EXTERNAL_OAUTH_JWS_KEYS_URL = 'https://login.microsoftonline.com/common/discovery/keys'")
		assert.Contains(t, got, `EXTERNAL_OAUTH_AUDIENCE_LIST = ('https://analysis.windows.net/powerbi/connector/Snowflake')`)
		assert.Contains(t, got, `EXTERNAL_OAUTH_ALLOWED_ROLES_LIST = ('ANALYST', 'DATA_ENG')`)
		assert.Contains(t, got, `EXTERNAL_OAUTH_BLOCKED_ROLES_LIST = ('ACCOUNTADMIN', 'SECURITYADMIN')`)
		assert.Contains(t, got, "EXTERNAL_OAUTH_ANY_ROLE_MODE = DISABLE")
		assert.Contains(t, got, "EXTERNAL_OAUTH_SCOPE_DELIMITER = ' '")
		assert.Contains(t, got, "NETWORK_POLICY = 'my_policy'")
		assert.Contains(t, got, "ENABLED = TRUE")
		assert.Contains(t, got, "COMMENT = 'azure oauth'")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				ExternalOAuthType:             "OKTA",
				Issuer:                        "https://issuer",
				TokenUserMappingClaim:         "sub",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("InvalidType", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				Name:                          NewAccountObjectIdentifier("BAD"),
				ExternalOAuthType:             "INVALID_TYPE",
				Issuer:                        "https://issuer",
				TokenUserMappingClaim:         "sub",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid external_oauth_type")
		})

		t.Run("MissingIssuer", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				Name:                          NewAccountObjectIdentifier("NO_ISSUER"),
				ExternalOAuthType:             "OKTA",
				TokenUserMappingClaim:         "sub",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "external_oauth_issuer is required")
		})

		t.Run("MissingMappingClaim", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				Name:                          NewAccountObjectIdentifier("NO_CLAIM"),
				ExternalOAuthType:             "OKTA",
				Issuer:                        "https://issuer",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "external_oauth_token_user_mapping_claim is required")
		})

		t.Run("MissingMappingAttribute", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				Name:                  NewAccountObjectIdentifier("NO_ATTR"),
				ExternalOAuthType:     "OKTA",
				Issuer:                "https://issuer",
				TokenUserMappingClaim: "sub",
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "external_oauth_snowflake_user_mapping_attribute is required")
		})

		t.Run("InvalidAnyRoleMode", func(t *testing.T) {
			t.Parallel()
			opts := CreateExternalOAuthIntegrationOptions{
				Name:                          NewAccountObjectIdentifier("BAD_ROLE"),
				ExternalOAuthType:             "CUSTOM",
				Issuer:                        "https://issuer",
				TokenUserMappingClaim:         "sub",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
				AnyRoleMode:                   ptr("INVALID_MODE"),
			}
			_, err := buildCreateExternalOAuthIntegrationSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid external_oauth_any_role_mode")
		})
	})
}

func TestBuildAlterExternalOAuthIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_OAUTH"),
			Enabled: ptr(true),
		}
		stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER SECURITY INTEGRATION "MY_OAUTH" SET`)
		assert.Contains(t, stmts[0], "ENABLED = TRUE")
	})

	t.Run("SetMultipleFields", func(t *testing.T) {
		t.Parallel()
		claim := "email"
		opts := AlterExternalOAuthIntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_OAUTH"),
			TokenUserMappingClaim: &claim,
			JWSKeysURL:            ptr("https://new-keys.example.com/.well-known/jwks"),
			AnyRoleMode:           ptr("ENABLE"),
			Comment:               ptr("updated comment"),
		}
		stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'email'")
		assert.Contains(t, stmts[0], "EXTERNAL_OAUTH_JWS_KEYS_URL = 'https://new-keys.example.com/.well-known/jwks'")
		assert.Contains(t, stmts[0], "EXTERNAL_OAUTH_ANY_ROLE_MODE = ENABLE")
		assert.Contains(t, stmts[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetListFields", func(t *testing.T) {
		t.Parallel()
		audience := []string{"aud1", "aud2"}
		allowed := []string{"ROLE_A"}
		blocked := []string{"ACCOUNTADMIN"}
		opts := AlterExternalOAuthIntegrationOptions{
			Name:         NewAccountObjectIdentifier("MY_OAUTH"),
			AudienceList: &audience,
			AllowedRoles: &allowed,
			BlockedRoles: &blocked,
		}
		stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `EXTERNAL_OAUTH_AUDIENCE_LIST = ('aud1', 'aud2')`)
		assert.Contains(t, stmts[0], `EXTERNAL_OAUTH_ALLOWED_ROLES_LIST = ('ROLE_A')`)
		assert.Contains(t, stmts[0], `EXTERNAL_OAUTH_BLOCKED_ROLES_LIST = ('ACCOUNTADMIN')`)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_OAUTH"),
			UnsetFields: []string{"COMMENT", "NETWORK_POLICY"},
		}
		stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ALTER SECURITY INTEGRATION "MY_OAUTH" UNSET`)
		assert.Contains(t, stmts[0], "COMMENT")
		assert.Contains(t, stmts[0], "NETWORK_POLICY")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_OAUTH"),
			Enabled:     ptr(false),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[0], "ENABLED = FALSE")
		assert.Contains(t, stmts[1], "UNSET")
		assert.Contains(t, stmts[1], "COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name: NewAccountObjectIdentifier("MY_OAUTH"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("ValidationInvalidAnyRoleMode", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_OAUTH"),
			AnyRoleMode: ptr("BAD_MODE"),
		}
		_, err := buildAlterExternalOAuthIntegrationStatements(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid external_oauth_any_role_mode")
	})
}

// --------------------------------------------------------------------------
// HasChanges tests
// --------------------------------------------------------------------------

func TestExternalOAuthAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoFieldsSet", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{Name: NewAccountObjectIdentifier("X")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("OnlyUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:        NewAccountObjectIdentifier("X"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("EnabledSet", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:    NewAccountObjectIdentifier("X"),
			Enabled: ptr(true),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ListFieldSet", func(t *testing.T) {
		t.Parallel()
		list := []string{"aud1"}
		opts := AlterExternalOAuthIntegrationOptions{
			Name:         NewAccountObjectIdentifier("X"),
			AudienceList: &list,
		}
		assert.True(t, opts.HasChanges())
	})
}

// --------------------------------------------------------------------------
// Validate tests
// --------------------------------------------------------------------------

func TestExternalOAuthCreateValidate(t *testing.T) {
	t.Parallel()

	validOpts := func() CreateExternalOAuthIntegrationOptions {
		return CreateExternalOAuthIntegrationOptions{
			Name:                          NewAccountObjectIdentifier("V"),
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://issuer",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
		}
	}

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := validOpts()
		assert.NoError(t, opts.Validate())
	})

	t.Run("AllTypes", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []string{"OKTA", "AZURE", "PING_FEDERATE", "CUSTOM"} {
			opts := validOpts()
			opts.ExternalOAuthType = typ
			assert.NoError(t, opts.Validate(), "type %s should be valid", typ)
		}
	})

	t.Run("AllAnyRoleModes", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"DISABLE", "ENABLE", "ENABLE_FOR_PRIVILEGE"} {
			opts := validOpts()
			opts.AnyRoleMode = ptr(mode)
			assert.NoError(t, opts.Validate(), "mode %s should be valid", mode)
		}
	})
}

func TestExternalOAuthAlterValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:    NewAccountObjectIdentifier("V"),
			Enabled: ptr(true),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Enabled: ptr(true),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidAnyRoleMode", func(t *testing.T) {
		t.Parallel()
		opts := AlterExternalOAuthIntegrationOptions{
			Name:        NewAccountObjectIdentifier("V"),
			AnyRoleMode: ptr("INVALID"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid external_oauth_any_role_mode")
	})
}
