package clientfactory

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
func (f *fakeClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return snowflake.NewErrorRow(fmt.Errorf("fake: no real connection"))
}
func (f *fakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, fmt.Errorf("fake: no real connection")
}
func (f *fakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, func(context.Context) {}, fmt.Errorf("fake: no real connection")
}

type unhealthyFakeClient struct {
	fakeClient
}

func (u *unhealthyFakeClient) Ping(_ context.Context) error {
	return errors.New("connection refused")
}

func newFakeFactory() (*ClientFactory, *[]*fakeClient) {
	created := make([]*fakeClient, 0)
	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
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

	client, err := factory.GetOrCreate(context.Background(), "default", "hash1", snowflake.Config{})
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Len(t, *created, 1)
}

func TestGetOrCreate_CacheHit(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate(context.Background(), "default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate(context.Background(), "default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	assert.Same(t, c1, c2)
	assert.Len(t, *created, 1)
}

func TestGetOrCreate_HashChange(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate(context.Background(), "default", "hash1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate(context.Background(), "default", "hash2", snowflake.Config{})
	require.NoError(t, err)

	assert.NotSame(t, c1, c2)
	assert.Len(t, *created, 2)
	assert.True(t, (*created)[0].closed.Load(), "old client should be closed")
}

func TestGetOrCreate_MultipleProviders(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	c1, err := factory.GetOrCreate(context.Background(), "prov-a", "h1", snowflake.Config{})
	require.NoError(t, err)

	c2, err := factory.GetOrCreate(context.Background(), "prov-b", "h1", snowflake.Config{})
	require.NoError(t, err)

	assert.NotSame(t, c1, c2)
	assert.Len(t, *created, 2)
}

func TestGetOrCreate_ConstructorError(t *testing.T) {
	t.Parallel()
	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		return nil, errors.New("connection refused")
	})

	c, err := factory.GetOrCreate(context.Background(), "default", "h1", snowflake.Config{})
	assert.Nil(t, c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestEvict(t *testing.T) {
	t.Parallel()
	factory, created := newFakeFactory()
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "default", "h1", snowflake.Config{})
	require.NoError(t, err)

	factory.Evict("default")

	assert.True(t, (*created)[0].closed.Load())

	// Next GetOrCreate should create a new client.
	_, err = factory.GetOrCreate(context.Background(), "default", "h1", snowflake.Config{})
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

	_, err := factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "b", "h2", snowflake.Config{})
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

	_, err := factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Both fakeClients return nil from Ping — health check should pass.
	err = factory.CheckHealth(nil)
	assert.NoError(t, err)
}

func TestCheckHealth_OneUnhealthy(t *testing.T) {
	t.Parallel()

	callCount := 0
	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		callCount++
		if callCount == 2 {
			return &unhealthyFakeClient{}, nil
		}

		return &fakeClient{}, nil
	})
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "healthy", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "unhealthy", "h2", snowflake.Config{})
	require.NoError(t, err)

	err = factory.CheckHealth(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectivity check failed")
	assert.Contains(t, err.Error(), "unhealthy")
}

func TestCheckHealth_MultipleUnhealthy(t *testing.T) {
	t.Parallel()

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		return &unhealthyFakeClient{}, nil
	})
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "sick-a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "sick-b", "h2", snowflake.Config{})
	require.NoError(t, err)

	err = factory.CheckHealth(nil)
	assert.Error(t, err)
	// Both providers should appear in the error message.
	assert.Contains(t, err.Error(), "sick-a")
	assert.Contains(t, err.Error(), "sick-b")
}

// --------------------------------------------------------------------------
// Tests: LRU eviction and max size
// --------------------------------------------------------------------------

func TestGetOrCreate_MaxSizeEvictsLRU(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithMaxSize(2)
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Adding a third should evict "a" (least recently used).
	_, err = factory.GetOrCreate(context.Background(), "c", "h3", snowflake.Config{})
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

	_, err := factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Access "a" to move it to the end of LRU order.
	_, err = factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	// Adding "c" should now evict "b" (LRU), not "a".
	_, err = factory.GetOrCreate(context.Background(), "c", "h3", snowflake.Config{})
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
		_, err := factory.GetOrCreate(context.Background(), fmt.Sprintf("p%d", i), fmt.Sprintf("h%d", i), snowflake.Config{})
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

	_, err := factory.GetOrCreate(context.Background(), "a", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "b", "h2", snowflake.Config{})
	require.NoError(t, err)

	// Evict "a" explicitly.
	factory.Evict("a")
	assert.True(t, (*created)[0].closed.Load())

	// Now adding "c" should NOT evict "b" since we only have 1 client.
	_, err = factory.GetOrCreate(context.Background(), "c", "h3", snowflake.Config{})
	require.NoError(t, err)

	assert.False(t, (*created)[1].closed.Load(), "'b' should still be open")
	assert.False(t, (*created)[2].closed.Load(), "'c' should still be open")
}

// ---------------------------------------------------------------------------
// Tests: Idle TTL
// ---------------------------------------------------------------------------

func TestGetOrCreate_IdleTTL_EvictsExpiredClient(t *testing.T) {
	t.Parallel()

	factory, created := newFakeFactory()
	factory.WithIdleTTL(1) // 1 nanosecond — effectively always expired
	defer factory.Close()

	// First call — creates client.
	c1, err := factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
	require.NoError(t, err)
	require.NotNil(t, c1)

	// Second call with same hash — client should be evicted due to TTL.
	c2, err := factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
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

	_, err := factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
	require.NoError(t, err)

	_, err = factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
	require.NoError(t, err)

	// Should reuse the same client — only 1 created.
	assert.Len(t, *created, 1)
	assert.False(t, (*created)[0].closed.Load())
}

// ---------------------------------------------------------------------------
// Tests: Startup grace period
// ---------------------------------------------------------------------------

func TestCheckHealth_StartupGrace_PassesWhenNoClients(t *testing.T) {
	t.Parallel()

	factory, _ := newFakeFactory()
	factory.WithStartupGrace(10 * time.Minute) // long grace — should pass
	defer factory.Close()

	// No clients cached — within grace period, should pass.
	err := factory.CheckHealth(nil)
	assert.NoError(t, err)
}

func TestCheckHealth_StartupGrace_ChecksAfterGraceExpires(t *testing.T) {
	t.Parallel()

	factory, _ := newFakeFactory()
	factory.startupGrace = 1 * time.Nanosecond // effectively expired immediately
	factory.startTime = time.Now().Add(-time.Second)
	defer factory.Close()

	// No clients cached, grace expired — should still pass (zero providers is valid).
	err := factory.CheckHealth(nil)
	assert.NoError(t, err)
}

func TestCheckHealth_StartupGrace_FailsWithUnhealthyClient(t *testing.T) {
	t.Parallel()

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		return &unhealthyFakeClient{}, nil
	})
	factory.WithStartupGrace(10 * time.Minute) // within grace
	defer factory.Close()

	// Add an unhealthy client — grace period doesn't protect when clients exist.
	_, err := factory.GetOrCreate(context.Background(), "sick", "h1", snowflake.Config{})
	require.NoError(t, err)

	err = factory.CheckHealth(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connectivity check failed")
}

// ---------------------------------------------------------------------------
// Tests: Close error logging
// ---------------------------------------------------------------------------

type closeErrorClient struct {
	fakeClient
}

func (c *closeErrorClient) Close() error {
	c.closed.Store(true)
	return errors.New("connection already closed")
}

func TestCloseClient_LogsError(t *testing.T) {
	t.Parallel()

	// Use a custom logger that we can observe.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		return &closeErrorClient{}, nil
	})
	factory.WithLogger(logger)
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "bad-closer", "h1", snowflake.Config{})
	require.NoError(t, err)

	// Evict triggers Close which returns an error.
	factory.Evict("bad-closer")

	logged := buf.String()
	assert.Contains(t, logged, "error closing Snowflake client")
	assert.Contains(t, logged, "bad-closer")
	assert.Contains(t, logged, "connection already closed")
}

func TestCloseClient_SilentOnSuccess(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	factory, _ := newFakeFactory()
	factory.WithLogger(logger)
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "good", "h1", snowflake.Config{})
	require.NoError(t, err)

	factory.Evict("good")

	// No error — nothing should be logged.
	assert.Empty(t, buf.String())
}

// ---------------------------------------------------------------------------
// Tests: O(1) LRU with container/list
// ---------------------------------------------------------------------------

func TestGetOrCreate_LRU_LargeScale_O1(t *testing.T) {
	t.Parallel()

	// Create a factory with maxSize=100 and push 200 providers through it
	// to verify O(1) operations work correctly at scale.
	factory, created := newFakeFactory()
	factory.WithMaxSize(100)
	defer factory.Close()

	for i := range 200 {
		_, err := factory.GetOrCreate(context.Background(), fmt.Sprintf("p%d", i), fmt.Sprintf("h%d", i), snowflake.Config{})
		require.NoError(t, err)
	}

	// 200 created total. First 100 should have been evicted.
	assert.Len(t, *created, 200)

	for i := range 100 {
		assert.True(t, (*created)[i].closed.Load(), "provider p%d should have been evicted", i)
	}

	for i := 100; i < 200; i++ {
		assert.False(t, (*created)[i].closed.Load(), "provider p%d should still be open", i)
	}
}

func TestHasStaleHash(t *testing.T) {
	t.Parallel()

	factory, _ := newFakeFactory()
	defer factory.Close()

	_, err := factory.GetOrCreate(context.Background(), "p", "hash-v1", snowflake.Config{})
	require.NoError(t, err)

	assert.False(t, factory.HasStaleHash("p", "hash-v1"))
	assert.True(t, factory.HasStaleHash("p", "hash-v2"))
	assert.False(t, factory.HasStaleHash("nonexistent", "hash-v1"))
}

// ---------------------------------------------------------------------------
// Tests: Singleflight deduplication
// ---------------------------------------------------------------------------

func TestGetOrCreate_Singleflight_DeduplicatesConcurrent(t *testing.T) {
	t.Parallel()

	// Track how many times newFn is actually called.
	var createCount atomic.Int32

	// Use a channel to block newFn until all goroutines have started.
	gate := make(chan struct{})

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		<-gate // wait for signal
		createCount.Add(1)

		return &fakeClient{}, nil
	})
	defer factory.Close()

	const concurrency = 10

	var wg sync.WaitGroup

	results := make([]SnowflakeClient, concurrency)
	errs := make([]error, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = factory.GetOrCreate(context.Background(), "provider-x", "h1", snowflake.Config{})
		}(i)
	}

	// Give goroutines time to enter singleflight.
	time.Sleep(50 * time.Millisecond)

	// Release the gate — one goroutine creates, others wait.
	close(gate)
	wg.Wait()

	for i := range concurrency {
		assert.NoError(t, errs[i], "goroutine %d", i)
		assert.NotNil(t, results[i], "goroutine %d", i)
	}

	// Singleflight should deduplicate: exactly 1 creation call.
	assert.Equal(t, int32(1), createCount.Load(), "singleflight should deduplicate to 1 creation")
}

func TestGetOrCreate_Singleflight_DifferentProvidersConcurrent(t *testing.T) {
	t.Parallel()

	var createCount atomic.Int32

	gate := make(chan struct{})

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		<-gate
		createCount.Add(1)

		return &fakeClient{}, nil
	})
	defer factory.Close()

	var wg sync.WaitGroup

	// 3 different providers, 5 goroutines each.
	for _, prov := range []string{"a", "b", "c"} {
		for range 5 {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				_, _ = factory.GetOrCreate(context.Background(), p, "h1", snowflake.Config{})
			}(prov)
		}
	}

	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	// Each provider should have exactly 1 creation — 3 total.
	assert.Equal(t, int32(3), createCount.Load(),
		"each provider should have exactly 1 connection created")
}

func TestGetOrCreate_Singleflight_ErrorNotCached(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	factory := NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (SnowflakeClient, error) {
		n := callCount.Add(1)
		if n == 1 {
			return nil, errors.New("transient failure")
		}

		return &fakeClient{}, nil
	})
	defer factory.Close()

	// First call fails.
	_, err := factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
	assert.Error(t, err)

	// Second call should retry (singleflight doesn't cache errors).
	c, err := factory.GetOrCreate(context.Background(), "p", "h1", snowflake.Config{})
	assert.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestGetOrCreate_Singleflight_DoesNotBlockOtherProviders(t *testing.T) {
	t.Parallel()

	// Provider "slow" takes a long time; provider "fast" should not be blocked.
	slowGate := make(chan struct{})

	factory := NewTestClientFactoryWithFn(func(_ context.Context, cfg snowflake.Config) (SnowflakeClient, error) {
		if cfg.Account == "slow" {
			<-slowGate
		}

		return &fakeClient{}, nil
	})
	defer factory.Close()

	// Start slow provider in background.
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		_, _ = factory.GetOrCreate(context.Background(), "slow-provider", "h1", snowflake.Config{Account: "slow"})
	}()

	// Give slow provider time to start.
	time.Sleep(50 * time.Millisecond)

	// Fast provider should complete immediately, not blocked by slow one.
	done := make(chan struct{})
	go func() {
		c, err := factory.GetOrCreate(context.Background(), "fast-provider", "h1", snowflake.Config{Account: "fast"})
		assert.NoError(t, err)
		assert.NotNil(t, c)
		close(done)
	}()

	select {
	case <-done:
		// success — fast provider was not blocked
	case <-time.After(2 * time.Second):
		t.Fatal("fast provider was blocked by slow provider — lock not released during newFn")
	}

	// Clean up slow provider.
	close(slowGate)
	wg.Wait()
}
