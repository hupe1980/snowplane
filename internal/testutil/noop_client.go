package testutil

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// errNoopClient is returned by NoopSnowflakeClient SQL methods to simulate
// a client that is not connected to a real Snowflake instance.
var errNoopClient = fmt.Errorf("noop test client: not connected to Snowflake")

// NoopSnowflakeClient implements clientfactory.SnowflakeClient with no-op
// behavior. SQL methods return errNoopClient (non-definitive error), which
// the reconciler's pre-flight checks classify as "skip" rather than "not found".
//
// Use NewTestClientFactory() to create a ClientFactory backed by this client.
type NoopSnowflakeClient struct{}

// Ping is a no-op health check.
func (c *NoopSnowflakeClient) Ping(_ context.Context) error { return nil }

// Close is a no-op connection close.
func (c *NoopSnowflakeClient) Close() error { return nil }

// Exec returns errNoopClient to simulate a disconnected client.
func (c *NoopSnowflakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, errNoopClient
}

// QueryRow returns an error row wrapping errNoopClient.
func (c *NoopSnowflakeClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return snowflake.NewErrorRow(errNoopClient)
}

// Query returns errNoopClient to simulate a disconnected client.
func (c *NoopSnowflakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, errNoopClient
}

// WithRole returns errNoopClient since role switching requires a real connection.
func (c *NoopSnowflakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, func(context.Context) {}, errNoopClient
}

// NewTestClientFactory returns a ClientFactory backed by NoopSnowflakeClient.
// This eliminates real Snowflake connection attempts in unit tests, making
// tests faster, deterministic, and free of gosnowflake authentication noise.
//
// The NoopSnowflakeClient returns non-definitive errors from Query/Exec,
// which the reconciler's auto pre-flight correctly classifies as "skip"
// (not a definitive "object not found").
func NewTestClientFactory() *clientfactory.ClientFactory {
	return clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &NoopSnowflakeClient{}, nil
	})
}
