package conditions

import (
	"fmt"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// SetConditionFromError inspects err and sets the appropriate condition on the
// resource. Terminal errors set Ready=False with ReasonTerminalError.
// Recoverable errors set Ready=False with the appropriate reason.
//
// Returns true if the error is terminal (caller should NOT requeue).
func SetConditionFromError(o ConditionedObject, err error) bool {
	if err == nil {
		return false
	}

	if snowflake.IsTerminalError(err) {
		reason := snowplanev1alpha1.ReasonTerminalError

		SetNotReady(o, reason, err.Error())
		SetNotSynced(o, reason, err.Error())

		return true
	}

	// Recoverable errors — controller will requeue with backoff.
	reason := snowplanev1alpha1.ReasonReconcileError
	msg := fmt.Sprintf("recoverable error: %v", err)

	if snowflake.IsConnectionFailed(err) {
		reason = snowplanev1alpha1.ReasonClientFailed
		msg = fmt.Sprintf("connection failed: %v", err)
	}

	SetNotReady(o, reason, msg)
	SetNotSynced(o, reason, msg)

	return false
}
