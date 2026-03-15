package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateOAuthCustomClientsSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateOAuthIntegrationForCustomClientsOptions{
			Name:             NewAccountObjectIdentifier("MY_OAUTH"),
			OAuthClientType:  "PUBLIC",
			OAuthRedirectURI: "https://example.com/callback",
		}
		got, err := buildCreateOAuthCustomClientsSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `"MY_OAUTH"`)
		assert.Contains(t, got, "TYPE = OAUTH")
		assert.Contains(t, got, "OAUTH_CLIENT = CUSTOM")
		assert.Contains(t, got, "OAUTH_CLIENT_TYPE = 'PUBLIC'")
		assert.Contains(t, got, "OAUTH_REDIRECT_URI = 'https://example.com/callback'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		validity := int64(86400)
		opts := CreateOAuthIntegrationForCustomClientsOptions{
			Name:                        NewAccountObjectIdentifier("FULL_OAUTH"),
			OAuthClientType:             "CONFIDENTIAL",
			OAuthRedirectURI:            "https://example.com/oauth",
			OAuthAllowNonTLSRedirectURI: ptr(true),
			OAuthEnforcePKCE:            ptr(true),
			OAuthUseSecondaryRoles:      ptr("IMPLICIT"),
			PreAuthorizedRolesList:      []string{"ROLE_A", "ROLE_B"},
			BlockedRolesList:            []string{"ACCOUNTADMIN"},
			OAuthIssueRefreshTokens:     ptr(true),
			OAuthRefreshTokenValidity:   &validity,
			NetworkPolicy:               ptr("my_policy"),
			OAuthClientRSAPublicKey:     ptr("MIIBI..."),
			OAuthClientRSAPublicKey2:    ptr("MIIBJ..."),
			Comment:                     ptr("full oauth"),
			Enabled:                     ptr(true),
		}
		got, err := buildCreateOAuthCustomClientsSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "OAUTH_CLIENT_TYPE = 'CONFIDENTIAL'")
		assert.Contains(t, got, "OAUTH_ALLOW_NON_TLS_REDIRECT_URI = TRUE")
		assert.Contains(t, got, "OAUTH_ENFORCE_PKCE = TRUE")
		assert.Contains(t, got, "OAUTH_USE_SECONDARY_ROLES = IMPLICIT")
		assert.Contains(t, got, "PRE_AUTHORIZED_ROLES_LIST")
		assert.Contains(t, got, "BLOCKED_ROLES_LIST")
		assert.Contains(t, got, "OAUTH_ISSUE_REFRESH_TOKENS = TRUE")
		assert.Contains(t, got, "OAUTH_REFRESH_TOKEN_VALIDITY = 86400")
		assert.Contains(t, got, "NETWORK_POLICY = 'my_policy'")
		assert.Contains(t, got, "OAUTH_CLIENT_RSA_PUBLIC_KEY = 'MIIBI...'")
		assert.Contains(t, got, "OAUTH_CLIENT_RSA_PUBLIC_KEY_2 = 'MIIBJ...'")
		assert.Contains(t, got, "COMMENT = 'full oauth'")
		assert.Contains(t, got, "ENABLED = TRUE")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForCustomClientsOptions{
				OAuthClientType:  "PUBLIC",
				OAuthRedirectURI: "https://example.com/callback",
			}
			_, err := buildCreateOAuthCustomClientsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("MissingClientType", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForCustomClientsOptions{
				Name:             NewAccountObjectIdentifier("X"),
				OAuthRedirectURI: "https://example.com",
			}
			_, err := buildCreateOAuthCustomClientsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_client_type is required")
		})

		t.Run("InvalidClientType", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForCustomClientsOptions{
				Name:             NewAccountObjectIdentifier("X"),
				OAuthClientType:  "INVALID",
				OAuthRedirectURI: "https://example.com",
			}
			_, err := buildCreateOAuthCustomClientsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_client_type")
		})

		t.Run("MissingRedirectURI", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForCustomClientsOptions{
				Name:            NewAccountObjectIdentifier("X"),
				OAuthClientType: "PUBLIC",
			}
			_, err := buildCreateOAuthCustomClientsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_redirect_uri is required")
		})

		t.Run("InvalidOAuthUseSecondaryRoles", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForCustomClientsOptions{
				Name:                   NewAccountObjectIdentifier("X"),
				OAuthClientType:        "PUBLIC",
				OAuthRedirectURI:       "https://example.com",
				OAuthUseSecondaryRoles: ptr("INVALID_MODE"),
			}
			_, err := buildCreateOAuthCustomClientsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_use_secondary_roles")
		})
	})
}

func TestBuildAlterOAuthCustomClientsStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:    NewAccountObjectIdentifier("MY_OAUTH"),
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:        NewAccountObjectIdentifier("MY_OAUTH"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()
		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := AlterOAuthIntegrationForCustomClientsOptions{}
			_, err := buildAlterOAuthCustomClientsStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("InvalidOAuthUseSecondaryRoles", func(t *testing.T) {
			t.Parallel()
			opts := AlterOAuthIntegrationForCustomClientsOptions{
				Name:                   NewAccountObjectIdentifier("MY_OAUTH"),
				OAuthUseSecondaryRoles: ptr("INJECT; DROP TABLE"),
			}
			_, err := buildAlterOAuthCustomClientsStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_use_secondary_roles")
		})
	})

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:    NewAccountObjectIdentifier("MY_OAUTH"),
			Enabled: ptr(true),
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "ENABLED = TRUE")
	})

	t.Run("SetBooleanFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:                        NewAccountObjectIdentifier("MY_OAUTH"),
			OAuthAllowNonTLSRedirectURI: ptr(true),
			OAuthEnforcePKCE:            ptr(false),
			OAuthIssueRefreshTokens:     ptr(true),
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		joined := stmts[0]
		assert.Contains(t, joined, "OAUTH_ALLOW_NON_TLS_REDIRECT_URI = TRUE")
		assert.Contains(t, joined, "OAUTH_ENFORCE_PKCE = FALSE")
		assert.Contains(t, joined, "OAUTH_ISSUE_REFRESH_TOKENS = TRUE")
	})

	t.Run("SetRefreshTokenValidity", func(t *testing.T) {
		t.Parallel()
		validity := int64(86400)
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:                      NewAccountObjectIdentifier("MY_OAUTH"),
			OAuthRefreshTokenValidity: &validity,
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "OAUTH_REFRESH_TOKEN_VALIDITY = 86400")
	})

	t.Run("SetListFields", func(t *testing.T) {
		t.Parallel()
		preAuth := []string{"ROLE_A", "ROLE_B"}
		blocked := []string{"ACCOUNTADMIN"}
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:                   NewAccountObjectIdentifier("MY_OAUTH"),
			PreAuthorizedRolesList: &preAuth,
			BlockedRolesList:       &blocked,
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "PRE_AUTHORIZED_ROLES_LIST")
		assert.Contains(t, stmts[0], "BLOCKED_ROLES_LIST")
	})

	t.Run("CommentInjectionEscaped", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:    NewAccountObjectIdentifier("MY_OAUTH"),
			Comment: ptr("test'; DROP TABLE users; --"),
		}
		stmts, err := buildAlterOAuthCustomClientsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "test''; DROP TABLE users; --")
		assert.NotContains(t, stmts[0], "test'; DROP TABLE")
	})
}

func TestOAuthCustomClientsAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{Name: NewAccountObjectIdentifier("C")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:    NewAccountObjectIdentifier("C"),
			Comment: ptr("x"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForCustomClientsOptions{
			Name:        NewAccountObjectIdentifier("C"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
