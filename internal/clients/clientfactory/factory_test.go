package clientfactory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

type fakeClient struct {
	closed atomic.Bool
}

func (f *fakeClient) Ping(_ context.Context) error { return nil }
func (f *fakeClient) Close() error {
	f.closed.Store(true)
	return nil
}
func (f *fakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (f *fakeClient) QueryRow(_ context.Context, _ string, _ ...any) *sql.Row { return nil }
func (f *fakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, nil, nil
}

type unhealthyFakeClient struct {
	fakeClient
}

func (u *unhealthyFakeClient) Ping(_ context.Context) error {
	return errors.New("connection refused")
}

func newFakeFactory() (*ClientFactory, *[]*fakeClient) {
	created := make([]*fakeClient, 0)
	factory := NewTestClientFactoryWithFn(func(_ snowflake.Config) (SnowflakeClient, error) {
		c := &fakeClient{}
		created = append(created, c)
		return c, nil
	})
	return factory, &created
}

func TestGetOrCreate_NewClient(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	client, err := factory.GetOrCreate("default", "hash1", snowflake.Config{})
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Len(t, *created, 1)
}

func TestGetOrCreate_CacheHit(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate("default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate("default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	assert.Same(t, c1, c2)
	assert.Len(t, *created, 1)
}

func TestGetOrCreate_HashChange(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate("default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate("default", "hash2", snowflake.Config{})
	require.NoError(t, err)

	assert.NotSame(t, c1, c2)
	assert.Len(t, *created, 2)
	assert.True(t, (*created)[0].closed.Load(), "old client should be closed")
}

func TestGetOrCreate_MultipleProviders(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate("prov-a", "h1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate("prov-b", "h1", snowflake.Config{})
	require.NoError(t, err)

	assert.NotSame(t, c1, c2)
	assert.Len(t, *created, 2)
}

func TestGetOrCreate_ConstructorError(t *testing.T) {
	t.Parallel()
	factory := NewTestClientFactoryWithFn(func(_ snowflake.Config) (SnowflakeClient, error) {
		return nil, errors.New("connection refused")
	})

	c, err := factory.GetOrCreate("default", "h1", snowflake.Config{})
	assert.Nil(t, c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestEvict(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	_, err := factory.GetOrCreate("default", "h1", snowflake.Config{})
	require.NoError(t, err)

	factory.Evict("default")

	assert.True(t, (*created)[0].closed.Load())

	// Next GetOrCreate should create a new client.
	_, err = factory.GetOrCreate("default", "h1", snowflake.Config{})
	require.NoError(t, err)
	assert.Len(t, *created, 2)
}

func TestEvict_NoOp(t *testing.T) {
	t.Parallel()
	factory, _ := newFakeFactory()
	defer factory.Close()
	factory.Evict("nonexistent") // should not panic
}

func TestClose(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()

	_, err := factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("b", "h2", snowflake.Config{})
	require.NoError(t, err)

	factory.Close()

	for i, c := range *created {
		assert.True(t, c.closed.Load(), "client %d should be closed", i)
	}
}

func TestCheckHealth_NoClients(t *testing.T) {
	t.Parallel()
	factory, _ := newFakeFactory()
	defer factory.Close()

	// No clients cached — health check should pass.
	err := factory.CheckHealth(nil)
	assert.NoError(t, err)
}

func TestCheckHealth_AllHealthy(t *testing.T) {
	t.Parallel()
	factory, _ := newFakeFactory()
	defer factory.Close()

	_, err := factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Both fakeClients return nil from Ping — health check should pass.
	err = factory.CheckHealth(nil)
	assert.NoError(t, err)
}

func TestCheckHealth_OneUnhealthy(t *testing.T) {
	t.Parallel()

	callCount := 0
	factory := NewTestClientFactoryWithFn(func(_ snowflake.Config) (SnowflakeClient, error) {
		callCount++
		if callCount == 2 {
			return &unhealthyFakeClient{}, nil
		}

		return &fakeClient{}, nil
	})
	defer factory.Close()

	_, err := factory.GetOrCreate("healthy", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("unhealthy", "h2", snowflake.Config{})
	require.NoError(t, err)

	err = factory.CheckHealth(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectivity check failed")
}

// --------------------------------------------------------------------------
// Tests: LRU eviction and max size (L-18)
// --------------------------------------------------------------------------

func TestGetOrCreate_MaxSizeEvictsLRU(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithMaxSize(2)
	defer factory.Close()

	_, err := factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Adding a third should evict "a" (least recently used).
	_, err = factory.GetOrCreate("c", "h3", snowflake.Config{})
	require.NoError(t, err)

	assert.Len(t, *created, 3)
	assert.True(t, (*created)[0].closed.Load(), "LRU victim 'a' should be closed")
	assert.False(t, (*created)[1].closed.Load(), "'b' should still be open")
	assert.False(t, (*created)[2].closed.Load(), "'c' should still be open")
}

func TestGetOrCreate_LRUOrderUpdatedOnAccess(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithMaxSize(2)
	defer factory.Close()

	_, err := factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Access "a" to move it to the end of LRU order.
	_, err = factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	// Adding "c" should now evict "b" (LRU), not "a".
	_, err = factory.GetOrCreate("c", "h3", snowflake.Config{})
	require.NoError(t, err)

	assert.Len(t, *created, 3)
	assert.False(t, (*created)[0].closed.Load(), "'a' was recently accessed — should still be open")
	assert.True(t, (*created)[1].closed.Load(), "'b' was LRU victim — should be closed")
	assert.False(t, (*created)[2].closed.Load(), "'c' should still be open")
}

func TestGetOrCreate_MaxSizeZeroUnlimited(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	// maxSize=0 is the default — no limit.
	defer factory.Close()

	for i := range 100 {
		_, err := factory.GetOrCreate(fmt.Sprintf("p%d", i), fmt.Sprintf("h%d", i), snowflake.Config{})
		require.NoError(t, err)
	}

	assert.Len(t, *created, 100)
	for _, c := range *created {
		assert.False(t, c.closed.Load(), "no client should be evicted when maxSize=0")
	}
}

func TestEvict_RemovesFromLRUOrder(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithMaxSize(2)
	defer factory.Close()

	_, err := factory.GetOrCreate("a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Evict "a" explicitly.
	factory.Evict("a")
	assert.True(t, (*created)[0].closed.Load())

	// Now adding "c" should NOT evict "b" since we only have 1 client.
	_, err = factory.GetOrCreate("c", "h3", snowflake.Config{})
	require.NoError(t, err)

	assert.False(t, (*created)[1].closed.Load(), "'b' should still be open")
	assert.False(t, (*created)[2].closed.Load(), "'c' should still be open")
}

// ---------------------------------------------------------------------------
// Tests: Idle TTL (M-5)
// ---------------------------------------------------------------------------

func TestGetOrCreate_IdleTTL_EvictsExpiredClient(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithIdleTTL(1) // 1 nanosecond — effectively always expired
	defer factory.Close()

	// First call — creates client.
	c1, err := factory.GetOrCreate("p", "h1", snowflake.Config{})
	require.NoError(t, err)
	require.NotNil(t, c1)

	// Second call with same hash — client should be evicted due to TTL.
	c2, err := factory.GetOrCreate("p", "h1", snowflake.Config{})
	require.NoError(t, err)
	require.NotNil(t, c2)

	// The first client should have been closed.
	assert.True(t, (*created)[0].closed.Load(), "expired client should be closed")
	// A new (second) client should have been created.
	assert.Len(t, *created, 2, "should create a new client after TTL expiry")
}

func TestGetOrCreate_IdleTTL_Zero_NeverExpires(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	// idleTTL = 0 (default) — no TTL check.
	defer factory.Close()

	_, err := factory.GetOrCreate("p", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate("p", "h1", snowflake.Config{})
	require.NoError(t, err)

	// Should reuse the same client — only 1 created.
	assert.Len(t, *created, 1)
	assert.False(t, (*created)[0].closed.Load())
}
