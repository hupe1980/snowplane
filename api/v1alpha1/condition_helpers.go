package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TerminalReasons lists Ready condition reasons that indicate
// non-recoverable (terminal) errors — the controller will not retry.
var TerminalReasons = map[string]bool{
	ReasonTerminalError:    true,
	ReasonValidationFailed: true,
	ReasonImmutableField:   true,
	ReasonResourceExists:   true,
	ReasonDeleteBlocked:    true,
	ReasonConflictDetected: true,
}

// ---------------------------------------------------------------------------
// Condition value constructors — pure functions returning metav1.Condition.
// These follow the Crossplane / ACK pattern of defining typed constructors
// in the API package so external consumers never need to import internal code.
// ---------------------------------------------------------------------------

// ReadyCondition returns a Ready=True condition with reason Available.
func ReadyCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonAvailable,
		Message: message,
	}
}

// NotReadyCondition returns a Ready=False condition.
func NotReadyCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

// SyncedCondition returns a Synced=True condition with reason ReconcileSuccess.
func SyncedCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeSynced,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonReconcileSuccess,
		Message: message,
	}
}

// NotSyncedCondition returns a Synced=False condition.
func NotSyncedCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeSynced,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

// ReferencesResolvedCondition returns a ReferencesResolved=True condition.
func ReferencesResolvedCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeReferencesResolved,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonAvailable,
		Message: message,
	}
}

// ReferencesNotResolvedCondition returns a ReferencesResolved=False condition.
func ReferencesNotResolvedCondition(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeReferencesResolved,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

// DriftDetectedCondition returns a DriftDetected=True condition.
func DriftDetectedCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeDriftDetected,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonDriftDetected,
		Message: message,
	}
}

// CreatingCondition returns a Ready=False condition indicating the resource is being created.
func CreatingCondition() metav1.Condition {
	return metav1.Condition{
		Type:    TypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ReasonCreating,
		Message: "resource is being created",
	}
}

// DeletingCondition returns a Ready=False condition indicating the resource is being deleted.
func DeletingCondition() metav1.Condition {
	return metav1.Condition{
		Type:    TypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  ReasonDeleting,
		Message: "resource is being deleted",
	}
}

// ReconcilePausedCondition returns a Synced=False condition indicating reconciliation is paused.
func ReconcilePausedCondition(message string) metav1.Condition {
	return metav1.Condition{
		Type:    TypeSynced,
		Status:  metav1.ConditionFalse,
		Reason:  ReasonReconcilePaused,
		Message: message,
	}
}
