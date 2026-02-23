package snowflake

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gosnowflake "github.com/snowflakedb/gosnowflake"
)

func TestTerminalError_Error(t *testing.T) {
	t.Parallel()

	cause := errors.New("insufficient privileges")
	terr := NewTerminalError(cause)

	assert.Equal(t, "terminal: insufficient privileges", terr.Error())
}

func TestTerminalError_Unwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("access denied")
	terr := NewTerminalError(cause)

	assert.True(t, errors.Is(terr, cause))
}

func TestIsTerminalError_True(t *testing.T) {
	t.Parallel()

	terr := NewTerminalError(errors.New("boom"))
	assert.True(t, IsTerminalError(terr))
}

func TestIsTerminalError_Wrapped(t *testing.T) {
	t.Parallel()

	inner := NewTerminalError(errors.New("root cause"))
	wrapped := fmt.Errorf("context: %w", inner)

	assert.True(t, IsTerminalError(wrapped))
}

func TestIsTerminalError_False(t *testing.T) {
	t.Parallel()

	assert.False(t, IsTerminalError(errors.New("regular error")))
	assert.False(t, IsTerminalError(nil))
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"ErrObjectNotFound", ErrObjectNotFound},
		{"ErrObjectNotExistOrNotAuthorized", ErrObjectNotExistOrNotAuthorized},
		{"ErrInvalidIdentifier", ErrInvalidIdentifier},
		{"ErrObjectAlreadyExists", ErrObjectAlreadyExists},
		{"ErrInsufficientPrivileges", ErrInsufficientPrivileges},
		{"ErrReferenceNotReady", ErrReferenceNotReady},
		{"ErrConnectionFailed", ErrConnectionFailed},
		{"ErrObjectInUse", ErrObjectInUse},
		{"ErrAccountLocked", ErrAccountLocked},
		{"ErrInvalidValue", ErrInvalidValue},
		{"ErrRoleSwitchFailed", ErrRoleSwitchFailed},
		{"ErrQuotaExceeded", ErrQuotaExceeded},
		{"ErrStatementTimeout", ErrStatementTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, tt.err)
			require.NotEmpty(t, tt.err.Error())
		})
	}
}

func TestIsObjectNotFound(t *testing.T) {
	t.Parallel()

	assert.True(t, IsObjectNotFound(ErrObjectNotFound))
	assert.True(t, IsObjectNotFound(fmt.Errorf("wrap: %w", ErrObjectNotFound)))
	assert.False(t, IsObjectNotFound(errors.New("other")))
	assert.False(t, IsObjectNotFound(nil))
}

func TestIsObjectAlreadyExists(t *testing.T) {
	t.Parallel()

	assert.True(t, IsObjectAlreadyExists(ErrObjectAlreadyExists))
	assert.True(t, IsObjectAlreadyExists(fmt.Errorf("wrap: %w", ErrObjectAlreadyExists)))
	assert.False(t, IsObjectAlreadyExists(errors.New("other")))
}

func TestIsInsufficientPrivileges(t *testing.T) {
	t.Parallel()

	assert.True(t, IsInsufficientPrivileges(ErrInsufficientPrivileges))
	assert.False(t, IsInsufficientPrivileges(errors.New("other")))
}

func TestIsObjectInUse(t *testing.T) {
	t.Parallel()

	assert.True(t, IsObjectInUse(ErrObjectInUse))
	assert.True(t, IsObjectInUse(fmt.Errorf("wrap: %w", ErrObjectInUse)))
	assert.False(t, IsObjectInUse(errors.New("other")))
	assert.False(t, IsObjectInUse(nil))
}

func TestIsRoleSwitchFailed(t *testing.T) {
	t.Parallel()

	assert.True(t, IsRoleSwitchFailed(ErrRoleSwitchFailed))
	assert.True(t, IsRoleSwitchFailed(fmt.Errorf("wrap: %w", ErrRoleSwitchFailed)))
	assert.False(t, IsRoleSwitchFailed(errors.New("other")))
	assert.False(t, IsRoleSwitchFailed(nil))
}

func TestIsConnectionFailed(t *testing.T) {
	t.Parallel()

	assert.True(t, IsConnectionFailed(ErrConnectionFailed))
	assert.True(t, IsConnectionFailed(fmt.Errorf("wrap: %w", ErrConnectionFailed)))
	assert.False(t, IsConnectionFailed(errors.New("other")))
	assert.False(t, IsConnectionFailed(nil))
}

func TestMapSnowflakeError_Nil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, MapSnowflakeError(nil))
}

func TestMapSnowflakeError_NonSnowflakeError(t *testing.T) {
	t.Parallel()

	err := errors.New("generic database error")
	mapped := MapSnowflakeError(err)
	assert.Equal(t, err, mapped) // unchanged
}

func TestMapSnowflakeError_UnknownCode(t *testing.T) {
	t.Parallel()

	sfErr := &gosnowflake.SnowflakeError{Number: 99999, Message: "unknown thing"}
	mapped := MapSnowflakeError(sfErr)
	assert.Equal(t, sfErr, mapped) // unchanged
}

func TestMapSnowflakeError_KnownCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		sentinel error
	}{
		{"ObjectAlreadyExists", ErrCodeObjectAlreadyExists, ErrObjectAlreadyExists},
		{"InsufficientPrivileges", ErrCodeInsufficientPrivileges, ErrInsufficientPrivileges},
		{"ObjectNotExist", ErrCodeObjectNotExist, ErrObjectNotExistOrNotAuthorized},
		{"StatementTimeout", ErrCodeStatementTimeout, ErrStatementTimeout},
		{"AccountLocked", ErrCodeAccountLocked, ErrAccountLocked},
		{"QuotaExceeded", ErrCodeQuotaExceeded, ErrQuotaExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sfErr := &gosnowflake.SnowflakeError{
				Number:  tt.code,
				Message: "test error message",
			}
			mapped := MapSnowflakeError(sfErr)
			require.Error(t, mapped)

			// The mapped error should match the sentinel.
			assert.True(t, errors.Is(mapped, tt.sentinel),
				"expected errors.Is(%v, %v) to be true", mapped, tt.sentinel)

			// The original SnowflakeError should still be in the chain.
			assert.True(t, errors.Is(mapped, sfErr),
				"expected original SnowflakeError to remain in error chain")
		})
	}
}

func TestMapSnowflakeError_WrappedSnowflakeError(t *testing.T) {
	t.Parallel()

	sfErr := &gosnowflake.SnowflakeError{
		Number:  ErrCodeInsufficientPrivileges,
		Message: "no access",
	}
	wrapped := fmt.Errorf("exec failed: %w", sfErr)
	mapped := MapSnowflakeError(wrapped)

	assert.True(t, errors.Is(mapped, ErrInsufficientPrivileges))
}

func TestMapSnowflakeError_IsCheckers(t *testing.T) {
	t.Parallel()

	// Verify the Is* helper functions work with mapped errors.
	t.Run("IsObjectAlreadyExists", func(t *testing.T) {
		t.Parallel()
		sfErr := &gosnowflake.SnowflakeError{Number: ErrCodeObjectAlreadyExists, Message: "already exists"}
		assert.True(t, IsObjectAlreadyExists(MapSnowflakeError(sfErr)))
	})

	t.Run("IsInsufficientPrivileges", func(t *testing.T) {
		t.Parallel()
		sfErr := &gosnowflake.SnowflakeError{Number: ErrCodeInsufficientPrivileges, Message: "no access"}
		assert.True(t, IsInsufficientPrivileges(MapSnowflakeError(sfErr)))
	})
}
