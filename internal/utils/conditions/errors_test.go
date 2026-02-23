package conditions

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

func TestSetConditionFromError_NilError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	terminal := SetConditionFromError(obj, nil)
	assert.False(t, terminal, "nil error should not be terminal")
	assert.Empty(t, obj.GetConditions(), "nil error should not set any conditions")
}

func TestSetConditionFromError_TerminalError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	err := snowflake.NewTerminalError(fmt.Errorf("invalid config"))
	terminal := SetConditionFromError(obj, err)

	assert.True(t, terminal, "terminal error should return true")

	// Ready should be False with TerminalError reason.
	ready := Get(obj, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "TerminalError", ready.Reason)
	assert.Contains(t, ready.Message, "invalid config")

	// Synced should be False.
	synced := Get(obj, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)

	// IsTerminal should return true.
	assert.True(t, IsTerminal(obj))
}

func TestSetConditionFromError_TerminalOverwritesRecoverable(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	// Set a recoverable Ready=False first.
	SetNotReady(obj, "ReconcileError", "temporary issue")
	assert.True(t, IsRecoverable(obj))

	// Now set a terminal error — Ready reason should change to terminal.
	err := snowflake.NewTerminalError(fmt.Errorf("permanent failure"))
	SetConditionFromError(obj, err)

	assert.True(t, IsTerminal(obj), "should now be terminal")
	assert.False(t, IsRecoverable(obj), "should no longer be recoverable")
}

func TestSetConditionFromError_RecoverableError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	err := fmt.Errorf("transient network blip")
	terminal := SetConditionFromError(obj, err)

	assert.False(t, terminal, "recoverable error should return false")

	// Ready should be False with ReconcileError reason.
	ready := Get(obj, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, "ReconcileError", ready.Reason)
	assert.Contains(t, ready.Message, "transient network blip")

	// Synced should be False.
	synced := Get(obj, "Synced")
	require.NotNil(t, synced)
	assert.Equal(t, metav1.ConditionFalse, synced.Status)

	// IsRecoverable should be true, IsTerminal false.
	assert.True(t, IsRecoverable(obj))
	assert.False(t, IsTerminal(obj))
}

func TestSetConditionFromError_ConnectionFailedError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	err := fmt.Errorf("connection refused: %w", snowflake.ErrConnectionFailed)
	terminal := SetConditionFromError(obj, err)

	assert.False(t, terminal, "connection error should be recoverable")

	ready := Get(obj, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "ClientCreationFailed", ready.Reason)
	assert.Contains(t, ready.Message, "connection failed")
	assert.True(t, IsRecoverable(obj))
}

func TestSetConditionFromError_WrappedTerminalError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	inner := snowflake.NewTerminalError(snowflake.ErrInsufficientPrivileges)
	err := fmt.Errorf("create failed: %w", inner)
	terminal := SetConditionFromError(obj, err)

	assert.True(t, terminal, "wrapped terminal error should still be terminal")
	assert.True(t, IsTerminal(obj))
}

func TestSetConditionFromError_WrappedConnectionError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	err := fmt.Errorf("observe: %w", fmt.Errorf("dial: %w", snowflake.ErrConnectionFailed))
	terminal := SetConditionFromError(obj, err)

	assert.False(t, terminal)

	ready := Get(obj, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "ClientCreationFailed", ready.Reason)
	assert.True(t, IsRecoverable(obj))
}

func TestSetConditionFromError_JoinedError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	err := errors.Join(fmt.Errorf("field1 invalid"), fmt.Errorf("field2 invalid"))
	terminal := SetConditionFromError(obj, err)

	assert.False(t, terminal, "joined non-terminal errors should be recoverable")
	assert.True(t, IsRecoverable(obj))
}

func TestSetConditionFromError_SuccessOverwritesError(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	// Set an error condition.
	SetConditionFromError(obj, fmt.Errorf("something broke"))
	assert.True(t, IsRecoverable(obj))

	// Simulate success by setting Ready=True.
	SetReady(obj, "all good")
	assert.False(t, IsRecoverable(obj))
	assert.False(t, IsTerminal(obj))
	assert.True(t, IsTrue(obj, "Ready"))
}

func TestSetConditionFromError_RoleSwitchTerminal(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	// Simulates what WithUseRole produces for role switch failures:
	// a terminal error wrapping the role switch error with a helpful message.
	inner := fmt.Errorf("%w: role not granted", snowflake.ErrRoleSwitchFailed)
	err := snowflake.NewTerminalError(fmt.Errorf(
		"USE ROLE %q failed: %w — ensure the role is granted to the service user with: GRANT ROLE %s TO USER <service_user>",
		"ENGINEER", inner, "ENGINEER",
	))

	terminal := SetConditionFromError(obj, err)
	assert.True(t, terminal, "role switch wrapped as terminal should be terminal")

	ready := Get(obj, "Ready")
	require.NotNil(t, ready)
	assert.Equal(t, "TerminalError", ready.Reason)
	assert.Contains(t, ready.Message, "GRANT ROLE ENGINEER TO USER")
	assert.True(t, IsTerminal(obj))
}
