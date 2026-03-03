package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateSecurityIntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("SAML2Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:          NewAccountObjectIdentifier("MY_SAML"),
			Type:          "SAML2",
			SAML2Issuer:   ptr("https://idp.example.com"),
			SAML2SSOURL:   ptr("https://idp.example.com/sso"),
			SAML2Provider: ptr("CUSTOM"),
			SAML2X509Cert: ptr("MIIBxDCCAW..."),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECURITY INTEGRATION IF NOT EXISTS "MY_SAML"`)
		assert.Contains(t, got, "TYPE = SAML2")
		assert.Contains(t, got, "SAML2_ISSUER = 'https://idp.example.com'")
		assert.Contains(t, got, "SAML2_SSO_URL = 'https://idp.example.com/sso'")
		assert.Contains(t, got, "SAML2_PROVIDER = 'CUSTOM'")
		assert.Contains(t, got, "SAML2_X509_CERT = 'MIIBxDCCAW...'")
	})

	t.Run("SAML2WithAllowedEmailPatterns", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:                      NewAccountObjectIdentifier("MY_SAML"),
			Type:                      "SAML2",
			SAML2Issuer:               ptr("https://idp.example.com"),
			SAML2SSOURL:               ptr("https://idp.example.com/sso"),
			SAML2Provider:             ptr("CUSTOM"),
			SAML2X509Cert:             ptr("cert"),
			SAML2AllowedEmailPatterns: []string{"*@example.com", "*@corp.com"},
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ALLOWED_EMAIL_PATTERNS")
		assert.NotContains(t, got, "SAML2_SNOWFLAKE_ACS_URL")
	})

	t.Run("ExternalOAuth", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:                                  NewAccountObjectIdentifier("MY_OAUTH"),
			Type:                                  "EXTERNAL_OAUTH",
			ExternalOAuthType:                     ptr("AZURE"),
			ExternalOAuthIssuer:                   ptr("https://login.microsoftonline.com/tenant/v2.0"),
			ExternalOAuthTokenUserMappingClaim:    ptr("upn"),
			ExternalOAuthSnowflakeUserMappingAttr: ptr("login_name"),
			ExternalOAuthJWSKeysURL:               ptr("https://login.microsoftonline.com/tenant/discovery/v2.0/keys"),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = EXTERNAL_OAUTH")
		assert.Contains(t, got, "EXTERNAL_OAUTH_TYPE = AZURE")
		assert.Contains(t, got, "EXTERNAL_OAUTH_ISSUER = 'https://login.microsoftonline.com/tenant/v2.0'")
		assert.Contains(t, got, "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = 'upn'")
	})

	t.Run("SCIM", func(t *testing.T) {
		t.Parallel()
		enabled := true
		opts := CreateSecurityIntegrationOptions{
			Name:          NewAccountObjectIdentifier("MY_SCIM"),
			Type:          "SCIM",
			Enabled:       &enabled,
			SCIMClient:    ptr("AZURE"),
			SCIMRunAsRole: ptr("AAD_PROVISIONER"),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = SCIM")
		assert.Contains(t, got, "SCIM_CLIENT = 'AZURE'")
		assert.Contains(t, got, "RUN_AS_ROLE = 'AAD_PROVISIONER'")
		assert.Contains(t, got, "ENABLED = TRUE")
	})

	t.Run("APIAuthentication", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:               NewAccountObjectIdentifier("MY_API"),
			Type:               "API_AUTHENTICATION",
			OAuthClientID:      ptr("client-id"),
			OAuthClientSecret:  ptr("client-secret"),
			OAuthTokenEndpoint: ptr("https://example.com/token"),
			OAuthGrantType:     ptr("CLIENT_CREDENTIALS"),
			OAuthAllowedScopes: []string{"scope1", "scope2"},
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TYPE = API_AUTHENTICATION")
		assert.Contains(t, got, "AUTH_TYPE = OAUTH2")
		assert.Contains(t, got, "OAUTH_CLIENT_ID = 'client-id'")
		assert.Contains(t, got, "OAUTH_GRANT = 'CLIENT_CREDENTIALS'")
		assert.Contains(t, got, "OAUTH_TOKEN_ENDPOINT = 'https://example.com/token'")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:          NewAccountObjectIdentifier("MY_INT"),
			Type:          "SCIM",
			SCIMClient:    ptr("AZURE"),
			SCIMRunAsRole: ptr("ROLE"),
			Comment:       ptr("test comment"),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'test comment'")
	})

	t.Run("EscapeStringOnSCIMFields", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:          NewAccountObjectIdentifier("INT"),
			Type:          "SCIM",
			SCIMClient:    ptr("O'BRIEN"),
			SCIMRunAsRole: ptr("ROLE'S"),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "SCIM_CLIENT = 'O''BRIEN'")
		assert.Contains(t, got, "RUN_AS_ROLE = 'ROLE''S'")
	})

	t.Run("EscapeStringOnExternalOAuthAttr", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:                                  NewAccountObjectIdentifier("INT"),
			Type:                                  "EXTERNAL_OAUTH",
			ExternalOAuthType:                     ptr("CUSTOM"),
			ExternalOAuthIssuer:                   ptr("https://issuer"),
			ExternalOAuthTokenUserMappingClaim:    ptr("upn"),
			ExternalOAuthSnowflakeUserMappingAttr: ptr("user'name"),
		}
		got, err := buildCreateSecurityIntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = 'user''name'")
	})
}

func TestBuildAlterSecurityIntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "SCIM",
		}
		stmts, err := buildAlterSecurityIntegrationStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetEnabled", func(t *testing.T) {
		t.Parallel()
		enabled := false
		opts := AlterSecurityIntegrationOptions{
			Name:    NewAccountObjectIdentifier("INT"),
			Type:    "SCIM",
			Enabled: &enabled,
		}
		stmts, err := buildAlterSecurityIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ENABLED = FALSE")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{
			Name:    NewAccountObjectIdentifier("INT"),
			Type:    "SCIM",
			Comment: ptr("new comment"),
		}
		stmts, err := buildAlterSecurityIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'new comment'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{
			Name:        NewAccountObjectIdentifier("INT"),
			Type:        "SCIM",
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterSecurityIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("SetSCIMNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{
			Name:              NewAccountObjectIdentifier("INT"),
			Type:              "SCIM",
			SCIMNetworkPolicy: ptr("MY_POLICY"),
		}
		stmts, err := buildAlterSecurityIntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "NETWORK_POLICY = 'MY_POLICY'")
	})
}

func TestBuildShowSecurityIntegrationByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowSecurityIntegrationByIDSQL(NewAccountObjectIdentifier("MY_SAML"))
	assert.Contains(t, got, "SHOW SECURITY INTEGRATIONS LIKE 'MY\\_SAML'")
}

func TestCreateSecurityIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:          NewAccountObjectIdentifier("INT"),
			Type:          "SCIM",
			SCIMClient:    ptr("AZURE"),
			SCIMRunAsRole: ptr("AAD_PROVISIONER"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{Type: "SCIM"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
		}
		require.Error(t, opts.Validate())
	})

	t.Run("InvalidType", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "INVALID",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("ExternalOAuthMissingRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "EXTERNAL_OAUTH",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "external_oauth_type")
		assert.Contains(t, err.Error(), "external_oauth_issuer")
	})

	t.Run("ExternalOAuthInvalidType", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:                                  NewAccountObjectIdentifier("INT"),
			Type:                                  "EXTERNAL_OAUTH",
			ExternalOAuthType:                     ptr("INVALID_TYPE"),
			ExternalOAuthIssuer:                   ptr("https://issuer"),
			ExternalOAuthTokenUserMappingClaim:    ptr("upn"),
			ExternalOAuthSnowflakeUserMappingAttr: ptr("login_name"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid external_oauth_type")
	})

	t.Run("ExternalOAuthValid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:                                  NewAccountObjectIdentifier("INT"),
			Type:                                  "EXTERNAL_OAUTH",
			ExternalOAuthType:                     ptr("AZURE"),
			ExternalOAuthIssuer:                   ptr("https://issuer"),
			ExternalOAuthTokenUserMappingClaim:    ptr("upn"),
			ExternalOAuthSnowflakeUserMappingAttr: ptr("login_name"),
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("SAML2MissingRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "SAML2",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saml2_issuer")
		assert.Contains(t, err.Error(), "saml2_sso_url")
		assert.Contains(t, err.Error(), "saml2_provider")
		assert.Contains(t, err.Error(), "saml2_x509_cert")
	})

	t.Run("SCIMMissingRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "SCIM",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scim_client")
		assert.Contains(t, err.Error(), "run_as_role")
	})

	t.Run("APIAuthMissingRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name: NewAccountObjectIdentifier("INT"),
			Type: "API_AUTHENTICATION",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "oauth_client_id")
		assert.Contains(t, err.Error(), "oauth_client_secret")
	})

	t.Run("InvalidOAuthGrantType", func(t *testing.T) {
		t.Parallel()
		opts := CreateSecurityIntegrationOptions{
			Name:               NewAccountObjectIdentifier("INT"),
			Type:               "API_AUTHENTICATION",
			OAuthClientID:      ptr("id"),
			OAuthClientSecret:  ptr("secret"),
			OAuthTokenEndpoint: ptr("https://endpoint"),
			OAuthGrantType:     ptr("INVALID_GRANT"),
		}
		require.Error(t, opts.Validate())
	})
}

func TestAlterSecurityIntegrationOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("INT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{}
		require.Error(t, opts.Validate())
	})
}

func TestAlterSecurityIntegrationOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("I")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithEnabled", func(t *testing.T) {
		t.Parallel()
		e := true
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("I"), Enabled: &e}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("I"), Comment: ptr("c")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("I"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithSCIMNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		opts := AlterSecurityIntegrationOptions{Name: NewAccountObjectIdentifier("I"), SCIMNetworkPolicy: ptr("p")}
		assert.True(t, opts.HasChanges())
	})
}
