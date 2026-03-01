package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNew_DefaultOptions(t *testing.T) {
	t.Parallel()

	cb := New(DefaultOptions())
	assert.NotNil(t, cb)
	assert.Equal(t, 5, cb.opts.FailureThreshold)
	assert.Equal(t, 60*time.Second, cb.opts.ResetTimeout)
}

func TestNew_InvalidOptions_UsesDefaults(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: -1, ResetTimeout: 0})
	assert.Equal(t, 5, cb.opts.FailureThreshold)
	assert.Equal(t, 60*time.Second, cb.opts.ResetTimeout)
}

func TestBreaker_Closed_AllowsCalls(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: 3, ResetTimeout: time.Minute})
	assert.NoError(t, cb.Allow("provider-a"))
	assert.Equal(t, StateClosed, cb.State("provider-a"))
}

func TestBreaker_OpensAfterThreshold(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: 3, ResetTimeout: time.Minute})
	provider := "provider-b"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)
	assert.Equal(t, StateClosed, cb.State(provider))

	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)
}

func TestBreaker_SuccessResets(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: 2, ResetTimeout: time.Minute})
	provider := "provider-c"

	cb.RecordFailure(provider)
	assert.Equal(t, 1, cb.ConsecutiveFailures(provider))

	cb.RecordSuccess(provider)
	assert.Equal(t, 0, cb.ConsecutiveFailures(provider))
	assert.Equal(t, StateClosed, cb.State(provider))
}

func TestBreaker_HalfOpenAfterTimeout(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{FailureThreshold: 2, ResetTimeout: 10 * time.Second})
	cb.now = func() time.Time { return now }

	provider := "provider-d"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	// Before timeout: still open.
	cb.now = func() time.Time { return now.Add(5 * time.Second) }
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	// After timeout: transitions to half-open.
	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	assert.Equal(t, StateHalfOpen, cb.State(provider))
	assert.NoError(t, cb.Allow(provider))
}

func TestBreaker_HalfOpenSuccess_ResetsToClosed(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{FailureThreshold: 2, ResetTimeout: 10 * time.Second})
	cb.now = func() time.Time { return now }

	provider := "provider-e"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)

	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb.Allow(provider))

	cb.RecordSuccess(provider)
	assert.Equal(t, StateClosed, cb.State(provider))
	assert.Equal(t, 0, cb.ConsecutiveFailures(provider))
}

func TestBreaker_HalfOpenFailure_ReOpens(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{FailureThreshold: 2, ResetTimeout: 10 * time.Second})
	cb.now = func() time.Time { return now }

	provider := "provider-f"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)

	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb.Allow(provider))

	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))
}

func TestBreaker_HalfOpen_SingleProbeOnly(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{FailureThreshold: 2, ResetTimeout: 10 * time.Second})
	cb.now = func() time.Time { return now }

	provider := "provider-g"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)

	// Advance past reset timeout → HalfOpen.
	cb.now = func() time.Time { return now.Add(11 * time.Second) }

	// First probe should be allowed.
	require.NoError(t, cb.Allow(provider))

	// Second concurrent probe must be rejected while first is in-flight.
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	// After success, next call is allowed (back to Closed).
	cb.RecordSuccess(provider)
	assert.NoError(t, cb.Allow(provider))
}

func TestBreaker_IndependentProviders(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: 2, ResetTimeout: time.Minute})

	cb.RecordFailure("provider-a")
	cb.RecordFailure("provider-a")
	assert.ErrorIs(t, cb.Allow("provider-a"), ErrCircuitOpen)

	assert.NoError(t, cb.Allow("provider-b"))
}

func TestBreaker_Reset(t *testing.T) {
	t.Parallel()

	cb := New(Options{FailureThreshold: 2, ResetTimeout: time.Minute})
	provider := "provider-g"

	cb.RecordFailure(provider)
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	cb.Reset(provider)
	assert.Equal(t, StateClosed, cb.State(provider))
	assert.Equal(t, 0, cb.ConsecutiveFailures(provider))
}

func TestBreaker_UnknownProvider_IsClosed(t *testing.T) {
	t.Parallel()

	cb := New(DefaultOptions())
	assert.Equal(t, StateClosed, cb.State("never-seen"))
	assert.Equal(t, 0, cb.ConsecutiveFailures("never-seen"))
}

func TestState_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "closed", StateClosed.String())
	assert.Equal(t, "open", StateOpen.String())
	assert.Equal(t, "half-open", StateHalfOpen.String())
	assert.Equal(t, "unknown", State(99).String())
}

func TestBreaker_ExponentialBackoff(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Second,
		MaxResetTimeout:  80 * time.Second,
	})
	cb.now = func() time.Time { return now }
	cb.jitterFrac = func() float64 { return 0 } // deterministic: no jitter

	provider := "exp-backoff"

	// Trip the breaker (threshold=1).
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	// After 10s (initial timeout), should transition to HalfOpen.
	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb.Allow(provider))

	// Half-open probe fails → re-opens with 2x timeout (20s).
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	// After 15s (< 20s), should still be open.
	cb.now = func() time.Time { return now.Add(11*time.Second + 15*time.Second) }
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	// After 21s (> 20s), should transition to HalfOpen.
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second) }
	require.NoError(t, cb.Allow(provider))

	// Half-open probe fails again → re-opens with 4x timeout (40s).
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	// After 35s (< 40s), should still be open.
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second + 35*time.Second) }
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	// After 41s (> 40s), should transition to HalfOpen.
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second + 41*time.Second) }
	require.NoError(t, cb.Allow(provider))

	// Success resets to Closed and resets backoff.
	cb.RecordSuccess(provider)
	assert.Equal(t, StateClosed, cb.State(provider))

	// Trip again — should use initial timeout (10s), not the doubled one.
	cb.now = func() time.Time { return now.Add(100 * time.Second) }
	cb.RecordFailure(provider)
	assert.Equal(t, StateOpen, cb.State(provider))

	// After 11s, should transition to HalfOpen (initial timeout restored).
	cb.now = func() time.Time { return now.Add(112 * time.Second) }
	assert.Equal(t, StateHalfOpen, cb.State(provider))
}

func TestBreaker_ExponentialBackoffCapsAtMax(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cb := New(Options{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Second,
		MaxResetTimeout:  30 * time.Second,
	})
	cb.now = func() time.Time { return now }
	cb.jitterFrac = func() float64 { return 0 } // deterministic: no jitter

	provider := "exp-cap"

	// Trip → 10s, fail → 20s, fail → 30s (capped), fail → still 30s.
	cb.RecordFailure(provider)

	// First half-open → fail (timeout doubles to 20s)
	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb.Allow(provider))
	cb.RecordFailure(provider)

	// Second half-open → fail (timeout doubles to 30s, hits cap)
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second) }
	require.NoError(t, cb.Allow(provider))
	cb.RecordFailure(provider)

	// Third half-open → fail (timeout stays at 30s, already at cap)
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second + 31*time.Second) }
	require.NoError(t, cb.Allow(provider))
	cb.RecordFailure(provider)

	// Should still need 30s (not 60s) — verify cap is enforced.
	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second + 31*time.Second + 25*time.Second) }
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	cb.now = func() time.Time { return now.Add(11*time.Second + 21*time.Second + 31*time.Second + 31*time.Second) }
	assert.Equal(t, StateHalfOpen, cb.State(provider))
}

func TestBreaker_JitterApplied(t *testing.T) {
	t.Parallel()

	now := time.Now()

	// Test with positive jitter (+20% of 2*10s = +4s → 24s).
	cb := New(Options{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Second,
		MaxResetTimeout:  120 * time.Second,
	})
	cb.now = func() time.Time { return now }
	cb.jitterFrac = func() float64 { return 1.0 } // max positive jitter

	provider := "jitter-positive"

	// Trip → initial 10s, then half-open fail → 2*10s + 20%*20s*1.0 = 20+4 = 24s.
	cb.RecordFailure(provider)
	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb.Allow(provider))
	cb.RecordFailure(provider) // now backoff = 24s

	// At 23s after second trip: still open.
	cb.now = func() time.Time { return now.Add(11*time.Second + 23*time.Second) }
	assert.ErrorIs(t, cb.Allow(provider), ErrCircuitOpen)

	// At 25s: should be half-open (24s timeout elapsed).
	cb.now = func() time.Time { return now.Add(11*time.Second + 25*time.Second) }
	require.NoError(t, cb.Allow(provider))

	// Test with negative jitter (-20% of 2*10s = -4s → 16s).
	cb2 := New(Options{
		FailureThreshold: 1,
		ResetTimeout:     10 * time.Second,
		MaxResetTimeout:  120 * time.Second,
	})
	cb2.now = func() time.Time { return now }
	cb2.jitterFrac = func() float64 { return -1.0 } // max negative jitter

	provider2 := "jitter-negative"

	// Trip → initial 10s, then half-open fail → 2*10s + 20%*20s*(-1.0) = 20-4 = 16s.
	cb2.RecordFailure(provider2)
	cb2.now = func() time.Time { return now.Add(11 * time.Second) }
	require.NoError(t, cb2.Allow(provider2))
	cb2.RecordFailure(provider2) // now backoff = 16s

	// At 15s after second trip: still open.
	cb2.now = func() time.Time { return now.Add(11*time.Second + 15*time.Second) }
	assert.ErrorIs(t, cb2.Allow(provider2), ErrCircuitOpen)

	// At 17s: should be half-open (16s timeout elapsed).
	cb2.now = func() time.Time { return now.Add(11*time.Second + 17*time.Second) }
	require.NoError(t, cb2.Allow(provider2))
}
