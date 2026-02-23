package snowflake

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := Config{}
	assert.Equal(t, DefaultMaxOpenConns, cfg.maxOpenConns())
	assert.Equal(t, DefaultMaxIdleConns, cfg.maxIdleConns())
	assert.Equal(t, DefaultConnMaxLifetime, cfg.connMaxLifetime())
	assert.Equal(t, DefaultPingTimeout, cfg.pingTimeout())
}

func TestConfigOverrides(t *testing.T) {
	t.Parallel()

	cfg := Config{
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: time.Hour,
		PingTimeout:     30 * time.Second,
	}

	assert.Equal(t, 20, cfg.maxOpenConns())
	assert.Equal(t, 10, cfg.maxIdleConns())
	assert.Equal(t, time.Hour, cfg.connMaxLifetime())
	assert.Equal(t, 30*time.Second, cfg.pingTimeout())
}

func TestParsePrivateKey_PKCS8_Unencrypted(t *testing.T) {
	t.Parallel()

	pemStr := generateTestPKCS8PEM(t)
	key, err := parsePrivateKey(pemStr)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_PKCS1(t *testing.T) {
	t.Parallel()

	pemStr := generateTestPKCS1PEM(t)
	key, err := parsePrivateKey(pemStr)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_InvalidPEM(t *testing.T) {
	t.Parallel()

	_, err := parsePrivateKey("not-a-pem")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM block")
}

func TestParsePrivateKey_InvalidDER(t *testing.T) {
	t.Parallel()

	badPEM := "-----BEGIN PRIVATE KEY-----\nYWJj\n-----END PRIVATE KEY-----"
	_, err := parsePrivateKey(badPEM)
	require.Error(t, err)
}

func TestNewClient_NoCredentials(t *testing.T) {
	t.Parallel()

	_, err := NewClient(Config{
		Account: "acct",
		User:    "user",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either Password, PrivateKey, or TokenFilePath must be provided")
}

// generateTestPKCS8PEM creates a PEM-encoded PKCS#8 RSA key for testing.
func generateTestPKCS8PEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}))
}

// generateTestPKCS1PEM creates a PEM-encoded PKCS#1 RSA key for testing.
func generateTestPKCS1PEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der := x509.MarshalPKCS1PrivateKey(key)

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	}))
}
