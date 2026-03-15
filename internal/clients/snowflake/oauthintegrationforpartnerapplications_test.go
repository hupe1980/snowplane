package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateOAuthPartnerAppsSQL(t *testing.T) {
	t.Parallel()

	t.Run("TableauDesktop", func(t *testing.T) {
		t.Parallel()
		opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
			Name:        NewAccountObjectIdentifier("MY_TABLEAU"),
			OAuthClient: "TABLEAU_DESKTOP",
		}
		got, err := buildCreateOAuthPartnerAppsSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `"MY_TABLEAU"`)
		assert.Contains(t, got, "TYPE = OAUTH")
		assert.Contains(t, got, "OAUTH_CLIENT = TABLEAU_DESKTOP")
	})

	t.Run("Looker", func(t *testing.T) {
		t.Parallel()
		opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
			Name:        NewAccountObjectIdentifier("MY_LOOKER"),
			OAuthClient: "LOOKER",
		}
		got, err := buildCreateOAuthPartnerAppsSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "OAUTH_CLIENT = LOOKER")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		validity := int64(7200)
		opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
			Name:                      NewAccountObjectIdentifier("FULL"),
			OAuthClient:               "TABLEAU_SERVER",
			OAuthRedirectURI:          ptr("https://tableau.example.com/auth"),
			OAuthUseSecondaryRoles:    ptr("IMPLICIT"),
			OAuthIssueRefreshTokens:   ptr(true),
			OAuthRefreshTokenValidity: &validity,
			BlockedRolesList:          []string{"ACCOUNTADMIN"},
			Comment:                   ptr("partner app"),
			Enabled:                   ptr(true),
		}
		got, err := buildCreateOAuthPartnerAppsSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "OAUTH_CLIENT = TABLEAU_SERVER")
		assert.Contains(t, got, "OAUTH_REDIRECT_URI = 'https://tableau.example.com/auth'")
		assert.Contains(t, got, "OAUTH_USE_SECONDARY_ROLES = IMPLICIT")
		assert.Contains(t, got, "OAUTH_ISSUE_REFRESH_TOKENS = TRUE")
		assert.Contains(t, got, "OAUTH_REFRESH_TOKEN_VALIDITY = 7200")
		assert.Contains(t, got, "BLOCKED_ROLES_LIST")
		assert.Contains(t, got, "COMMENT = 'partner app'")
		assert.Contains(t, got, "ENABLED = TRUE")
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()

		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
				OAuthClient: "LOOKER",
			}
			_, err := buildCreateOAuthPartnerAppsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("InvalidClient", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
				Name:        NewAccountObjectIdentifier("X"),
				OAuthClient: "INVALID_PARTNER",
			}
			_, err := buildCreateOAuthPartnerAppsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_client")
		})

		t.Run("MissingClient", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
				Name: NewAccountObjectIdentifier("X"),
			}
			_, err := buildCreateOAuthPartnerAppsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_client is required")
		})

		t.Run("InvalidOAuthUseSecondaryRoles", func(t *testing.T) {
			t.Parallel()
			opts := CreateOAuthIntegrationForPartnerApplicationsOptions{
				Name:                   NewAccountObjectIdentifier("X"),
				OAuthClient:            "LOOKER",
				OAuthUseSecondaryRoles: ptr("EVIL_MODE"),
			}
			_, err := buildCreateOAuthPartnerAppsSQL(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_use_secondary_roles")
		})
	})
}

func TestBuildAlterOAuthPartnerAppsStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:    NewAccountObjectIdentifier("MY_TABLEAU"),
			Comment: ptr("updated"),
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:        NewAccountObjectIdentifier("MY_TABLEAU"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
	})

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Parallel()
		t.Run("MissingName", func(t *testing.T) {
			t.Parallel()
			opts := AlterOAuthIntegrationForPartnerApplicationsOptions{}
			_, err := buildAlterOAuthPartnerAppsStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name is required")
		})

		t.Run("InvalidOAuthUseSecondaryRoles", func(t *testing.T) {
			t.Parallel()
			opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
				Name:                   NewAccountObjectIdentifier("MY_TABLEAU"),
				OAuthUseSecondaryRoles: ptr("INJECT; DROP TABLE"),
			}
			_, err := buildAlterOAuthPartnerAppsStatements(opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "oauth_use_secondary_roles")
		})
	})

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:    NewAccountObjectIdentifier("MY_TABLEAU"),
			Enabled: ptr(true),
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "ENABLED = TRUE")
	})

	t.Run("SetRefreshTokenValidity", func(t *testing.T) {
		t.Parallel()
		validity := int64(86400)
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:                      NewAccountObjectIdentifier("MY_TABLEAU"),
			OAuthRefreshTokenValidity: &validity,
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "OAUTH_REFRESH_TOKEN_VALIDITY = 86400")
	})

	t.Run("SetBlockedRolesList", func(t *testing.T) {
		t.Parallel()
		blocked := []string{"ACCOUNTADMIN"}
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:             NewAccountObjectIdentifier("MY_TABLEAU"),
			BlockedRolesList: &blocked,
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "BLOCKED_ROLES_LIST")
	})

	t.Run("CommentInjectionEscaped", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:    NewAccountObjectIdentifier("MY_TABLEAU"),
			Comment: ptr("test'; DROP TABLE users; --"),
		}
		stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Contains(t, stmts[0], "test''; DROP TABLE users; --")
		assert.NotContains(t, stmts[0], "test'; DROP TABLE")
	})
}

func TestOAuthPartnerAppsAlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{Name: NewAccountObjectIdentifier("C")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:    NewAccountObjectIdentifier("C"),
			Comment: ptr("x"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterOAuthIntegrationForPartnerApplicationsOptions{
			Name:        NewAccountObjectIdentifier("C"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
