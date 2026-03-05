package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateSAML2IntegrationSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRequired", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("MY_SAML2_SSO"),
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/saml/sso",
			Provider: "CUSTOM",
			X509Cert: "MIIC_base64cert",
		}
		got, err := buildCreateSAML2IntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE SECURITY INTEGRATION IF NOT EXISTS "MY_SAML2_SSO"`)
		assert.Contains(t, got, "TYPE = SAML2")
		assert.Contains(t, got, "SAML2_ISSUER = 'https://idp.example.com'")
		assert.Contains(t, got, "SAML2_SSO_URL = 'https://idp.example.com/saml/sso'")
		assert.Contains(t, got, "SAML2_PROVIDER = 'CUSTOM'")
		assert.Contains(t, got, "SAML2_X509_CERT = 'MIIC_base64cert'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:                  NewAccountObjectIdentifier("FULL_SAML"),
			Issuer:                "https://idp.example.com",
			SSOURL:                "https://idp.example.com/saml/sso",
			Provider:              "CUSTOM",
			X509Cert:              "cert",
			Enabled:               ptr(true),
			Comment:               ptr("my comment"),
			AllowedEmailPatterns:  []string{"*@example.com", "*@corp.com"},
			AllowedUserDomains:    []string{"example.com"},
			SPInitiatedLoginLabel: ptr("Login via SSO"),
			EnableSPInitiated:     ptr(true),
			ForceAuthn:            ptr(false),
			RequestedNameIDFormat: ptr("urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"),
			PostLogoutRedirectURL: ptr("https://example.com/logout"),
		}
		got, err := buildCreateSAML2IntegrationSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "SAML2_SP_INITIATED_LOGIN_PAGE_LABEL = 'Login via SSO'")
		assert.Contains(t, got, "SAML2_ENABLE_SP_INITIATED = TRUE")
		assert.Contains(t, got, "SAML2_FORCE_AUTHN = FALSE")
		assert.Contains(t, got, "SAML2_REQUESTED_NAMEID_FORMAT")
		assert.Contains(t, got, "SAML2_POST_LOGOUT_REDIRECT_URL")
		assert.Contains(t, got, "ALLOWED_EMAIL_PATTERNS")
		assert.Contains(t, got, "ALLOWED_USER_DOMAINS")
		assert.Contains(t, got, "ENABLED = TRUE")
		assert.Contains(t, got, "COMMENT = 'my comment'")
	})
}

func TestBuildCreateSAML2IntegrationSQL_Validation(t *testing.T) {
	t.Parallel()

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
			X509Cert: "cert",
		}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingIssuer", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("TEST"),
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
			X509Cert: "cert",
		}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saml2_issuer is required")
	})

	t.Run("MissingSSOURL", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("TEST"),
			Issuer:   "https://idp.example.com",
			Provider: "CUSTOM",
			X509Cert: "cert",
		}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saml2_sso_url is required")
	})

	t.Run("MissingProvider", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("TEST"),
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			X509Cert: "cert",
		}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saml2_provider is required")
	})

	t.Run("MissingX509Cert", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("TEST"),
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
		}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "saml2_x509_cert is required")
	})

	t.Run("MultipleErrors", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{}
		_, err := buildCreateSAML2IntegrationSQL(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
		assert.Contains(t, err.Error(), "saml2_issuer is required")
		assert.Contains(t, err.Error(), "saml2_sso_url is required")
		assert.Contains(t, err.Error(), "saml2_provider is required")
		assert.Contains(t, err.Error(), "saml2_x509_cert is required")
	})
}

// --------------------------------------------------------------------------
// Alter SQL generation tests
// --------------------------------------------------------------------------

func TestBuildAlterSAML2IntegrationStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Name:    NewAccountObjectIdentifier("MY_SAML2_SSO"),
			Comment: ptr("updated comment"),
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], `ALTER SECURITY INTEGRATION "MY_SAML2_SSO" SET`)
		assert.Contains(t, got[0], "COMMENT = 'updated comment'")
	})

	t.Run("SetMultipleFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("MY_SAML2_SSO"),
			Enabled:  ptr(false),
			X509Cert: ptr("NEW_CERT"),
			Comment:  ptr("new comment"),
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "ENABLED = FALSE")
		assert.Contains(t, got[0], "SAML2_X509_CERT = 'NEW_CERT'")
		assert.Contains(t, got[0], "COMMENT = 'new comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_SAML2_SSO"),
			UnsetFields: []string{"COMMENT", "SAML2_FORCE_AUTHN"},
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "UNSET")
		assert.Contains(t, got[0], "COMMENT")
		assert.Contains(t, got[0], "SAML2_FORCE_AUTHN")
	})

	t.Run("SetAndUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Name:        NewAccountObjectIdentifier("MY_SAML2_SSO"),
			Enabled:     ptr(true),
			UnsetFields: []string{"COMMENT"},
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Contains(t, got[0], "SET")
		assert.Contains(t, got[0], "ENABLED = TRUE")
		assert.Contains(t, got[1], "UNSET")
		assert.Contains(t, got[1], "COMMENT")
	})

	t.Run("SetListFields", func(t *testing.T) {
		t.Parallel()
		emails := []string{"*@example.com", "*@corp.com"}
		domains := []string{"example.com"}
		opts := AlterSAML2IntegrationOptions{
			Name:                 NewAccountObjectIdentifier("MY_SAML2_SSO"),
			AllowedEmailPatterns: &emails,
			AllowedUserDomains:   &domains,
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "ALLOWED_EMAIL_PATTERNS")
		assert.Contains(t, got[0], "ALLOWED_USER_DOMAINS")
	})

	t.Run("ValidationFailsMissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Comment: ptr("test"),
		}
		_, err := buildAlterSAML2IntegrationStatements(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("SetSPInitiatedFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{
			Name:                  NewAccountObjectIdentifier("MY_SAML2_SSO"),
			SPInitiatedLoginLabel: ptr("Login with SSO"),
			EnableSPInitiated:     ptr(true),
			ForceAuthn:            ptr(true),
			RequestedNameIDFormat: ptr("urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"),
			PostLogoutRedirectURL: ptr("https://example.com/logout"),
		}
		got, err := buildAlterSAML2IntegrationStatements(opts)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "SAML2_SP_INITIATED_LOGIN_PAGE_LABEL")
		assert.Contains(t, got[0], "SAML2_ENABLE_SP_INITIATED = TRUE")
		assert.Contains(t, got[0], "SAML2_FORCE_AUTHN = TRUE")
		assert.Contains(t, got[0], "SAML2_REQUESTED_NAMEID_FORMAT")
		assert.Contains(t, got[0], "SAML2_POST_LOGOUT_REDIRECT_URL")
	})
}

// --------------------------------------------------------------------------
// HasChanges tests
// --------------------------------------------------------------------------

func TestSAML2AlterHasChanges(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithEnabled", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), Enabled: ptr(true)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithX509Cert", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), X509Cert: ptr("cert")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), Comment: ptr("hello")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithAllowedEmailPatterns", func(t *testing.T) {
		t.Parallel()
		patterns := []string{"*@example.com"}
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), AllowedEmailPatterns: &patterns}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithSPInitiated", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), EnableSPInitiated: ptr(true)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithForceAuthn", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), ForceAuthn: ptr(false)}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithRequestedNameIDFormat", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), RequestedNameIDFormat: ptr("fmt")}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithPostLogoutRedirectURL", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST"), PostLogoutRedirectURL: ptr("url")}
		assert.True(t, opts.HasChanges())
	})
}

// --------------------------------------------------------------------------
// Validate tests
// --------------------------------------------------------------------------

func TestSAML2CreateValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateSAML2IntegrationOptions{
			Name:     NewAccountObjectIdentifier("TEST"),
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
			X509Cert: "cert",
		}
		assert.NoError(t, opts.Validate())
	})
}

func TestSAML2AlterValidate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{Name: NewAccountObjectIdentifier("TEST")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterSAML2IntegrationOptions{}
		assert.Error(t, opts.Validate())
	})
}
