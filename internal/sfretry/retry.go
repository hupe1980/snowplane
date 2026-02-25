// Package sfretry provides retry logic for transient Snowflake errors.
//
// Instead of relying solely on controller-runtime exponential backoff (which
// restarts the entire reconcile cycle), sfretry wraps individual Snowflake
// operations with a small retry loop (configurable attempts, constant backoff).
// This avoids redundant Observe calls after a transient network blip.
//
// # Idempotency Safety
//
// All three retried Snowflake operations are inherently safe to retry:
//
//   - CREATE: If the object already exists, Snowflake returns an
//     "already exists" error which is classified as non-retryable
//     (ErrObjectAlreadyExists). The retry loop stops immediately.
//
//   - ALTER: ALTER statements are idempotent — applying the same
//     desired state a second time produces no additional side effects.
//
//   - DROP: If the object was already dropped, the reconciler treats
//     "object not found" as success (not an error), so a double-drop
//     is harmless.
//
// Non-idempotent operations (e.g., GRANT ... ON ALL TABLES) are NOT
// wrapped in sfretry.Do. Grant operations use single-target GRANT/REVOKE
// statements, which are idempotent (granting an already-held privilege or
// revoking an already-revoked privilege is a no-op in Snowflake).
package sfretry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// Options controls retry behaviour.
type Options struct {
	// MaxAttempts is the total number of attempts (including the initial call).
	MaxAttempts int
	// Backoff is the constant delay between retries.
	Backoff time.Duration
}

// DefaultOptions returns production defaults: 3 attempts, 2 s backoff.
func DefaultOptions() Options {
	return Options{
		MaxAttempts: 3,
		Backoff:     2 * time.Second,
	}
}

// IsRetryable returns true when err represents a transient failure that
// warrants a retry.  Terminal errors, permission errors, already-exists,
// and role-switch failures are NOT retried.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Terminal errors are explicitly non-retryable.
	if snowflake.IsTerminalError(err) {
		return false
	}

	// Specific sentinel errors that should not be retried.
	switch {
	case errors.Is(err, snowflake.ErrInsufficientPrivileges):
		return false
	case errors.Is(err, snowflake.ErrObjectAlreadyExists):
		return false
	case errors.Is(err, snowflake.ErrRoleSwitchFailed):
		return false
	case errors.Is(err, snowflake.ErrAccountLocked):
		return false
	case errors.Is(err, snowflake.ErrQuotaExceeded):
		return false
	case errors.Is(err, snowflake.ErrSQLCompilation):
		return false
	}

	return true
}

// Do executes fn up to opts.MaxAttempts times, sleeping opts.Backoff between
// retries.  It stops early on non-retryable errors or context cancellation.
// Debug-level logs (V(1)) are emitted for every retry attempt so that
// operators can trace transient failures without enabling full debug mode.
func Do(ctx context.Context, opts Options, fn func() error) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 1
	}

	logger := log.FromContext(ctx)

	var lastErr error

	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !IsRetryable(lastErr) {
			return lastErr
		}

		if attempt < opts.MaxAttempts {
			// Add jitter: sleep for 50-100% of the configured backoff
			// to spread retries across concurrent reconciliations.
			jitteredBackoff := opts.Backoff/2 + time.Duration(rand.Int64N(int64(opts.Backoff/2+1))) //nolint:gosec // G404: math/rand is fine for jitter

			logger.V(1).Info("retrying transient Snowflake error",
				"attempt", attempt,
				"maxAttempts", opts.MaxAttempts,
				"backoff", jitteredBackoff.String(),
				"error", lastErr.Error(),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("retry aborted: %w", ctx.Err())
			case <-time.After(jitteredBackoff):
			}
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", opts.MaxAttempts, lastErr)
}
