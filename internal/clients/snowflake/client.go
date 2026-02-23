// Package snowflake provides a client for interacting with Snowflake databases.
package snowflake

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"time"

	gosnowflake "github.com/snowflakedb/gosnowflake"
)

// Default connection pool settings.
const (
	DefaultMaxOpenConns    = 10
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = 30 * time.Minute
	DefaultPingTimeout     = 10 * time.Second
)

// Config holds all parameters needed to connect to a Snowflake account.
type Config struct {
	// Account is the Snowflake account identifier (e.g. "xy12345").
	Account string

	// Region is the Snowflake region (e.g. "us-east-1").
	Region string

	// User is the Snowflake username.
	User string

	// Password for username/password authentication. Mutually exclusive with PrivateKey and TokenFilePath.
	Password string

	// PrivateKey is the PEM-encoded RSA private key for key pair authentication.
	PrivateKey string

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

	// PingTimeout overrides the default timeout for health checks.
	PingTimeout time.Duration
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

func (c *Config) pingTimeout() time.Duration {
	if c.PingTimeout > 0 {
		return c.PingTimeout
	}

	return DefaultPingTimeout
}

// SQLExecutor defines the interface for executing SQL statements against Snowflake.
// Both pooled (*Client) and scoped (pinned-connection) clients satisfy this
// interface. Resource clients (DatabaseClient, SchemaClient, etc.) depend on
// this interface rather than the concrete *Client type.
type SQLExecutor interface {
	Exec(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
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
func NewClient(cfg Config) (*Client, error) {
	if cfg.Password == "" && cfg.PrivateKey == "" && cfg.TokenFilePath == "" {
		return nil, fmt.Errorf("either Password, PrivateKey, or TokenFilePath must be provided")
	}

	sfConfig := &gosnowflake.Config{
		Account:   cfg.Account,
		Region:    cfg.Region,
		User:      cfg.User,
		Role:      cfg.Role,
		Warehouse: cfg.Warehouse,
	}

	// Configure authentication.
	switch {
	case cfg.PrivateKey != "":
		key, err := parsePrivateKey(cfg.PrivateKey)
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
	case cfg.Password != "":
		sfConfig.Password = cfg.Password
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
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	if c.conn != nil {
		return c.conn.QueryRowContext(ctx, query, args...)
	}

	return c.db.QueryRowContext(ctx, query, args...)
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
	if err := conn.QueryRowContext(ctx, "SELECT CURRENT_ROLE()").Scan(&originalRole); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("querying current role: %w", err)
	}

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("USE ROLE %s", quoteIdentifier(role))); err != nil {
		_ = conn.Close()

		return nil, nil, fmt.Errorf("%w: %w", ErrRoleSwitchFailed, err)
	}

	scoped := &Client{
		db:          c.db,
		conn:        conn,
		account:     c.account,
		pingTimeout: c.pingTimeout,
	}

	cleanup := func(ctx context.Context) {
		// Restore the original role so the pooled connection is returned in a clean state.
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("USE ROLE %s", quoteIdentifier(originalRole)))
		_ = conn.Close()
	}

	return scoped, cleanup, nil
}

// parsePrivateKey decodes a PEM-encoded RSA private key.
// The key must be unencrypted — passphrase-protected keys are not yet supported.
func parsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	derBytes := block.Bytes

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
