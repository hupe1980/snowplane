package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func newPC() *snowplanev1alpha1.ProviderConfig {
	return &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "pc", Namespace: "default"},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "acct",
			User:               "user",
			Region:             "us-east-1",
			Role:               "SYSADMIN",
			Warehouse:          "WH",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name:      "sec",
					Namespace: "default",
					Key:       "password",
				},
			},
		},
	}
}

func newSecret(data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sec", Namespace: "default"},
		Data:       data,
	}
}

func TestBuildSnowflakeConfig_UsernamePassword(t *testing.T) {
	t.Parallel()

	cfg, err := BuildSnowflakeConfig(newPC(), newSecret(map[string][]byte{"password": []byte("s3cret")}))
	require.NoError(t, err)
	assert.Equal(t, "acct", cfg.Account)
	assert.Equal(t, "user", cfg.User)
	assert.Equal(t, []byte("s3cret"), cfg.Password)
	assert.Empty(t, cfg.PrivateKey)
}

func TestBuildSnowflakeConfig_KeyPair(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeKeyPair
	pc.Spec.Credentials.SecretRef.Key = "private-key"

	cfg, err := BuildSnowflakeConfig(pc, newSecret(map[string][]byte{
		"private-key": []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----"),
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.PrivateKey)
	assert.Empty(t, cfg.Password)
}

func TestBuildSnowflakeConfig_KeyPair_WithPassphrase(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeKeyPair
	pc.Spec.Credentials.SecretRef.Key = "private-key"
	pc.Spec.Credentials.PassphraseKey = "passphrase"

	cfg, err := BuildSnowflakeConfig(pc, newSecret(map[string][]byte{
		"private-key": []byte("-----BEGIN ENCRYPTED PRIVATE KEY-----\nfake\n-----END ENCRYPTED PRIVATE KEY-----"),
		"passphrase":  []byte("my-secret-passphrase"),
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.PrivateKey)
	assert.Equal(t, []byte("my-secret-passphrase"), cfg.Passphrase)
}

func TestBuildSnowflakeConfig_KeyPair_PassphraseMissing(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeKeyPair
	pc.Spec.Credentials.SecretRef.Key = "private-key"
	pc.Spec.Credentials.PassphraseKey = "passphrase"

	_, err := BuildSnowflakeConfig(pc, newSecret(map[string][]byte{
		"private-key": []byte("-----BEGIN ENCRYPTED PRIVATE KEY-----\nfake\n-----END ENCRYPTED PRIVATE KEY-----"),
		// passphrase key missing from secret
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain passphrase key")
}

func TestComputeHash_PassphraseAffectsHash(t *testing.T) {
	t.Parallel()

	cfg1 := snowflake.Config{Account: "a", PrivateKey: []byte("k")}
	cfg2 := snowflake.Config{Account: "a", PrivateKey: []byte("k"), Passphrase: []byte("pp")}
	assert.NotEqual(t, ComputeHash(cfg1), ComputeHash(cfg2))
}

func TestBuildSnowflakeConfig_MissingKey(t *testing.T) {
	t.Parallel()

	_, err := BuildSnowflakeConfig(newPC(), newSecret(map[string][]byte{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain key")
}

func TestBuildSnowflakeConfig_UnsupportedAuth(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = "ExternalOAuth"

	_, err := BuildSnowflakeConfig(pc, newSecret(map[string][]byte{"password": []byte("x")}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported authentication type")
	assert.Contains(t, err.Error(), "ExternalOAuth")
}

func TestBuildSnowflakeConfig_EmptyAuth(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = ""

	_, err := BuildSnowflakeConfig(pc, newSecret(map[string][]byte{"password": []byte("x")}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authenticationType is required")
	assert.Contains(t, err.Error(), "KeyPair")
	assert.Contains(t, err.Error(), "UsernamePassword")
	assert.Contains(t, err.Error(), "WorkloadIdentity")
}

func TestBuildSnowflakeConfig_WorkloadIdentity_OIDC(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &snowplanev1alpha1.WorkloadIdentitySpec{
		Provider:      snowplanev1alpha1.WIFProviderOIDC,
		TokenFilePath: "/var/run/secrets/snowflake/token",
	}

	cfg, err := BuildSnowflakeConfig(pc, nil)
	require.NoError(t, err)
	assert.Equal(t, "/var/run/secrets/snowflake/token", cfg.TokenFilePath)
	assert.Equal(t, "OIDC", cfg.WorkloadIdentityProvider)
	assert.Empty(t, cfg.Password)
	assert.Empty(t, cfg.PrivateKey)
}

func TestBuildSnowflakeConfig_WorkloadIdentity_DefaultOIDC(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &snowplanev1alpha1.WorkloadIdentitySpec{}

	cfg, err := BuildSnowflakeConfig(pc, nil)
	require.NoError(t, err)
	assert.Equal(t, snowplanev1alpha1.DefaultTokenFilePath, cfg.TokenFilePath)
	assert.Equal(t, "OIDC", cfg.WorkloadIdentityProvider)
}

func TestBuildSnowflakeConfig_WorkloadIdentity_AWS(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &snowplanev1alpha1.WorkloadIdentitySpec{
		Provider: snowplanev1alpha1.WIFProviderAWS,
	}

	// AWS provider uses IAM credentials and doesn't need a token file,
	// but BuildSnowflakeConfig still validates the default OIDC path.
	// For AWS, the path validation is skipped (only validated for OIDC).
	cfg, err := BuildSnowflakeConfig(pc, nil)
	require.NoError(t, err)
	assert.Equal(t, "AWS", cfg.WorkloadIdentityProvider)
}

func TestBuildSnowflakeConfig_WorkloadIdentity_MissingSpec(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = nil

	_, err := BuildSnowflakeConfig(pc, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.workloadIdentity is required")
}

func TestBuildSnowflakeConfig_WorkloadIdentity_InvalidTokenPath(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &snowplanev1alpha1.WorkloadIdentitySpec{
		TokenFilePath: "/etc/passwd",
	}

	_, err := BuildSnowflakeConfig(pc, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not under an allowed prefix")
}

func TestComputeHash_Deterministic(t *testing.T) {
	t.Parallel()

	cfg := snowflake.Config{Account: "a", User: "u", Password: []byte("p")}
	assert.Equal(t, ComputeHash(cfg), ComputeHash(cfg))
	assert.Len(t, ComputeHash(cfg), 64) // SHA-256 hex
}

func TestComputeHash_DifferentConfigs(t *testing.T) {
	t.Parallel()

	c1 := snowflake.Config{Account: "a"}
	c2 := snowflake.Config{Account: "b"}
	assert.NotEqual(t, ComputeHash(c1), ComputeHash(c2))
}

func TestComputeHash_SeparatorPreventsCollision(t *testing.T) {
	t.Parallel()

	c1 := snowflake.Config{Account: "ab", User: "cd"}
	c2 := snowflake.Config{Account: "abc", User: "d"}
	assert.NotEqual(t, ComputeHash(c1), ComputeHash(c2))
}

func TestComputeHash_WIFUsesPathNotToken(t *testing.T) {
	t.Parallel()

	// WIF configs with same token file path should produce the same hash,
	// regardless of actual token content (which changes on every kubelet refresh).
	c1 := snowflake.Config{Account: "a", TokenFilePath: "/var/run/secrets/snowflake/token", WorkloadIdentityProvider: "OIDC"}
	c2 := snowflake.Config{Account: "a", TokenFilePath: "/var/run/secrets/snowflake/token", WorkloadIdentityProvider: "OIDC"}
	assert.Equal(t, ComputeHash(c1), ComputeHash(c2))

	// Different token paths should produce different hashes.
	c3 := snowflake.Config{Account: "a", TokenFilePath: "/var/run/secrets/other/token", WorkloadIdentityProvider: "OIDC"}
	assert.NotEqual(t, ComputeHash(c1), ComputeHash(c3))
}

func TestValidateTokenFilePath_Allowed(t *testing.T) {
	t.Parallel()

	tests := []string{
		"/var/run/secrets/snowflake/token",
		"/var/run/secrets/kubernetes.io/serviceaccount/token",
		"/var/run/secrets/oauth/access_token",
	}

	for _, path := range tests {
		assert.NoError(t, ValidateTokenFilePath(path), "path %q should be allowed", path)
	}
}

func TestValidateTokenFilePath_Blocked(t *testing.T) {
	t.Parallel()

	tests := []string{
		"/etc/passwd",
		"/tmp/token",
		"/var/run/secrets/../../../etc/passwd",
		"../../../etc/shadow",
		"/home/user/.ssh/id_rsa",
	}

	for _, path := range tests {
		err := ValidateTokenFilePath(path)
		assert.Error(t, err, "path %q should be blocked", path)
		assert.Contains(t, err.Error(), "not under an allowed prefix")
	}
}

func TestValidateTokenFilePath_SymlinkBypass(t *testing.T) {
	// Not parallel — this test temporarily overrides AllowedTokenPathPrefixes.

	// Create a temp directory that simulates /var/run/secrets/ structure.
	// We can't create files in /var/run/secrets/ in tests, so we override
	// AllowedTokenPathPrefixes temporarily.
	tmpDir := t.TempDir()

	// Resolve the tmpDir itself — on macOS /var is a symlink to /private/var.
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	allowedDir := filepath.Join(tmpDir, "var", "run", "secrets")
	require.NoError(t, os.MkdirAll(allowedDir, 0o755))

	// Create a file outside the allowed tree.
	outsideFile := filepath.Join(tmpDir, "etc", "shadow")
	require.NoError(t, os.MkdirAll(filepath.Dir(outsideFile), 0o755))
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0o600))

	// Create a symlink inside the allowed tree that points outside.
	symlink := filepath.Join(allowedDir, "evil")
	require.NoError(t, os.Symlink(outsideFile, symlink))

	// Save and restore the original prefixes.
	origPrefixes := AllowedTokenPathPrefixes
	AllowedTokenPathPrefixes = []string{allowedDir + "/"}
	t.Cleanup(func() { AllowedTokenPathPrefixes = origPrefixes })

	// The symlink is under the allowed prefix lexically, but resolves outside.
	err = ValidateTokenFilePath(symlink)
	assert.Error(t, err, "symlink pointing outside allowed tree should be blocked")
	assert.Contains(t, err.Error(), "not under an allowed prefix")

	// A real file inside the allowed tree should still work.
	realFile := filepath.Join(allowedDir, "token")
	require.NoError(t, os.WriteFile(realFile, []byte("tok"), 0o600))
	assert.NoError(t, ValidateTokenFilePath(realFile), "real file inside allowed tree should pass")
}

func TestBuildSnowflakeConfig_WorkloadIdentity_TraversalBlocked(t *testing.T) {
	t.Parallel()

	pc := newPC()
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeWorkloadIdentity
	pc.Spec.Credentials = snowplanev1alpha1.ProviderCredentials{}
	pc.Spec.WorkloadIdentity = &snowplanev1alpha1.WorkloadIdentitySpec{
		TokenFilePath: "/etc/passwd",
	}

	_, err := BuildSnowflakeConfig(pc, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not under an allowed prefix")
}

func TestBuildSnowflakeConfig_SetsStatementTimeout(t *testing.T) {
	t.Parallel()

	cfg, err := BuildSnowflakeConfig(newPC(), newSecret(map[string][]byte{"password": []byte("s3cret")}))
	require.NoError(t, err)
	assert.Equal(t, 300, cfg.StatementTimeoutSeconds, "default StatementTimeoutSeconds should be 300 (5 minutes)")
}
