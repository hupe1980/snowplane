// Package ratelimit provides per-provider token-bucket rate limiting for
// Snowflake API calls.
package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter provides per-provider token-bucket rate limiting. Each provider
// gets its own independent limiter, and the limits can be tuned via Options.
type Limiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	opts     Options
}

// Options configures the rate limiter.
type Options struct {
	// QPS is the maximum sustained queries per second per provider.
	// Zero or negative means no rate limiting.
	QPS float64

	// Burst is the maximum number of requests allowed in a burst.
	Burst int
}

// DefaultOptions returns sensible defaults:
// 10 QPS with a burst of 20 per provider.
func DefaultOptions() Options {
	return Options{
		QPS:   10,
		Burst: 20,
	}
}

// New creates a new per-provider rate limiter.
func New(opts Options) *Limiter {
	return &Limiter{
		limiters: make(map[string]*rate.Limiter),
		opts:     opts,
	}
}

// Wait blocks until the rate limiter allows the request for the given provider,
// or the context is cancelled. It returns true if it actually waited (was
// rate-limited), false if it passed through immediately.
func (l *Limiter) Wait(ctx context.Context, provider string) (bool, error) {
	if l.opts.QPS <= 0 {
		return false, nil
	}

	rl := l.getOrCreate(provider)

	// Check if we can proceed immediately.
	if rl.Allow() {
		return false, nil
	}

	// We need to wait: this counts as a rate-limit wait.
	if err := rl.Wait(ctx); err != nil {
		return true, err
	}

	return true, nil
}

// Evict removes the rate limiter for the given provider.
func (l *Limiter) Evict(provider string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.limiters, provider)
}

func (l *Limiter) getOrCreate(provider string) *rate.Limiter {
	l.mu.RLock()
	if rl, ok := l.limiters[provider]; ok {
		l.mu.RUnlock()
		return rl
	}

	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after write lock.
	if rl, ok := l.limiters[provider]; ok {
		return rl
	}

	rl := rate.NewLimiter(rate.Limit(l.opts.QPS), l.opts.Burst)
	l.limiters[provider] = rl

	return rl
}
