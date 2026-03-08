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
		{"ErrAccountLocked", ErrAccountLocked},
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
		{"SQLCompilation", ErrCodeSQLCompilation, ErrSQLCompilation},
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

// ---------------------------------------------------------------------------
// Tests: IsCreateOrAlterUnsupported
// ---------------------------------------------------------------------------

func TestIsCreateOrAlterUnsupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"generic error", errors.New("connection reset"), false},
		{"snowflake error code 2032", &gosnowflake.SnowflakeError{Number: ErrCodeCreateOrAlterUnsupported, Message: "unsupported"}, true},
		{"string match unsupported", errors.New("SQL compilation error: UNSUPPORTED feature"), true},
		{"string match unexpected OR", errors.New("SQL compilation error: unexpected 'OR'"), true},
		{"syntax error no longer matches", errors.New("SQL compilation error: syntax error"), false},
		{"string match 002032 no longer matches", errors.New("002032 (42601): SQL compilation error"), false},
		{"wrapped snowflake error 2032", fmt.Errorf("exec failed: %w", &gosnowflake.SnowflakeError{Number: ErrCodeCreateOrAlterUnsupported}), true},
		{"different snowflake error code", &gosnowflake.SnowflakeError{Number: 9999, Message: "other"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, IsCreateOrAlterUnsupported(tt.err))
		})
	}
}

func TestExtractErrorCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantOK   bool
	}{
		{"nil error", nil, 0, false},
		{"plain error", errors.New("something broke"), 0, false},
		{"snowflake error", &gosnowflake.SnowflakeError{Number: 2002}, 2002, true},
		{"wrapped snowflake error", fmt.Errorf("exec: %w", &gosnowflake.SnowflakeError{Number: 3001}), 3001, true},
		{"double wrapped", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &gosnowflake.SnowflakeError{Number: 1003})), 1003, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, ok := ExtractErrorCode(tt.err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}
