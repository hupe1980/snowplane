package sfretry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil", err: nil, expected: false},
		{name: "terminal error", err: snowflake.NewTerminalError(errors.New("fatal")), expected: false},
		{name: "insufficient privileges", err: fmt.Errorf("failed: %w", snowflake.ErrInsufficientPrivileges), expected: false},
		{name: "object already exists", err: fmt.Errorf("create: %w", snowflake.ErrObjectAlreadyExists), expected: false},
		{name: "role switch failed", err: fmt.Errorf("use role: %w", snowflake.ErrRoleSwitchFailed), expected: false},
		{name: "connection failed", err: fmt.Errorf("dial: %w", snowflake.ErrConnectionFailed), expected: true},
		{name: "statement timeout", err: fmt.Errorf("slow: %w", snowflake.ErrStatementTimeout), expected: true},
		{name: "account locked", err: fmt.Errorf("locked: %w", snowflake.ErrAccountLocked), expected: false},
		{name: "quota exceeded", err: fmt.Errorf("quota: %w", snowflake.ErrQuotaExceeded), expected: false},
		{name: "sql compilation", err: fmt.Errorf("compile: %w", snowflake.ErrSQLCompilation), expected: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, expected: true},
		{name: "context canceled", err: context.Canceled, expected: true},
		{name: "generic error", err: errors.New("something went wrong"), expected: true},
		{name: "object not found", err: fmt.Errorf("show: %w", snowflake.ErrObjectNotFound), expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, IsRetryable(tt.err))
		})
	}
}

func TestDo_SucceedsImmediately(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), DefaultOptions(), func() error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestDo_RetriesTransientError(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), Options{MaxAttempts: 3, Backoff: time.Millisecond}, func() error {
		calls++
		if calls < 3 {
			return fmt.Errorf("transient: %w", snowflake.ErrConnectionFailed)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestDo_StopsOnTerminalError(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), Options{MaxAttempts: 3, Backoff: time.Millisecond}, func() error {
		calls++
		return snowflake.NewTerminalError(errors.New("fatal"))
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
	assert.True(t, snowflake.IsTerminalError(err))
}

func TestDo_StopsOnNonRetryableError(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), Options{MaxAttempts: 3, Backoff: time.Millisecond}, func() error {
		calls++
		return fmt.Errorf("denied: %w", snowflake.ErrInsufficientPrivileges)
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestDo_ExhaustsAttempts(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), Options{MaxAttempts: 3, Backoff: time.Millisecond}, func() error {
		calls++
		return fmt.Errorf("dial: %w", snowflake.ErrConnectionFailed)
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls)
	assert.Contains(t, err.Error(), "operation failed after 3 attempts")
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := Do(ctx, Options{MaxAttempts: 5, Backoff: time.Second}, func() error {
		calls++
		return fmt.Errorf("timeout: %w", snowflake.ErrStatementTimeout)
	})

	require.Error(t, err)
	assert.LessOrEqual(t, calls, 2)
}

func TestDo_DefaultsMaxAttempts(t *testing.T) {
	t.Parallel()

	calls := 0
	err := Do(context.Background(), Options{MaxAttempts: 0, Backoff: time.Millisecond}, func() error {
		calls++
		return errors.New("fail")
	})

	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	opts := DefaultOptions()
	assert.Equal(t, 3, opts.MaxAttempts)
	assert.Equal(t, 2*time.Second, opts.Backoff)
}
