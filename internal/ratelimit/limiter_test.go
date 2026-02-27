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
	assert.Equal(t, float64(50), opts.AccountQPS)
	assert.Equal(t, 100, opts.AccountBurst)
}

func TestWait_DisabledWhenQPSZero(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 0, Burst: 1, AccountQPS: 0})

	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
	assert.False(t, accountWaited)
}

func TestWait_DisabledWhenQPSNegative(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: -1, Burst: 1, AccountQPS: -1})

	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
	assert.False(t, accountWaited)
}

func TestWait_AllowsWithinBurst(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 5, AccountQPS: 100, AccountBurst: 10})

	// Should allow 5 requests without waiting (per-controller burst).
	for i := range 5 {
		controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err, "request %d", i)
		assert.False(t, controllerWaited, "request %d should not have controller-waited", i)
		assert.False(t, accountWaited, "request %d should not have account-waited", i)
	}
}

func TestWait_ControllerRateLimitsAfterBurst(t *testing.T) {
	t.Parallel()

	// Very low per-controller rate: 1 QPS, burst 1.
	// High account rate so only the controller limit triggers.
	l := New(Options{QPS: 1, Burst: 1, AccountQPS: 1000, AccountBurst: 1000})

	controllerWaited, _, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)

	// Second request should be controller-rate-limited.
	start := time.Now()

	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, controllerWaited)
	assert.False(t, accountWaited)
	assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond, "should have waited ~1s")
}

func TestWait_AccountRateLimitsAfterBurst(t *testing.T) {
	t.Parallel()

	// Very high per-controller rate, very low per-account rate.
	l := New(Options{QPS: 1000, Burst: 1000, AccountQPS: 1, AccountBurst: 1})

	_, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)
	assert.False(t, accountWaited)

	// Second request (different controller, same provider) should be account-rate-limited.
	start := time.Now()

	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "schema")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, controllerWaited) // Per-controller is fine (different controller).
	assert.True(t, accountWaited)
	assert.GreaterOrEqual(t, elapsed, 500*time.Millisecond, "should have waited ~1s")
}

func TestWait_AccountLimitAggregatesAcrossControllers(t *testing.T) {
	t.Parallel()

	// Per-controller: generous (100 QPS burst 10 each).
	// Per-account: tight (1 QPS, burst 3) — low QPS so tokens don't refill within timeout.
	l := New(Options{QPS: 100, Burst: 10, AccountQPS: 1, AccountBurst: 3})

	// 3 requests from different controllers should pass (fills account burst).
	for _, ctrl := range []string{"database", "schema", "warehouse"} {
		_, accountWaited, err := l.Wait(context.Background(), "provider1", ctrl)
		require.NoError(t, err, ctrl)
		assert.False(t, accountWaited, "%s should not have account-waited", ctrl)
	}

	// 4th request (another controller) should block on account limiter.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := l.Wait(ctx, "provider1", "table")
	assert.Error(t, err, "should fail: account burst exhausted, timeout too short")
}

func TestWait_PerProviderIsolation(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2, AccountQPS: 100, AccountBurst: 2})

	// Exhaust provider1 burst.
	for range 2 {
		_, _, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err)
	}

	// Provider2 should still have full burst available.
	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider2", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
	assert.False(t, accountWaited)
}

func TestWait_PerControllerIsolation(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2, AccountQPS: 0})

	// Exhaust database controller burst on provider1.
	for range 2 {
		_, _, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err)
	}

	// Schema controller on same provider should still have full burst.
	controllerWaited, _, err := l.Wait(context.Background(), "provider1", "schema")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
}

func TestWait_ContextCanceled_ControllerLimiter(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 1, Burst: 1, AccountQPS: 0})

	// Exhaust burst.
	_, _, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)

	// Cancel context immediately so the next Wait fails.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = l.Wait(ctx, "provider1", "database")
	assert.Error(t, err)
}

func TestWait_ContextCanceled_AccountLimiter(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 0, AccountQPS: 1, AccountBurst: 1})

	// Exhaust account burst.
	_, _, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)

	// Cancel context immediately so the next Wait fails.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err = l.Wait(ctx, "provider1", "schema")
	assert.Error(t, err)
}

func TestWait_OnlyControllerLimitEnabled(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 5, AccountQPS: 0})

	for i := range 5 {
		controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err, "request %d", i)
		assert.False(t, controllerWaited)
		assert.False(t, accountWaited, "account limiter disabled, should never report waited")
	}
}

func TestWait_OnlyAccountLimitEnabled(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 0, AccountQPS: 100, AccountBurst: 5})

	for i := range 5 {
		controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err, "request %d", i)
		assert.False(t, controllerWaited, "controller limiter disabled, should never report waited")
		assert.False(t, accountWaited)
	}
}

func TestEvict_RemovesAllControllerEntriesForProvider(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2, AccountQPS: 100, AccountBurst: 2})

	// Create entries for multiple controllers under provider1.
	for _, ctrl := range []string{"database", "schema", "warehouse"} {
		_, _, err := l.Wait(context.Background(), "provider1", ctrl)
		require.NoError(t, err)
	}

	// Verify entries exist.
	l.mu.RLock()
	assert.Len(t, l.limiters, 3)
	assert.Len(t, l.accountLimiters, 1)
	l.mu.RUnlock()

	// Evict provider1.
	l.Evict("provider1")

	// All per-controller and per-account entries should be gone.
	l.mu.RLock()
	assert.Empty(t, l.limiters)
	assert.Empty(t, l.accountLimiters)
	l.mu.RUnlock()
}

func TestEvict_DoesNotAffectOtherProviders(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2, AccountQPS: 100, AccountBurst: 2})

	// Create entries for two providers.
	_, _, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)

	_, _, err = l.Wait(context.Background(), "provider2", "database")
	require.NoError(t, err)

	// Evict provider1 only.
	l.Evict("provider1")

	// Provider2 entries should remain.
	l.mu.RLock()
	assert.Len(t, l.limiters, 1)        // provider2/database
	assert.Len(t, l.accountLimiters, 1) // provider2
	l.mu.RUnlock()

	// Provider2 should still work with its existing limiter.
	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider2", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
	assert.False(t, accountWaited)
}

func TestEvict_ResetsProviderBudget(t *testing.T) {
	t.Parallel()

	l := New(Options{QPS: 100, Burst: 2, AccountQPS: 100, AccountBurst: 2})

	// Exhaust burst.
	for range 2 {
		_, _, err := l.Wait(context.Background(), "provider1", "database")
		require.NoError(t, err)
	}

	// Evict resets the provider's budgets.
	l.Evict("provider1")

	// Should get a fresh burst allowance.
	controllerWaited, accountWaited, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)
	assert.False(t, controllerWaited)
	assert.False(t, accountWaited)
}

func TestEvict_NonExistentProviderNoPanic(t *testing.T) {
	t.Parallel()

	l := New(DefaultOptions())
	assert.NotPanics(t, func() { l.Evict("nonexistent") })
}

func TestEvict_ProviderNamePrefix_DoesNotOverEvict(t *testing.T) {
	t.Parallel()

	// Ensure "provider1" eviction does not remove "provider10" entries.
	l := New(Options{QPS: 100, Burst: 5, AccountQPS: 100, AccountBurst: 5})

	_, _, err := l.Wait(context.Background(), "provider1", "database")
	require.NoError(t, err)

	_, _, err = l.Wait(context.Background(), "provider10", "database")
	require.NoError(t, err)

	l.Evict("provider1")

	// provider10 should still have its entries.
	l.mu.RLock()
	assert.Len(t, l.limiters, 1)        // provider10/database
	assert.Len(t, l.accountLimiters, 1) // provider10
	l.mu.RUnlock()
}

func TestWait_BothLimitsActive_AccountIsBottleneck(t *testing.T) {
	t.Parallel()

	// Per-controller: 10 QPS burst 10.
	// Per-account: 1 QPS burst 2 — low QPS so tokens don't refill within timeout.
	// Sending from 3 different controllers: per-controller allows 10 each,
	// but per-account only allows 2 total.
	l := New(Options{QPS: 10, Burst: 10, AccountQPS: 1, AccountBurst: 2})

	// First 2 pass through both.
	for i, ctrl := range []string{"db", "schema"} {
		cw, aw, err := l.Wait(context.Background(), "prov", ctrl)
		require.NoError(t, err, "request %d", i)
		assert.False(t, cw)
		assert.False(t, aw)
	}

	// 3rd from a new controller: per-controller is fine, but account-burst exhausted.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := l.Wait(ctx, "prov", "warehouse")
	assert.Error(t, err, "should fail: account burst exhausted")
}

func TestWait_BothLimitsActive_ControllerIsBottleneck(t *testing.T) {
	t.Parallel()

	// Per-controller: 1 QPS burst 1 — low QPS so tokens don't refill within timeout.
	// Per-account: 100 QPS burst 100.
	// Same controller twice: per-controller burst exhausted.
	l := New(Options{QPS: 1, Burst: 1, AccountQPS: 100, AccountBurst: 100})

	_, _, err := l.Wait(context.Background(), "prov", "database")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err = l.Wait(ctx, "prov", "database")
	assert.Error(t, err, "should fail: per-controller burst exhausted")
}
