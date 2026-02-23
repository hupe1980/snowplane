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
