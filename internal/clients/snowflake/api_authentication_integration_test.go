package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests — CREATE
// --------------------------------------------------------------------------

func TestBuildCreateAPIAuthIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("ClientCredentials", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:               NewAccountObjectIdentifier("MY_CC_AUTH"),
			OAuthGrantType:     OAuthGrantTypeClientCredentials,
			OAuthClientID:      "client-id",
			OAuthClientSecret:  "client-secret",
			Enabled:            &enabled,
			OAuthTokenEndpoint: strPtr("https://token.example.com/oauth/token"),
			OAuthAllowedScopes: []string{"read", "write"},
			Comment:            strPtr("CC auth integration"),
		}

		got := buildCreateAPIAuthIntegrationSQL(opts)
		assert.Contains(t, got, `CREATE SECURITY INTEGRATION IF NOT EXISTS "MY_CC_AUTH"`)
		assert.Contains(t, got, "TYPE = API_AUTHENTICATION AUTH_TYPE = OAUTH2")
		assert.Contains(t, got, "OAUTH_GRANT = CLIENT_CREDENTIALS")
		assert.Contains(t, got, "OAUTH_CLIENT_ID = 'client-id'")
		assert.Contains(t, got, "OAUTH_CLIENT_SECRET = 'client-secret'")
		assert.Contains(t, got, "OAUTH_TOKEN_ENDPOINT = 'https://token.example.com/oauth/token'")
		assert.Contains(t, got, "OAUTH_ALLOWED_SCOPES")
		assert.Contains(t, got, "'read'")
		assert.Contains(t, got, "'write'")
		assert.Contains(t, got, "COMMENT = 'CC auth integration'")
		assert.Contains(t, got, "ENABLED = TRUE")
	})

	t.Run("AuthorizationCodeGrant", func(t *testing.T) {
		t.Parallel()
		enabled := true
		refreshValidity := int32(86400)
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:                       NewAccountObjectIdentifier("MY_ACG_AUTH"),
			OAuthGrantType:             OAuthGrantTypeAuthorizationCode,
			OAuthClientID:              "acg-client",
			OAuthClientSecret:          "acg-secret",
			Enabled:                    &enabled,
			OAuthAuthorizationEndpoint: strPtr("https://auth.example.com/authorize"),
			OAuthRefreshTokenValidity:  &refreshValidity,
		}

		got := buildCreateAPIAuthIntegrationSQL(opts)
		assert.Contains(t, got, "OAUTH_GRANT = AUTHORIZATION_CODE")
		assert.Contains(t, got, "OAUTH_AUTHORIZATION_ENDPOINT = 'https://auth.example.com/authorize'")
		assert.Contains(t, got, "OAUTH_REFRESH_TOKEN_VALIDITY = 86400")
	})

	t.Run("JWTBearer", func(t *testing.T) {
		t.Parallel()
		enabled := false
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_JWT_AUTH"),
			OAuthGrantType:       OAuthGrantTypeJWTBearer,
			OAuthClientID:        "jwt-client",
			OAuthClientSecret:    "jwt-secret",
			Enabled:              &enabled,
			OAuthAssertionIssuer: strPtr("https://issuer.example.com"),
		}

		got := buildCreateAPIAuthIntegrationSQL(opts)
		assert.Contains(t, got, "OAUTH_GRANT = JWT_BEARER")
		assert.Contains(t, got, "OAUTH_ASSERTION_ISSUER = 'https://issuer.example.com'")
		assert.Contains(t, got, "ENABLED = FALSE")
	})

	t.Run("WithClientAuthMethod", func(t *testing.T) {
		t.Parallel()
		method := "CLIENT_SECRET_POST"
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:        OAuthGrantTypeClientCredentials,
			OAuthClientID:         "id",
			OAuthClientSecret:     "secret",
			OAuthClientAuthMethod: &method,
		}

		got := buildCreateAPIAuthIntegrationSQL(opts)
		assert.Contains(t, got, "OAUTH_CLIENT_AUTH_METHOD = CLIENT_SECRET_POST")
	})
}

// --------------------------------------------------------------------------
// SQL generation tests — ALTER
// --------------------------------------------------------------------------

func TestBuildAlterAPIAuthIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Comment:        strPtr("updated"),
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)
		assert.Contains(t, stmts[0], `ALTER SECURITY INTEGRATION "MY_AUTH" SET`)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Enabled:        &enabled,
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)
		assert.Contains(t, stmts[0], "ENABLED = TRUE")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			UnsetFields:    []string{"COMMENT"},
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)

		combined := ""
		for _, s := range stmts {
			combined += s + " "
		}

		assert.Contains(t, combined, "UNSET")
		assert.Contains(t, combined, "COMMENT")
	})

	t.Run("SetOAuthTokenEndpoint", func(t *testing.T) {
		t.Parallel()
		endpoint := "https://new-endpoint.com/token"
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:               NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:     OAuthGrantTypeClientCredentials,
			OAuthTokenEndpoint: &endpoint,
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)
		assert.Contains(t, stmts[0], "OAUTH_TOKEN_ENDPOINT = 'https://new-endpoint.com/token'")
	})

	t.Run("SetOAuthAllowedScopes", func(t *testing.T) {
		t.Parallel()
		scopes := []string{"scope1", "scope2"}
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:               NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:     OAuthGrantTypeClientCredentials,
			OAuthAllowedScopes: &scopes,
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(stmts), 1)

		combined := ""
		for _, s := range stmts {
			combined += s + " "
		}

		assert.Contains(t, combined, "'scope1'")
		assert.Contains(t, combined, "'scope2'")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
		}

		stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})
}

// --------------------------------------------------------------------------
// Validate
// --------------------------------------------------------------------------

func TestCreateAPIAuthenticationIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:    OAuthGrantTypeClientCredentials,
			OAuthClientID:     "id",
			OAuthClientSecret: "secret",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			OAuthGrantType:    OAuthGrantTypeClientCredentials,
			OAuthClientID:     "id",
			OAuthClientSecret: "secret",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingGrantType", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_AUTH"),
			OAuthClientID:     "id",
			OAuthClientSecret: "secret",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingClientID", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:    OAuthGrantTypeClientCredentials,
			OAuthClientSecret: "secret",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingClientSecret", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			OAuthClientID:  "id",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("JWTBearerMissingAssertionIssuer", func(t *testing.T) {
		t.Parallel()
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:              NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:    OAuthGrantTypeJWTBearer,
			OAuthClientID:     "id",
			OAuthClientSecret: "secret",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidAuthMethod", func(t *testing.T) {
		t.Parallel()
		method := "INVALID"
		opts := CreateAPIAuthenticationIntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:        OAuthGrantTypeClientCredentials,
			OAuthClientID:         "id",
			OAuthClientSecret:     "secret",
			OAuthClientAuthMethod: &method,
		}
		assert.Error(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// ALTER Validate
// --------------------------------------------------------------------------

func TestAlterAPIAuthenticationIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Comment:        strPtr("updated"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Comment:        strPtr("updated"),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidAuthMethod", func(t *testing.T) {
		t.Parallel()
		method := "BAD_METHOD"
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:        OAuthGrantTypeClientCredentials,
			OAuthClientAuthMethod: &method,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("ValidAuthMethod", func(t *testing.T) {
		t.Parallel()
		method := "CLIENT_SECRET_POST"
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType:        OAuthGrantTypeClientCredentials,
			OAuthClientAuthMethod: &method,
		}
		assert.NoError(t, opts.Validate())
	})
}

// --------------------------------------------------------------------------
// HasChanges
// --------------------------------------------------------------------------

func TestAlterAPIAuthenticationIntegrationOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Comment:        strPtr("hi"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("EnabledSet", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			Enabled:        &enabled,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAPIAuthenticationIntegrationOptions{
			Name:           NewAccountObjectIdentifier("MY_AUTH"),
			OAuthGrantType: OAuthGrantTypeClientCredentials,
			UnsetFields:    []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
