package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitiveStatusPath_NotInRegistry(t *testing.T) {
	t.Parallel()
	// Database is not in SensitiveStatusPaths — all paths are safe.
	assert.False(t, IsSensitiveStatusPath("Database", ".status.showOutput.name"))
	assert.False(t, IsSensitiveStatusPath("Database", ".status.fullyQualifiedName"))
}

func TestIsSensitiveStatusPath_ExactMatch(t *testing.T) {
	t.Parallel()
	// "describeOutput" is listed for ExternalOAuthIntegration.
	assert.True(t, IsSensitiveStatusPath("ExternalOAuthIntegration", ".status.describeOutput"))
}

func TestIsSensitiveStatusPath_PrefixMatch(t *testing.T) {
	t.Parallel()
	// Sub-paths of a registered prefix must also be blocked.
	assert.True(t, IsSensitiveStatusPath("ExternalOAuthIntegration", ".status.describeOutput.oauthClientId"))
	assert.True(t, IsSensitiveStatusPath("SAML2Integration", ".status.describeOutput.saml2X509Cert"))
	assert.True(t, IsSensitiveStatusPath("ExternalOAuthIntegration", ".status.describeOutput.externalOauthJwsKeysUrl"))
}

func TestIsSensitiveStatusPath_UserPII(t *testing.T) {
	t.Parallel()
	assert.True(t, IsSensitiveStatusPath("User", ".status.showOutput.email"))
	assert.True(t, IsSensitiveStatusPath("User", ".status.showOutput.loginName"))
	assert.True(t, IsSensitiveStatusPath("User", ".status.lastAppliedPasswordHash"))
	assert.True(t, IsSensitiveStatusPath("User", ".status.lastAppliedRSAPublicKeyHash"))
	assert.True(t, IsSensitiveStatusPath("User", ".status.describeOutput.rsaPublicKeyFp"))
}

func TestIsSensitiveStatusPath_UserSafePaths(t *testing.T) {
	t.Parallel()
	// Non-sensitive User status paths should be allowed.
	assert.False(t, IsSensitiveStatusPath("User", ".status.showOutput.name"))
	assert.False(t, IsSensitiveStatusPath("User", ".status.showOutput.createdOn"))
	assert.False(t, IsSensitiveStatusPath("User", ".status.fullyQualifiedName"))
}

func TestIsSensitiveStatusPath_InfrastructureSecrets(t *testing.T) {
	t.Parallel()
	assert.True(t, IsSensitiveStatusPath("StorageIntegrationAWS", ".status.storageAWSIAMUserARN"))
	assert.True(t, IsSensitiveStatusPath("StorageIntegrationAWS", ".status.storageAWSExternalID"))
	assert.True(t, IsSensitiveStatusPath("StorageIntegrationGCS", ".status.storageGCPServiceAccount"))
	assert.True(t, IsSensitiveStatusPath("StorageIntegrationAzure", ".status.azureConsentURL"))
	assert.True(t, IsSensitiveStatusPath("StorageIntegrationAzure", ".status.azureMultiTenantAppName"))
	assert.True(t, IsSensitiveStatusPath("ExternalFunction", ".status.url"))
	assert.True(t, IsSensitiveStatusPath("ExternalStage", ".status.showOutput.url"))
	assert.True(t, IsSensitiveStatusPath("Pipe", ".status.showOutput.notificationChannel"))
	assert.True(t, IsSensitiveStatusPath("Pipe", ".status.notificationChannel"))
	assert.True(t, IsSensitiveStatusPath("ImageRepository", ".status.showOutput.repositoryUrl"))
}

func TestIsSensitiveStatusPath_SecretCRDs(t *testing.T) {
	t.Parallel()
	// All Secret CRDs block describeOutput entirely.
	for _, kind := range []string{
		"SecretWithClientCredentials",
		"SecretWithAuthorizationCodeGrant",
		"SecretWithBasicAuthentication",
		"SecretWithGenericString",
	} {
		assert.True(t, IsSensitiveStatusPath(kind, ".status.describeOutput"), "kind=%s", kind)
		assert.True(t, IsSensitiveStatusPath(kind, ".status.describeOutput.oauthScopes"), "kind=%s", kind)
	}
}

func TestIsSensitiveStatusPath_APIAuthIntegrations(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{
		"APIAuthenticationIntegrationWithClientCredentials",
		"APIAuthenticationIntegrationWithAuthorizationCodeGrant",
		"APIAuthenticationIntegrationWithJWTBearer",
	} {
		assert.True(t, IsSensitiveStatusPath(kind, ".status.describeOutput"), "kind=%s", kind)
		assert.True(t, IsSensitiveStatusPath(kind, ".status.describeOutput.oauthTokenEndpoint"), "kind=%s", kind)
	}
}

func TestIsSensitiveStatusPath_NoStatusPrefix(t *testing.T) {
	t.Parallel()
	// Path without ".status." prefix should always return false (nothing to guard).
	assert.False(t, IsSensitiveStatusPath("User", ".spec.forProvider.email"))
	assert.False(t, IsSensitiveStatusPath("User", "status.showOutput.email"))
}

func TestIsSensitiveStatusPath_PartialPrefixNoMatch(t *testing.T) {
	t.Parallel()
	// "describeOutputExtra" should NOT match the "describeOutput" prefix —
	// the match must be exact or followed by a dot.
	assert.False(t, IsSensitiveStatusPath("ExternalOAuthIntegration", ".status.describeOutputExtra"))
}
