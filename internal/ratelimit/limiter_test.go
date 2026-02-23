package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ReturnsNonNil(t *testing.T) {
	t.Parallel()

	l := New(DefaultOptions())
	assert.NotNil(t, l)
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultOptions()
	assert.Equal(t, float64(10), opts.QPS)
	assert.Equal(t, 20, opts.Burst)
}

func TestWait_DisabledWhenQPSZero(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 0, Burst: 1})

	waited, err := l.Wait(context.Background(), "provider1")
	require.NoError(t, err)
	assert.False(t, waited)
}

func TestWait_DisabledWhenQPSNegative(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: -1, Burst: 1})

	waited, err := l.Wait(context.Background(), "provider1")
	require.NoError(t, err)
	assert.False(t, waited)
}

func TestWait_AllowsWithinBurst(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 5})

	// Should allow 5 requests without waiting.
	for i := range 5 {
		waited, err := l.Wait(context.Background(), "provider1")
		require.NoError(t, err, "request %d", i)
		assert.False(t, waited, "request %d should not have waited", i)
	}
}

func TestWait_RateLimitsAfterBurst(t *testing.T) {
	t.Parallel()

	// Very low rate: 1 QPS, burst 1. First request passes, second must wait.
	l := New(Options{QPS: 1, Burst: 1})

	waited, err := l.Wait(context.Background(), "provider1")
	require.NoError(t, err)
	assert.False(t, waited)

	// Second request should be rate-limited.
	start := time.Now()

	waited, err = l.Wait(context.Background(), "provider1")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, waited)
	assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond, "should have waited ~1s")
}

func TestWait_PerProviderIsolation(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2})

	// Exhaust provider1 burst.
	for range 2 {
		_, err := l.Wait(context.Background(), "provider1")
		require.NoError(t, err)
	}

	// Provider2 should still have full burst available.
	waited, err := l.Wait(context.Background(), "provider2")
	require.NoError(t, err)
	assert.False(t, waited)
}

func TestWait_ContextCanceled(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 1, Burst: 1})

	// Exhaust burst.
	_, err := l.Wait(context.Background(), "provider1")
	require.NoError(t, err)

	// Cancel context immediately so the next Wait fails.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = l.Wait(ctx, "provider1")
	assert.Error(t, err)
}

func TestEvict_RemovesProvider(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2})

	// Exhaust burst.
	for range 2 {
		_, err := l.Wait(context.Background(), "provider1")
		require.NoError(t, err)
	}

	// Evict resets the provider's bucket.
	l.Evict("provider1")

	// Should get a fresh burst allowance.
	waited, err := l.Wait(context.Background(), "provider1")
	require.NoError(t, err)
	assert.False(t, waited)
}

func TestEvict_NonExistentProviderNoPanic(t *testing.T) {
	t.Parallel()

	l := New(DefaultOptions())
	assert.NotPanics(t, func() { l.Evict("nonexistent") })
}
