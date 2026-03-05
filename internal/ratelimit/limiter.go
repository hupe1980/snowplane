// Package ratelimit provides hierarchical token-bucket rate limiting for
// Snowflake API calls. Each request is gated by two independent limiters:
//
//  1. Per-controller limiter (keyed by provider+controller) — ensures fairness
//     between controllers so a noisy reconciler cannot starve others.
//  2. Per-account limiter (keyed by provider) — caps aggregate QPS across all
//     controllers for a given Snowflake account, preventing HTTP 429s.
package ratelimit

import (
	"context"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter provides hierarchical token-bucket rate limiting. Each provider
// gets its own per-account limiter, and each provider+controller pair gets
// an independent per-controller limiter. Both limits are configurable.
type Limiter struct {
	mu              sync.RWMutex
	limiters        map[string]*rate.Limiter // keyed by "provider/controller"
	accountLimiters map[string]*rate.Limiter // keyed by "provider"
	opts            Options
}

// Options configures the rate limiter.
type Options struct {
	// QPS is the maximum sustained queries per second per controller per provider.
	// Zero or negative means no per-controller rate limiting.
	QPS float64

	// Burst is the maximum number of requests allowed in a burst per controller.
	Burst int

	// AccountQPS is the maximum aggregate queries per second per provider
	// (Snowflake account). This caps total QPS across all controllers using
	// the same provider. Zero or negative means no account-level rate limiting.
	AccountQPS float64

	// AccountBurst is the maximum burst size for the per-account rate limiter.
	AccountBurst int
}

// DefaultOptions returns sensible defaults:
// 10 QPS per controller with burst 20, 50 QPS aggregate per account with burst 100.
func DefaultOptions() Options {
	return Options{
		QPS:          10,
		Burst:        20,
		AccountQPS:   50,
		AccountBurst: 100,
	}
}

// New creates a new hierarchical rate limiter. Burst values are clamped to
// at least 1 when QPS > 0 to prevent creating limiters that reject all requests.
func New(opts Options) *Limiter {
	if opts.QPS > 0 && opts.Burst < 1 {
		opts.Burst = 1
	}

	if opts.AccountQPS > 0 && opts.AccountBurst < 1 {
		opts.AccountBurst = 1
	}

	return &Limiter{
		limiters:        make(map[string]*rate.Limiter),
		accountLimiters: make(map[string]*rate.Limiter),
		opts:            opts,
	}
}

// Wait blocks until both the per-controller and per-account rate limiters
// allow the request, or the context is cancelled. It returns two booleans
// indicating whether the caller was delayed by the controller or account
// limiter respectively.
func (l *Limiter) Wait(ctx context.Context, provider, controller string) (controllerWaited, accountWaited bool, err error) {
	// Per-controller limit.
	if l.opts.QPS > 0 {
		key := provider + "/" + controller
		rl := l.getOrCreate(l.limiters, key, l.opts.QPS, l.opts.Burst)

		if !rl.Allow() {
			if err := rl.Wait(ctx); err != nil {
				return false, false, err
			}

			controllerWaited = true
		}
	}

	// Per-account (aggregate) limit.
	if l.opts.AccountQPS > 0 {
		rl := l.getOrCreate(l.accountLimiters, provider, l.opts.AccountQPS, l.opts.AccountBurst)

		if !rl.Allow() {
			if err := rl.Wait(ctx); err != nil {
				return controllerWaited, false, err
			}

			accountWaited = true
		}
	}

	return controllerWaited, accountWaited, nil
}

// Evict removes rate limiters for the given provider. This removes all
// per-controller limiters with the "provider/" prefix and the per-account
// limiter for the provider.
func (l *Limiter) Evict(provider string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	prefix := provider + "/"

	for key := range l.limiters {
		if strings.HasPrefix(key, prefix) {
			delete(l.limiters, key)
		}
	}

	delete(l.accountLimiters, provider)
}

func (l *Limiter) getOrCreate(m map[string]*rate.Limiter, key string, qps float64, burst int) *rate.Limiter {
	l.mu.RLock()
	if rl, ok := m[key]; ok {
		l.mu.RUnlock()
		return rl
	}

	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after write lock.
	if rl, ok := m[key]; ok {
		return rl
	}

	rl := rate.NewLimiter(rate.Limit(qps), burst)
	m[key] = rl

	return rl
}
