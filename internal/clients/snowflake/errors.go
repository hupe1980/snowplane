package snowflake

import (
	"errors"
	"fmt"
	"strings"

	gosnowflake "github.com/snowflakedb/gosnowflake"
)

// Snowflake SQL error codes mapped to sentinel errors.
// See: https://docs.snowflake.com/en/developer-guide/snowflake-scripting/exceptions#list-of-error-codes
const (
	// Object already exists (CREATE without OR REPLACE/IF NOT EXISTS).
	ErrCodeObjectAlreadyExists = 2002

	// Insufficient privileges for the operation.
	ErrCodeInsufficientPrivileges = 3001

	// Object does not exist (DROP/ALTER/GRANT on missing object).
	ErrCodeObjectNotExist = 2003

	// SQL compilation error — often invalid identifier or syntax.
	ErrCodeSQLCompilation = 1003

	// Statement timed out.
	ErrCodeStatementTimeout = 604

	// CREATE OR ALTER is not supported on the account.
	ErrCodeCreateOrAlterUnsupported = 2032

	// Account is locked/suspended.
	ErrCodeAccountLocked = 260009

	// Quota exceeded.
	ErrCodeQuotaExceeded = 90101
)

// errorCodeMap maps gosnowflake error codes to sentinel errors.
var errorCodeMap = map[int]error{
	ErrCodeObjectAlreadyExists:    ErrObjectAlreadyExists,
	ErrCodeInsufficientPrivileges: ErrInsufficientPrivileges,
	ErrCodeObjectNotExist:         ErrObjectNotExistOrNotAuthorized,
	ErrCodeSQLCompilation:         ErrSQLCompilation,
	ErrCodeStatementTimeout:       ErrStatementTimeout,
	ErrCodeAccountLocked:          ErrAccountLocked,
	ErrCodeQuotaExceeded:          ErrQuotaExceeded,
}

// Sentinel errors for common Snowflake failure conditions.
var (
	// ErrObjectNotFound indicates that a SHOW command returned no rows.
	ErrObjectNotFound = errors.New("snowflake: object not found")

	// ErrObjectNotExistOrNotAuthorized maps to SQL state 02000.
	ErrObjectNotExistOrNotAuthorized = errors.New("snowflake: object does not exist or not authorized")

	// ErrObjectAlreadyExists maps to SQL error code 2002.
	ErrObjectAlreadyExists = errors.New("snowflake: object already exists")

	// ErrInsufficientPrivileges maps to SQL error code 3001.
	ErrInsufficientPrivileges = errors.New("snowflake: insufficient privileges")

	// ErrInvalidIdentifier indicates a malformed or empty identifier.
	ErrInvalidIdentifier = errors.New("snowflake: invalid identifier")

	// ErrReferenceNotReady indicates a referenced resource is not yet ready.
	ErrReferenceNotReady = errors.New("snowflake: referenced resource is not ready")

	// ErrConnectionFailed indicates the Snowflake connection could not be established.
	ErrConnectionFailed = errors.New("snowflake: connection failed")

	// ErrAccountLocked indicates the Snowflake account is locked.
	ErrAccountLocked = errors.New("snowflake: account locked")

	// ErrRoleSwitchFailed indicates USE ROLE failed (role does not exist or not granted).
	ErrRoleSwitchFailed = errors.New("snowflake: role switch failed")

	// ErrQuotaExceeded indicates a resource quota has been exceeded.
	ErrQuotaExceeded = errors.New("snowflake: quota exceeded")

	// ErrSQLCompilation indicates a SQL compilation error (invalid identifier, syntax, etc.).
	// These are permanent errors that should not be retried.
	ErrSQLCompilation = errors.New("snowflake: SQL compilation error")

	// ErrStatementTimeout indicates a SQL statement exceeded its time limit.
	ErrStatementTimeout = errors.New("snowflake: statement timeout")
)

// TerminalError wraps an error that should not be retried.
// Reconcilers inspect this type to decide whether to requeue.
type TerminalError struct {
	Err error
}

// NewTerminalError returns a new TerminalError wrapping cause.
func NewTerminalError(cause error) *TerminalError {
	return &TerminalError{Err: cause}
}

func (e *TerminalError) Error() string {
	return fmt.Sprintf("terminal: %s", e.Err.Error())
}

func (e *TerminalError) Unwrap() error {
	return e.Err
}

// IsTerminalError reports whether err (or any error in its chain) is a TerminalError.
func IsTerminalError(err error) bool {
	if err == nil {
		return false
	}

	var terr *TerminalError

	return errors.As(err, &terr)
}

// IsObjectNotFound reports whether err (or any error in its chain) is ErrObjectNotFound.
func IsObjectNotFound(err error) bool {
	return err != nil && errors.Is(err, ErrObjectNotFound)
}

// IsObjectNotExistOrNotAuthorized reports whether err (or any error in its chain)
// is ErrObjectNotExistOrNotAuthorized (SQL state 02000, code 2003).
// This is commonly returned when a parent object (database/schema) has already
// been dropped, making child objects implicitly gone.
func IsObjectNotExistOrNotAuthorized(err error) bool {
	return err != nil && errors.Is(err, ErrObjectNotExistOrNotAuthorized)
}

// IsRoleSwitchFailed reports whether err (or any error in its chain) is ErrRoleSwitchFailed.
func IsRoleSwitchFailed(err error) bool {
	return err != nil && errors.Is(err, ErrRoleSwitchFailed)
}

// IsConnectionFailed reports whether err (or any error in its chain) is ErrConnectionFailed.
func IsConnectionFailed(err error) bool {
	return err != nil && errors.Is(err, ErrConnectionFailed)
}

// IsCreateOrAlterUnsupported checks whether err indicates that the
// CREATE OR ALTER syntax is not supported by the Snowflake account.
// It first checks for a structured gosnowflake.SnowflakeError with code 2032,
// then falls back to string matching for non-structured errors.
func IsCreateOrAlterUnsupported(err error) bool {
	if err == nil {
		return false
	}

	// Prefer structured error code matching.
	var sfErr *gosnowflake.SnowflakeError
	if errors.As(err, &sfErr) && sfErr.Number == ErrCodeCreateOrAlterUnsupported {
		return true
	}

	// Fallback: string matching for non-structured errors (e.g. wrapped or
	// non-driver errors that don't produce gosnowflake.SnowflakeError).
	msg := strings.ToUpper(err.Error())

	return strings.Contains(msg, "UNSUPPORTED") ||
		strings.Contains(msg, "UNEXPECTED 'OR'") ||
		strings.Contains(msg, "SYNTAX ERROR")
}

// MapSnowflakeError wraps err with the matching sentinel error from errorCodeMap.
// If the error code is not recognized, the original error is returned unchanged.
// This ensures that IsObjectAlreadyExists, IsInsufficientPrivileges, and other
// sentinel checkers work correctly for errors returned by the Snowflake driver.
func MapSnowflakeError(err error) error {
	if err == nil {
		return nil
	}

	var sfErr *gosnowflake.SnowflakeError
	if !errors.As(err, &sfErr) {
		return err
	}

	sentinel, ok := errorCodeMap[sfErr.Number]
	if !ok {
		return err
	}

	return fmt.Errorf("%w: %w", sentinel, err)
}
