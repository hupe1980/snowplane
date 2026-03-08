// Package snowflake provides a client for interacting with Snowflake databases.
package snowflake

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // G505: SHA-1 is the mandatory default PRF in PKCS#8/PBKDF2 (RFC 8018 §6.2)
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"log/slog"
	"time"

	gosnowflake "github.com/snowflakedb/gosnowflake"
	"golang.org/x/crypto/pbkdf2"
)

// Default connection pool settings.
const (
	// DefaultMaxOpenConns is the maximum number of open connections per
	// Snowflake provider. With maxConcurrentReconciles=3 across 22+
	// controllers sharing the same pool via WithRole, the pool must
	// accommodate concurrent pinned connections without starving.
	DefaultMaxOpenConns    = 30
	DefaultMaxIdleConns    = 10
	DefaultConnMaxLifetime = 30 * time.Minute
	DefaultConnMaxIdleTime = 5 * time.Minute
	DefaultPingTimeout     = 10 * time.Second
)

// Zeroize overwrites a byte slice with zeros, preventing credential data from
// lingering in memory after use. This is a best-effort defence — the Go
// runtime may have already copied the backing array during GC compaction, and
// the compiler is free to elide stores to "dead" memory. Nonetheless, zeroing
// reduces the window during which credentials are recoverable from a memory
// dump or core file.
func Zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// Config holds all parameters needed to connect to a Snowflake account.
type Config struct {
	// Account is the Snowflake account identifier (e.g. "xy12345").
	Account string

	// Region is the Snowflake region (e.g. "us-east-1").
	Region string

	// User is the Snowflake username.
	User string

	// Password for username/password authentication. Mutually exclusive with PrivateKey and TokenFilePath.
	// Stored as []byte for zeroization after use.
	Password []byte //nolint:gosec // G117: struct must hold credentials for Snowflake connection

	// PrivateKey is the PEM-encoded RSA private key for key pair authentication.
	// Stored as []byte for zeroization after use.
	PrivateKey []byte //nolint:gosec // G117: struct must hold credentials for Snowflake connection

	// Passphrase is the passphrase for decrypting an encrypted PKCS#8 private key.
	// Only used with PrivateKey when the PEM block type is "ENCRYPTED PRIVATE KEY".
	// Stored as []byte for zeroization after use.
	Passphrase []byte //nolint:gosec // G117: struct must hold credentials for Snowflake connection

	// TokenFilePath is the path to a file containing an OIDC/OAuth token.
	// Used with WorkloadIdentity authentication. The gosnowflake driver reads
	// this file on every new connection, providing automatic token rotation
	// when kubelet refreshes projected ServiceAccount tokens.
	TokenFilePath string

	// WorkloadIdentityProvider selects the cloud-specific WIF attestation provider.
	// Valid values: "OIDC", "AWS", "GCP", "Azure".
	WorkloadIdentityProvider string

	// Role is the Snowflake role to assume after connecting.
	Role string

	// Warehouse is the default warehouse for the session.
	Warehouse string

	// MaxOpenConns overrides the default maximum open connections.
	MaxOpenConns int

	// MaxIdleConns overrides the default maximum idle connections.
	MaxIdleConns int

	// ConnMaxLifetime overrides the default maximum connection lifetime.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime overrides the default maximum idle connection time.
	// Idle connections older than this are closed proactively.
	ConnMaxIdleTime time.Duration

	// PingTimeout overrides the default timeout for health checks.
	PingTimeout time.Duration

	// StatementTimeoutSeconds sets the Snowflake session parameter
	// STATEMENT_TIMEOUT_IN_SECONDS, which limits the maximum execution time
	// for any single SQL statement. When set, this is passed as a connection
	// parameter so every connection in the pool inherits the limit.
	// A value of 0 means no server-side timeout (Snowflake default).
	StatementTimeoutSeconds int
}

func (c *Config) maxOpenConns() int {
	if c.MaxOpenConns > 0 {
		return c.MaxOpenConns
	}

	return DefaultMaxOpenConns
}

func (c *Config) maxIdleConns() int {
	if c.MaxIdleConns > 0 {
		return c.MaxIdleConns
	}

	return DefaultMaxIdleConns
}

func (c *Config) connMaxLifetime() time.Duration {
	if c.ConnMaxLifetime > 0 {
		return c.ConnMaxLifetime
	}

	return DefaultConnMaxLifetime
}

func (c *Config) connMaxIdleTime() time.Duration {
	if c.ConnMaxIdleTime > 0 {
		return c.ConnMaxIdleTime
	}

	return DefaultConnMaxIdleTime
}

func (c *Config) pingTimeout() time.Duration {
	if c.PingTimeout > 0 {
		return c.PingTimeout
	}

	return DefaultPingTimeout
}

// ZeroizeCredentials overwrites Password, PrivateKey, and Passphrase with
// zeros so that plaintext credentials do not linger in process memory after
// the Snowflake DSN has been built.
func (c *Config) ZeroizeCredentials() {
	Zeroize(c.Password)
	Zeroize(c.PrivateKey)
	Zeroize(c.Passphrase)
}

// Row wraps *sql.Row and applies MapSnowflakeError to the Scan result.
// This ensures that Snowflake driver errors are automatically mapped to
// sentinel errors (e.g. ErrObjectNotExistOrNotAuthorized), matching the
// error-mapping behavior of Exec and Query.
//
// When err is set (via NewErrorRow), Scan and Err return that error
// without touching the underlying sql.Row. This allows test mocks to
// return a safe Row that reports a fixed error instead of returning nil.
type Row struct {
	row *sql.Row
	err error // preset error — returned by Scan/Err when non-nil
}

// NewErrorRow returns a Row that always returns the given error from
// Scan and Err. Use this in test mocks instead of returning nil from
// QueryRow, which would cause nil-pointer panics in callers that chain
// QueryRow(...).Scan(...).
func NewErrorRow(err error) *Row {
	return &Row{err: err}
}

// Scan delegates to the underlying sql.Row.Scan and maps any Snowflake
// driver error to the corresponding sentinel error. If the Row was
// created via NewErrorRow, the preset error is returned directly.
func (r *Row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	return MapSnowflakeError(r.row.Scan(dest...))
}

// Err delegates to the underlying sql.Row.Err. If the Row was created
// via NewErrorRow, the preset error is returned directly.
func (r *Row) Err() error {
	if r.err != nil {
		return r.err
	}

	return MapSnowflakeError(r.row.Err())
}

// SQLExecutor defines the interface for executing SQL statements against Snowflake.
// Both pooled (*Client) and scoped (pinned-connection) clients satisfy this
// interface. Resource clients (DatabaseClient, SchemaClient, etc.) depend on
// this interface rather than the concrete *Client type.
type SQLExecutor interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRow(ctx context.Context, query string, args ...any) *Row
	Query(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Compile-time check: *Client implements SQLExecutor.
var _ SQLExecutor = (*Client)(nil)

// Client wraps a sql.DB connection pool to a Snowflake account.
// When conn is set (via WithRole), all queries run on that pinned connection
// instead of the pool — this guarantees the session role is consistent.
type Client struct {
	db          *sql.DB
	conn        *sql.Conn // non-nil for scoped (pinned) clients
	account     string
	pingTimeout time.Duration
}

// NewClient creates a new Snowflake client and opens a connection pool.
// The caller should invoke Ping separately to verify connectivity.
// Credential fields in cfg are zeroed after the DSN is built so that
// plaintext secrets do not linger in process memory.
func NewClient(cfg Config) (*Client, error) {
	// Zeroize credential bytes once the DSN has been constructed (or on error).
	defer cfg.ZeroizeCredentials()

	if len(cfg.Password) == 0 && len(cfg.PrivateKey) == 0 && cfg.TokenFilePath == "" {
		return nil, fmt.Errorf("either Password, PrivateKey, or TokenFilePath must be provided")
	}

	sfConfig := &gosnowflake.Config{
		Account:   cfg.Account,
		Region:    cfg.Region,
		User:      cfg.User,
		Role:      cfg.Role,
		Warehouse: cfg.Warehouse,
	}

	// M8: Set STATEMENT_TIMEOUT_IN_SECONDS as a session parameter so that
	// Snowflake enforces a server-side timeout matching the Go-side context
	// timeout. This ensures long-running DDL is cancelled on the Snowflake
	// side, not just on the client side (which leaves the query consuming
	// warehouse credits until Snowflake detects the client disconnect).
	if cfg.StatementTimeoutSeconds > 0 {
		timeoutStr := fmt.Sprintf("%d", cfg.StatementTimeoutSeconds)
		sfConfig.Params = map[string]*string{
			"STATEMENT_TIMEOUT_IN_SECONDS": &timeoutStr,
		}
	}

	// Configure authentication.
	switch {
	case len(cfg.PrivateKey) > 0:
		key, err := parsePrivateKey(cfg.PrivateKey, cfg.Passphrase)
		if err != nil {
			return nil, fmt.Errorf("parsing private key: %w", err)
		}

		sfConfig.Authenticator = gosnowflake.AuthTypeJwt
		sfConfig.PrivateKey = key
	case cfg.TokenFilePath != "":
		// Native WIF: the driver reads the token file on each new connection
		// and handles the attestation/exchange with Snowflake automatically.
		sfConfig.Authenticator = gosnowflake.AuthTypeWorkloadIdentityFederation
		sfConfig.TokenFilePath = cfg.TokenFilePath
		sfConfig.WorkloadIdentityProvider = cfg.WorkloadIdentityProvider
	case len(cfg.Password) > 0:
		// gosnowflake.Config.Password is string (external dependency);
		// the conversion is unavoidable. The []byte source is zeroed by
		// the deferred ZeroizeCredentials above.
		sfConfig.Password = string(cfg.Password)
	}

	dsn, err := gosnowflake.DSN(sfConfig)
	if err != nil {
		return nil, fmt.Errorf("building DSN: %w", err)
	}

	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	db.SetMaxOpenConns(cfg.maxOpenConns())
	db.SetMaxIdleConns(cfg.maxIdleConns())
	db.SetConnMaxLifetime(cfg.connMaxLifetime())
	db.SetConnMaxIdleTime(cfg.connMaxIdleTime())

	client := &Client{
		db:          db,
		account:     cfg.Account,
		pingTimeout: cfg.pingTimeout(),
	}

	return client, nil
}

// Ping verifies the connection to Snowflake is alive.
func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, c.pingTimeout)
	defer cancel()

	if err := c.db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	return nil
}

// Exec executes a SQL statement that does not return rows.
// When a pinned connection is available (via WithRole), it is used instead of the pool.
// Snowflake driver errors are automatically mapped to sentinel errors.
func (c *Client) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	var err error

	if c.conn != nil {
		result, err = c.conn.ExecContext(ctx, query, args...)
	} else {
		result, err = c.db.ExecContext(ctx, query, args...)
	}

	return result, MapSnowflakeError(err)
}

// QueryRow executes a query that returns at most one row.
// When a pinned connection is available (via WithRole), it is used instead of the pool.
// Snowflake driver errors are automatically mapped to sentinel errors when Scan() is called.
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) *Row {
	if c.conn != nil {
		return &Row{row: c.conn.QueryRowContext(ctx, query, args...)}
	}

	return &Row{row: c.db.QueryRowContext(ctx, query, args...)}
}

// Query executes a query that returns rows.
// When a pinned connection is available (via WithRole), it is used instead of the pool.
// Snowflake driver errors are automatically mapped to sentinel errors.
func (c *Client) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	var rows *sql.Rows
	var err error

	if c.conn != nil {
		rows, err = c.conn.QueryContext(ctx, query, args...)
	} else {
		rows, err = c.db.QueryContext(ctx, query, args...)
	}

	return rows, MapSnowflakeError(err)
}

// Close releases all resources held by the client.
// For scoped clients (created via WithRole), this is a no-op — cleanup is handled
// by the function returned from WithRole.
func (c *Client) Close() error {
	if c.conn != nil {
		// Scoped client — don't close the underlying pool.
		return nil
	}

	if c.db != nil {
		return c.db.Close()
	}

	return nil
}

// Account returns the Snowflake account identifier this client is connected to.
func (c *Client) Account() string {
	return c.account
}

// Stats returns the underlying sql.DB connection pool statistics.
// Returns a zero value if the client has no pool (scoped client created via WithRole).
func (c *Client) Stats() sql.DBStats {
	if c.db != nil {
		return c.db.Stats()
	}

	return sql.DBStats{}
}

// UseRole switches the session to the specified role.
// CAUTION: On a pooled connection (conn == nil) this affects a random connection
// and may not be seen by subsequent queries. Prefer WithRole for safe scoping.
func (c *Client) UseRole(ctx context.Context, role string) error {
	if _, err := c.Exec(ctx, fmt.Sprintf("USE ROLE %s", quoteIdentifier(role))); err != nil {
		return fmt.Errorf("switching to role %q: %w", role, err)
	}

	return nil
}

// CurrentRole returns the current session role.
func (c *Client) CurrentRole(ctx context.Context) (string, error) {
	var role string
	if err := c.QueryRow(ctx, "SELECT CURRENT_ROLE()").Scan(&role); err != nil {
		return "", fmt.Errorf("querying current role: %w", err)
	}

	return role, nil
}

// WithRole pins a connection from the pool, switches it to the given role,
// and returns a new *Client that executes all queries on that pinned connection.
// The returned cleanup function restores the original role and releases the connection.
//
// Usage:
//
//	scoped, cleanup, err := client.WithRole(ctx, "DATA_ADMIN")
//	if err != nil { return err }
//	defer cleanup(ctx)
//	// All operations on scoped use the "DATA_ADMIN" role.
func (c *Client) WithRole(ctx context.Context, role string) (*Client, func(context.Context), error) {
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("pinning connection: %w", err)
	}

	// Save current role so we can restore it before returning the connection to the pool.
	var originalRole string
	if err := MapSnowflakeError(conn.QueryRowContext(ctx, "SELECT CURRENT_ROLE()").Scan(&originalRole)); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			slog.Debug("error returning connection to pool after role query failure", "error", closeErr)
		}

		return nil, nil, fmt.Errorf("querying current role: %w", err)
	}

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("USE ROLE %s", quoteIdentifier(role))); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			slog.Debug("error returning connection to pool after role switch failure", "error", closeErr)
		}

		return nil, nil, fmt.Errorf("%w: %w", ErrRoleSwitchFailed, err)
	}

	scoped := &Client{
		db:          c.db,
		conn:        conn,
		account:     c.account,
		pingTimeout: c.pingTimeout,
	}

	cleanup := func(ctx context.Context) {
		// Use a detached context so role restoration completes even when the
		// parent context is cancelled (e.g., reconciler returned early).
		// Without this, a cancelled context would cause the USE ROLE restore
		// to fail, leaving the pooled connection with the wrong role active.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		// Restore the original role so the pooled connection is returned in a clean state.
		if _, err := conn.ExecContext(cleanupCtx, fmt.Sprintf("USE ROLE %s", quoteIdentifier(originalRole))); err != nil {
			slog.Debug("error restoring role on pooled connection", "role", originalRole, "error", err)
		}

		if err := conn.Close(); err != nil {
			slog.Debug("error returning connection to pool", "error", err)
		}
	}

	return scoped, cleanup, nil
}

// ASN.1 OIDs for PKCS#8 encrypted private key decryption (PBES2/PBKDF2/AES-CBC).
var (
	oidPBES2          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidAES128CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES256CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

// ASN.1 structures for PKCS#8 encrypted private key parsing.
type pbes2Params struct {
	KeyDerivationFunc pkixAlgorithmIdentifier
	EncryptionScheme  pkixAlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt           []byte
	IterationCount int
	KeyLength      int                     `asn1:"optional"`
	PRF            pkixAlgorithmIdentifier `asn1:"optional"`
}

type pkixAlgorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type encryptedPrivateKeyInfo struct {
	EncryptionAlgorithm pkixAlgorithmIdentifier
	EncryptedData       []byte
}

// decryptPKCS8PrivateKey decrypts an encrypted PKCS#8 private key (RFC 5958).
// Supports PBES2 with PBKDF2 key derivation and AES-128-CBC or AES-256-CBC
// encryption, with HMAC-SHA1 or HMAC-SHA256 as the PRF.
//
// This covers keys generated by OpenSSL commands such as:
//
//	openssl genrsa 2048 | openssl pkcs8 -topk8 -v2 aes-256-cbc
//
// which is the format recommended by Snowflake for key pair authentication.
// Go's stdlib does not provide a ParseEncryptedPKCS8PrivateKey function,
// so we implement the ASN.1 parsing and AES-CBC decryption directly using
// encoding/asn1 + golang.org/x/crypto/pbkdf2.
func decryptPKCS8PrivateKey(der, passphrase []byte) ([]byte, error) {
	var epki encryptedPrivateKeyInfo
	if _, err := asn1.Unmarshal(der, &epki); err != nil {
		return nil, fmt.Errorf("parse encrypted PKCS#8 envelope: %w", err)
	}

	if !epki.EncryptionAlgorithm.Algorithm.Equal(oidPBES2) {
		return nil, fmt.Errorf("unsupported key encryption scheme: %v (only PBES2 is supported)", epki.EncryptionAlgorithm.Algorithm)
	}

	var pbes pbes2Params
	if _, err := asn1.Unmarshal(epki.EncryptionAlgorithm.Parameters.FullBytes, &pbes); err != nil {
		return nil, fmt.Errorf("parse PBES2 parameters: %w", err)
	}

	if !pbes.KeyDerivationFunc.Algorithm.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("unsupported KDF: %v (only PBKDF2 is supported)", pbes.KeyDerivationFunc.Algorithm)
	}

	var kdfp pbkdf2Params
	if _, err := asn1.Unmarshal(pbes.KeyDerivationFunc.Parameters.FullBytes, &kdfp); err != nil {
		return nil, fmt.Errorf("parse PBKDF2 parameters: %w", err)
	}

	var hashFunc func() hash.Hash

	switch {
	case kdfp.PRF.Algorithm == nil, kdfp.PRF.Algorithm.Equal(oidHMACWithSHA1):
		hashFunc = sha1.New //nolint:gosec // SHA-1 is the PKCS#8 default PRF, not a security choice
	case kdfp.PRF.Algorithm.Equal(oidHMACWithSHA256):
		hashFunc = sha256.New
	default:
		return nil, fmt.Errorf("unsupported PRF: %v (only HMAC-SHA1 and HMAC-SHA256 are supported)", kdfp.PRF.Algorithm)
	}

	var keyLen int

	switch {
	case pbes.EncryptionScheme.Algorithm.Equal(oidAES128CBC):
		keyLen = 16
	case pbes.EncryptionScheme.Algorithm.Equal(oidAES256CBC):
		keyLen = 32
	default:
		return nil, fmt.Errorf("unsupported cipher: %v (only AES-128-CBC and AES-256-CBC are supported)", pbes.EncryptionScheme.Algorithm)
	}

	var iv []byte
	if _, err := asn1.Unmarshal(pbes.EncryptionScheme.Parameters.FullBytes, &iv); err != nil {
		return nil, fmt.Errorf("parse cipher IV: %w", err)
	}

	key := pbkdf2.Key(passphrase, kdfp.Salt, kdfp.IterationCount, keyLen, hashFunc)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	if len(iv) != aes.BlockSize {
		return nil, errors.New("IV size does not match AES block size")
	}

	if len(epki.EncryptedData)%aes.BlockSize != 0 {
		return nil, errors.New("encrypted data is not a multiple of AES block size")
	}

	plaintext := make([]byte, len(epki.EncryptedData))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, epki.EncryptedData)

	// PKCS#7 unpadding.
	if len(plaintext) == 0 {
		return nil, errors.New("decrypted data is empty")
	}

	pad := int(plaintext[len(plaintext)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(plaintext) {
		return nil, errors.New("invalid passphrase or corrupted key")
	}

	for _, b := range plaintext[len(plaintext)-pad:] {
		if int(b) != pad {
			return nil, errors.New("invalid passphrase or corrupted key")
		}
	}

	return plaintext[:len(plaintext)-pad], nil
}

// parsePrivateKey decodes a PEM-encoded RSA private key.
// It supports unencrypted PKCS#8, PKCS#1, and passphrase-encrypted PKCS#8 keys.
// For encrypted keys (PEM type "ENCRYPTED PRIVATE KEY"), a non-empty passphrase
// is required. The passphrase slice is zeroed after use.
func parsePrivateKey(pemData []byte, passphrase []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	derBytes := block.Bytes

	// Handle encrypted PKCS#8 keys (BEGIN ENCRYPTED PRIVATE KEY).
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		if len(passphrase) == 0 {
			return nil, fmt.Errorf("encrypted private key requires a passphrase")
		}

		decrypted, err := decryptPKCS8PrivateKey(derBytes, passphrase)
		if err != nil {
			return nil, fmt.Errorf("decrypting private key: %w", err)
		}

		// Zero decrypted DER bytes after parsing to avoid lingering
		// private key material on the heap.
		defer Zeroize(decrypted)

		derBytes = decrypted
	}

	// Try PKCS#8 first, then PKCS#1.
	if key, err := x509.ParsePKCS8PrivateKey(derBytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}

		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(derBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key (tried PKCS8 and PKCS1): %w", err)
	}

	return key, nil
}
