// Package circuitbreaker provides a per-provider circuit breaker that
// short-circuits Snowflake API calls after consecutive failures, allowing
// healthy providers to proceed unimpeded.
//
// States:
//
//   - Closed:   normal operation, calls pass through.
//   - Open:     after N consecutive failures, calls are rejected immediately.
//   - HalfOpen: after a backoff period, a single probe call is allowed.
//     Success resets to Closed; failure reopens.
package circuitbreaker

import (
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/snowplane/internal/metrics"
)

// State represents the current circuit breaker state.
type State int

const (
	// StateClosed is the normal state: calls pass through.
	StateClosed State = iota
	// StateOpen means the breaker has tripped: calls are rejected.
	StateOpen
	// StateHalfOpen allows a single probe call after the backoff period.
	StateHalfOpen
)

// String returns a human-readable state name.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when a call is rejected because the breaker is open.
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")

// Options configures the circuit breaker behaviour.
type Options struct {
	// FailureThreshold is the number of consecutive failures before the
	// circuit opens. Default: 5.
	FailureThreshold int

	// ResetTimeout is how long the circuit stays open before transitioning
	// to half-open. Default: 60s.
	ResetTimeout time.Duration
}

// DefaultOptions returns sensible production defaults.
func DefaultOptions() Options {
	return Options{
		FailureThreshold: 5,
		ResetTimeout:     60 * time.Second,
	}
}

// providerBreaker tracks circuit breaker state for a single provider.
type providerBreaker struct {
	state               State
	consecutiveFailures int
	lastFailureTime     time.Time
	probing             bool // true when a HalfOpen probe is in-flight
}

// Breaker provides per-provider circuit breaker state management.
// Each provider (ProviderConfig name) gets its own independent breaker.
type Breaker struct {
	mu       sync.RWMutex
	breakers map[string]*providerBreaker
	opts     Options
	now      func() time.Time // injectable clock for testing
}

// New creates a new per-provider circuit breaker.
func New(opts Options) *Breaker {
	if opts.FailureThreshold <= 0 {
		opts.FailureThreshold = 5
	}

	if opts.ResetTimeout <= 0 {
		opts.ResetTimeout = 60 * time.Second
	}

	return &Breaker{
		breakers: make(map[string]*providerBreaker),
		opts:     opts,
		now:      time.Now,
	}
}

// Allow checks whether a call to the given provider should proceed.
// Returns nil if the call is allowed, ErrCircuitOpen if the breaker is open.
func (b *Breaker) Allow(provider string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	pb := b.getOrCreate(provider)

	switch pb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if b.now().Sub(pb.lastFailureTime) >= b.opts.ResetTimeout {
			pb.state = StateHalfOpen
			pb.probing = true
			metrics.SetCircuitBreakerState(provider, float64(StateHalfOpen))

			return nil
		}

		return ErrCircuitOpen
	case StateHalfOpen:
		// Only one probe allowed at a time in HalfOpen.
		if pb.probing {
			return ErrCircuitOpen
		}

		pb.probing = true

		return nil
	}

	return nil
}

// RecordSuccess records a successful call for the provider.
// Resets the breaker to Closed state.
func (b *Breaker) RecordSuccess(provider string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pb := b.getOrCreate(provider)
	pb.consecutiveFailures = 0
	pb.state = StateClosed
	pb.probing = false
	metrics.SetCircuitBreakerState(provider, float64(StateClosed))
}

// RecordFailure records a failed call for the provider.
// Opens the breaker after reaching the failure threshold.
func (b *Breaker) RecordFailure(provider string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pb := b.getOrCreate(provider)
	pb.consecutiveFailures++
	pb.lastFailureTime = b.now()
	pb.probing = false

	if pb.consecutiveFailures >= b.opts.FailureThreshold {
		pb.state = StateOpen
		metrics.RecordCircuitBreakerTrip(provider)
		metrics.SetCircuitBreakerState(provider, float64(StateOpen))
	}
}

// State returns the current circuit breaker state for a provider.
func (b *Breaker) State(provider string) State {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if pb, ok := b.breakers[provider]; ok {
		if pb.state == StateOpen && b.now().Sub(pb.lastFailureTime) >= b.opts.ResetTimeout {
			return StateHalfOpen
		}

		return pb.state
	}

	return StateClosed
}

// ConsecutiveFailures returns the current failure count for a provider.
func (b *Breaker) ConsecutiveFailures(provider string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if pb, ok := b.breakers[provider]; ok {
		return pb.consecutiveFailures
	}

	return 0
}

// Reset clears all state for the given provider.
func (b *Breaker) Reset(provider string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.breakers, provider)
}

func (b *Breaker) getOrCreate(provider string) *providerBreaker {
	if pb, ok := b.breakers[provider]; ok {
		return pb
	}

	pb := &providerBreaker{state: StateClosed}
	b.breakers[provider] = pb

	return pb
}
