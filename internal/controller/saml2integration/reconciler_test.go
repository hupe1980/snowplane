package saml2integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func successfulObservation() *snowflake.SAML2IntegrationObservation {
	return &snowflake.SAML2IntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SAML2IntegrationShowOutput{
			Name:    "MY_SAML",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"SAML2_X509_CERT":                     "MIIC...",
			"SAML2_SP_INITIATED_LOGIN_PAGE_LABEL": "Login via SSO",
			"SAML2_ENABLE_SP_INITIATED":           "true",
			"SAML2_FORCE_AUTHN":                   "false",
			"SAML2_REQUESTED_NAMEID_FORMAT":       "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
			"SAML2_POST_LOGOUT_REDIRECT_URL":      "https://example.com/logout",
		},
	}
}

func newTestSAML2Integration(name, ns string) *snowplanev1alpha1.SAML2Integration {
	return &snowplanev1alpha1.SAML2Integration{
		Spec: snowplanev1alpha1.SAML2IntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:     "MY_SAML",
			Issuer:   "https://idp.example.com",
			SSOURL:   "https://idp.example.com/sso",
			Provider: "CUSTOM",
			X509Cert: "MIIC...",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("SPInitiatedLoginLabelSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.SPInitiatedLoginPageLabel = testutil.Ptr("Login via SSO")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.SPInitiatedLoginLabel, "Should skip when value matches DESCRIBE output")
	})

	t.Run("SPInitiatedLoginLabelSentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.SPInitiatedLoginPageLabel = testutil.Ptr("New Label")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.SPInitiatedLoginLabel)
		assert.Equal(t, "New Label", *opts.SPInitiatedLoginLabel)
	})

	t.Run("EnableSPInitiatedSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.EnableSPInitiated = testutil.Ptr(true)
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.EnableSPInitiated, "Should skip when value matches DESCRIBE output")
	})

	t.Run("EnableSPInitiatedSentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.EnableSPInitiated = testutil.Ptr(false)
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.EnableSPInitiated)
		assert.Equal(t, false, *opts.EnableSPInitiated)
	})

	t.Run("ForceAuthnSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.ForceAuthn = testutil.Ptr(false)
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.ForceAuthn, "Should skip when value matches DESCRIBE output")
	})

	t.Run("ForceAuthnSentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.ForceAuthn = testutil.Ptr(true)
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.ForceAuthn)
		assert.Equal(t, true, *opts.ForceAuthn)
	})

	t.Run("RequestedNameIDFormatSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.RequestedNameIDFormat = testutil.Ptr("urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.RequestedNameIDFormat, "Should skip when value matches DESCRIBE output")
	})

	t.Run("PostLogoutRedirectURLSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.PostLogoutRedirectURL = testutil.Ptr("https://example.com/logout")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.PostLogoutRedirectURL, "Should skip when value matches DESCRIBE output")
	})

	t.Run("PostLogoutRedirectURLSentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.PostLogoutRedirectURL = testutil.Ptr("https://new.example.com/logout")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.PostLogoutRedirectURL)
		assert.Equal(t, "https://new.example.com/logout", *opts.PostLogoutRedirectURL)
	})

	t.Run("AllFieldsSentWhenNoDescribeOutput", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.SPInitiatedLoginPageLabel = testutil.Ptr("Label")
		obj.Spec.EnableSPInitiated = testutil.Ptr(true)
		obj.Spec.ForceAuthn = testutil.Ptr(false)
		obj.Spec.RequestedNameIDFormat = testutil.Ptr("urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress")
		obj.Spec.PostLogoutRedirectURL = testutil.Ptr("https://example.com/logout")
		id := snowflake.NewAccountObjectIdentifier("MY_SAML")
		obs := &snowflake.SAML2IntegrationObservation{
			Exists:     true,
			ShowOutput: successfulObservation().ShowOutput,
		}

		opts := buildAlterOptions(obj, id, obs)
		assert.NotNil(t, opts.SPInitiatedLoginLabel, "Should send when no DESCRIBE output")
		assert.NotNil(t, opts.EnableSPInitiated, "Should send when no DESCRIBE output")
		assert.NotNil(t, opts.ForceAuthn, "Should send when no DESCRIBE output")
		assert.NotNil(t, opts.RequestedNameIDFormat, "Should send when no DESCRIBE output")
		assert.NotNil(t, opts.PostLogoutRedirectURL, "Should send when no DESCRIBE output")
	})
}

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.Enabled = testutil.Ptr(true)
		obj.Spec.SPInitiatedLoginPageLabel = testutil.Ptr("Login via SSO")
		obj.Spec.EnableSPInitiated = testutil.Ptr(true)
		obj.Spec.ForceAuthn = testutil.Ptr(false)
		obj.Spec.RequestedNameIDFormat = testutil.Ptr("urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress")
		obj.Spec.PostLogoutRedirectURL = testutil.Ptr("https://example.com/logout")
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("SPInitiatedLoginLabelDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.SPInitiatedLoginPageLabel = testutil.Ptr("New Label")
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.Contains(t, result.Summary(), "SAML2_SP_INITIATED_LOGIN_PAGE_LABEL")
	})

	t.Run("EnableSPInitiatedDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.EnableSPInitiated = testutil.Ptr(false)
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.Contains(t, result.Summary(), "SAML2_ENABLE_SP_INITIATED")
	})

	t.Run("ForceAuthnDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestSAML2Integration("myint", "default")
		obj.Spec.ForceAuthn = testutil.Ptr(true)
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.Contains(t, result.Summary(), "SAML2_FORCE_AUTHN")
	})
}
