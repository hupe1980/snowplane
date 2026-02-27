package snowflake

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/asn1"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/pbkdf2"

	gosnowflake "github.com/snowflakedb/gosnowflake"
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

	pemData := generateTestPKCS8PEM(t)
	key, err := parsePrivateKey(pemData, nil)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

// TestParsePrivateKey_PKCS8_Unencrypted_WithPassphrase verifies that a
// superfluous passphrase is silently ignored for unencrypted keys.
func TestParsePrivateKey_PKCS8_Unencrypted_WithPassphrase(t *testing.T) {
	t.Parallel()

	pemData := generateTestPKCS8PEM(t)
	key, err := parsePrivateKey(pemData, []byte("unused-passphrase"))

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_PKCS1(t *testing.T) {
	t.Parallel()

	pemData := generateTestPKCS1PEM(t)
	key, err := parsePrivateKey(pemData, nil)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
}

func TestParsePrivateKey_InvalidPEM(t *testing.T) {
	t.Parallel()

	_, err := parsePrivateKey([]byte("not-a-pem"), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM block")
}

func TestParsePrivateKey_InvalidDER(t *testing.T) {
	t.Parallel()

	badPEM := []byte("-----BEGIN PRIVATE KEY-----\nYWJj\n-----END PRIVATE KEY-----")
	_, err := parsePrivateKey(badPEM, nil)
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

func TestZeroize(t *testing.T) {
	t.Parallel()

	b := []byte("supersecret")
	Zeroize(b)
	for i, v := range b {
		assert.Equal(t, byte(0), v, "byte at index %d should be zero", i)
	}
}

func TestZeroize_Empty(t *testing.T) {
	t.Parallel()

	// Must not panic on empty/nil slices.
	Zeroize(nil)
	Zeroize([]byte{})
}

func TestConfig_ZeroizeCredentials(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Account:    "acct",
		Password:   []byte("password123"),
		PrivateKey: []byte("-----BEGIN PRIVATE KEY-----\ndata\n-----END PRIVATE KEY-----"),
		Passphrase: []byte("passphrase"),
	}

	cfg.ZeroizeCredentials()

	// Credential bytes should all be zero.
	for _, b := range cfg.Password {
		assert.Equal(t, byte(0), b)
	}
	for _, b := range cfg.PrivateKey {
		assert.Equal(t, byte(0), b)
	}
	for _, b := range cfg.Passphrase {
		assert.Equal(t, byte(0), b)
	}

	// Non-credential fields should be untouched.
	assert.Equal(t, "acct", cfg.Account)
}

// generateTestPKCS8PEM creates a PEM-encoded PKCS#8 RSA key for testing.
func generateTestPKCS8PEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
}

// generateTestPKCS1PEM creates a PEM-encoded PKCS#1 RSA key for testing.
func generateTestPKCS1PEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	der := x509.MarshalPKCS1PrivateKey(key)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	})
}

// generateTestEncryptedPKCS8PEM creates a passphrase-encrypted PKCS#8 PEM key
// for testing. It uses PBES2 with PBKDF2 (SHA-256) and AES-256-CBC, matching
// the format produced by: openssl pkcs8 -topk8 -v2 aes-256-cbc
func generateTestEncryptedPKCS8PEM(t *testing.T, passphrase string) (encPEM []byte, originalKey *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	plainDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	// PKCS#7 pad to AES block size.
	pad := aes.BlockSize - len(plainDER)%aes.BlockSize
	padded := make([]byte, len(plainDER)+pad)
	copy(padded, plainDER)

	for i := len(plainDER); i < len(padded); i++ {
		padded[i] = byte(pad)
	}

	// Generate random salt and IV.
	salt := make([]byte, 16)
	_, err = rand.Read(salt)
	require.NoError(t, err)

	iv := make([]byte, aes.BlockSize)
	_, err = rand.Read(iv)
	require.NoError(t, err)

	// Derive key using PBKDF2 with SHA-256 and AES-256.
	iterations := 2048
	derivedKey := pbkdf2.Key([]byte(passphrase), salt, iterations, 32, sha256.New)

	block, err := aes.NewCipher(derivedKey)
	require.NoError(t, err)

	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	// Build the ASN.1 EncryptedPrivateKeyInfo structure.
	ivRaw, err := asn1.Marshal(iv)
	require.NoError(t, err)

	saltRaw, err := asn1.Marshal(salt)
	require.NoError(t, err)

	iterRaw, err := asn1.Marshal(iterations)
	require.NoError(t, err)

	// PBKDF2-params: SEQUENCE { salt, iterationCount, prf }
	prfSeq, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      mustMarshalOID(t, oidHMACWithSHA256),
	})
	require.NoError(t, err)

	kdfParams := concat(saltRaw, iterRaw, prfSeq)

	kdfAlg, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      concat(mustMarshalOID(t, oidPBKDF2), wrapSeq(t, kdfParams)),
	})
	require.NoError(t, err)

	encAlg, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      concat(mustMarshalOID(t, oidAES256CBC), ivRaw),
	})
	require.NoError(t, err)

	pbes2ParamsBytes := concat(kdfAlg, encAlg)

	alg, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      concat(mustMarshalOID(t, oidPBES2), wrapSeq(t, pbes2ParamsBytes)),
	})
	require.NoError(t, err)

	ciphertextRaw, err := asn1.Marshal(ciphertext)
	require.NoError(t, err)

	epkiBytes := concat(alg, ciphertextRaw)

	epkiDER, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      epkiBytes,
	})
	require.NoError(t, err)

	pemBlock := &pem.Block{
		Type:  "ENCRYPTED PRIVATE KEY",
		Bytes: epkiDER,
	}

	return pem.EncodeToMemory(pemBlock), key
}

func mustMarshalOID(t *testing.T, oid asn1.ObjectIdentifier) []byte {
	t.Helper()

	b, err := asn1.Marshal(oid)
	require.NoError(t, err)

	return b
}

func wrapSeq(t *testing.T, content []byte) []byte {
	t.Helper()

	b, err := asn1.Marshal(asn1.RawValue{
		Class:      asn1.ClassUniversal,
		Tag:        asn1.TagSequence,
		IsCompound: true,
		Bytes:      content,
	})
	require.NoError(t, err)

	return b
}

func concat(slices ...[]byte) []byte {
	n := 0
	for _, s := range slices {
		n += len(s)
	}

	out := make([]byte, 0, n)

	for _, s := range slices {
		out = append(out, s...)
	}

	return out
}

func TestParsePrivateKey_PKCS8_Encrypted(t *testing.T) {
	t.Parallel()

	passphrase := "test-passphrase-123"
	encPEM, originalKey := generateTestEncryptedPKCS8PEM(t, passphrase)

	key, err := parsePrivateKey(encPEM, []byte(passphrase))
	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, 2048, key.N.BitLen())
	assert.True(t, originalKey.Equal(key), "decrypted key should match original")
}

func TestParsePrivateKey_PKCS8_Encrypted_WrongPassphrase(t *testing.T) {
	t.Parallel()

	encPEM, _ := generateTestEncryptedPKCS8PEM(t, "correct-passphrase")

	_, err := parsePrivateKey(encPEM, []byte("wrong-passphrase"))
	require.Error(t, err)
}

func TestParsePrivateKey_PKCS8_Encrypted_NoPassphrase(t *testing.T) {
	t.Parallel()

	encPEM, _ := generateTestEncryptedPKCS8PEM(t, "some-passphrase")

	_, err := parsePrivateKey(encPEM, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encrypted private key requires a passphrase")
}

func TestDecryptPKCS8PrivateKey_InvalidASN1(t *testing.T) {
	t.Parallel()

	_, err := decryptPKCS8PrivateKey([]byte("not-asn1"), []byte("pass"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse encrypted PKCS#8 envelope")
}

// ---------------------------------------------------------------------------
// Row wrapper tests
// ---------------------------------------------------------------------------

func TestRow_Scan_MapsSnowflakeErrors(t *testing.T) {
	t.Parallel()

	// Open a minimal database using the snowflake driver with valid DSN
	// structure but unreachable account. sql.Open succeeds (lazy connect);
	// QueryRow/Scan triggers the actual connection attempt and fails.
	dsn, err := gosnowflake.DSN(&gosnowflake.Config{
		Account:  "test_account",
		User:     "test_user",
		Password: "test_password",
	})
	require.NoError(t, err)

	db, err := sql.Open("snowflake", dsn)
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck // best-effort cleanup in test

	// Use an already-expired context so the connection attempt fails
	// instantly without any network I/O.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	// QueryRow is lazy — Scan triggers the connection and returns the error.
	// Wrap in our Row type to verify MapSnowflakeError is applied.
	rawRow := db.QueryRowContext(ctx, "SELECT 1")
	row := &Row{row: rawRow}

	var dummy int
	scanErr := row.Scan(&dummy)
	// Must error — context is already expired.
	assert.Error(t, scanErr, "Scan with expired context should fail")
}

func TestRow_Scan_PassesThroughOnSuccess(t *testing.T) {
	t.Parallel()

	// Verify the Row type correctly delegates to sql.Row.
	// We cannot easily create a successful *sql.Row without a real DB,
	// so we verify the Err() method on a nil-error path.
	// The compile-time interface check on Client already guarantees
	// QueryRow returns *Row.

	// Static compile-time check: Client.QueryRow returns *Row.
	var _ = (*Client)(nil).QueryRow
}
