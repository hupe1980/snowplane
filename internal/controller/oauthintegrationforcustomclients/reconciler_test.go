package oauthintegrationforcustomclients

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func newTestOAuthCustomClients(name, ns string) *snowplanev1alpha1.OAuthIntegrationForCustomClients {
	return &snowplanev1alpha1.OAuthIntegrationForCustomClients{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.OAuthIntegrationForCustomClientsSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:             "MY_OAUTH",
			OAuthClientType:  "CONFIDENTIAL",
			OAuthRedirectURI: "https://example.com/callback",
		},
	}
}

func successfulObservation() *snowflake.OAuthIntegrationForCustomClientsObservation {
	return &snowflake.OAuthIntegrationForCustomClientsObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.OAuthIntegrationForCustomClientsShowOutput{
			Name:    "MY_OAUTH",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"OAUTH_CLIENT_TYPE":                "CONFIDENTIAL",
			"OAUTH_REDIRECT_URI":               "https://example.com/callback",
			"OAUTH_ALLOW_NON_TLS_REDIRECT_URI": "false",
			"OAUTH_ENFORCE_PKCE":               "false",
			"OAUTH_USE_SECONDARY_ROLES":        "",
			"OAUTH_ISSUE_REFRESH_TOKENS":       "true",
			"OAUTH_REFRESH_TOKEN_VALIDITY":     "7776000",
			"NETWORK_POLICY":                   "",
			"OAUTH_CLIENT_RSA_PUBLIC_KEY":      "",
			"OAUTH_CLIENT_RSA_PUBLIC_KEY_2":    "",
			"PRE_AUTHORIZED_ROLES_LIST":        "",
			"BLOCKED_ROLES_LIST":               "",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("RSAKeySkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		key := "MIIBIjANBg..."
		obj := newTestOAuthCustomClients("x", "default")
		obj.Spec.OAuthClientRSAPublicKey = &key

		obs := successfulObservation()
		obs.DescribeOutput["OAUTH_CLIENT_RSA_PUBLIC_KEY"] = "MIIBIjANBg..."

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.OAuthClientRSAPublicKey, "RSA key should be skipped when unchanged")
	})

	t.Run("RSAKeySentWhenChanged", func(t *testing.T) {
		t.Parallel()

		key := "MIIBIjANBgNewKey..."
		obj := newTestOAuthCustomClients("x", "default")
		obj.Spec.OAuthClientRSAPublicKey = &key

		obs := successfulObservation()
		obs.DescribeOutput["OAUTH_CLIENT_RSA_PUBLIC_KEY"] = "MIIBIjANBgOldKey..."

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.OAuthClientRSAPublicKey)
		assert.Equal(t, key, *opts.OAuthClientRSAPublicKey)
	})

	t.Run("RSAKey2SkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		key2 := "MIIBIjANBgKey2..."
		obj := newTestOAuthCustomClients("x", "default")
		obj.Spec.OAuthClientRSAPublicKey2 = &key2

		obs := successfulObservation()
		obs.DescribeOutput["OAUTH_CLIENT_RSA_PUBLIC_KEY_2"] = "MIIBIjANBgKey2..."

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.OAuthClientRSAPublicKey2, "RSA key 2 should be skipped when unchanged")
	})

	t.Run("RSAKeysSentWhenNoObservation", func(t *testing.T) {
		t.Parallel()

		key := "MIIBIjANBg..."
		key2 := "MIIBIjANBgKey2..."
		obj := newTestOAuthCustomClients("x", "default")
		obj.Spec.OAuthClientRSAPublicKey = &key
		obj.Spec.OAuthClientRSAPublicKey2 = &key2

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.OAuthClientRSAPublicKey, "should be sent when no observation")
		require.NotNil(t, opts.OAuthClientRSAPublicKey2, "should be sent when no observation")
	})
}
