// Package conditions provides helpers for managing Kubernetes status conditions.
package conditions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// ConditionedObject can get and set conditions.
type ConditionedObject interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

// Get returns the condition with the given type from the object, or nil.
func Get(o ConditionedObject, conditionType string) *metav1.Condition {
	for i, c := range o.GetConditions() {
		if c.Type == conditionType {
			return &o.GetConditions()[i]
		}
	}
	return nil
}

// IsTrue returns true if the condition with the given type is True.
func IsTrue(o ConditionedObject, conditionType string) bool {
	c := Get(o, conditionType)
	return c != nil && c.Status == metav1.ConditionTrue
}

// Set sets the given condition on the object. It replaces any existing
// condition of the same type. If the object implements GetGeneration(),
// the condition's ObservedGeneration is set automatically per K8s API conventions.
func Set(o ConditionedObject, condition metav1.Condition) {
	conditions := o.GetConditions()
	condition.LastTransitionTime = metav1.Now()

	// Populate ObservedGeneration if the object exposes its generation.
	if ga, ok := o.(interface{ GetGeneration() int64 }); ok {
		condition.ObservedGeneration = ga.GetGeneration()
	}

	for i, c := range conditions {
		if c.Type == condition.Type {
			// Only update LastTransitionTime when the status actually changes.
			if c.Status == condition.Status {
				condition.LastTransitionTime = c.LastTransitionTime
			}
			conditions[i] = condition
			o.SetConditions(conditions)
			return
		}
	}
	conditions = append(conditions, condition)
	o.SetConditions(conditions)
}

// Remove removes the condition with the given type from the object.
func Remove(o ConditionedObject, conditionType string) {
	conditions := o.GetConditions()
	result := make([]metav1.Condition, 0, len(conditions))
	for _, c := range conditions {
		if c.Type != conditionType {
			result = append(result, c)
		}
	}
	o.SetConditions(result)
}

// SetReady sets the Ready condition to True with reason "Available".
func SetReady(o ConditionedObject, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  snowplanev1alpha1.ReasonAvailable,
		Message: message,
	})
}

// SetNotReady sets the Ready condition to False.
func SetNotReady(o ConditionedObject, reason, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

// SetSynced sets the Synced condition to True.
func SetSynced(o ConditionedObject, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeSynced,
		Status:  metav1.ConditionTrue,
		Reason:  snowplanev1alpha1.ReasonReconcileSuccess,
		Message: message,
	})
}

// SetNotSynced sets the Synced condition to False.
func SetNotSynced(o ConditionedObject, reason, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeSynced,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

// SetReferencesResolved sets the ReferencesResolved condition to True.
func SetReferencesResolved(o ConditionedObject, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeReferencesResolved,
		Status:  metav1.ConditionTrue,
		Reason:  snowplanev1alpha1.ReasonAvailable,
		Message: message,
	})
}

// SetReferencesNotResolved sets the ReferencesResolved condition to False.
func SetReferencesNotResolved(o ConditionedObject, reason, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeReferencesResolved,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

// SetDriftDetected sets the DriftDetected condition to True with the given message.
func SetDriftDetected(o ConditionedObject, message string) {
	Set(o, metav1.Condition{
		Type:    snowplanev1alpha1.TypeDriftDetected,
		Status:  metav1.ConditionTrue,
		Reason:  snowplanev1alpha1.ReasonDriftDetected,
		Message: message,
	})
}

// ClearDriftDetected removes the DriftDetected condition.
func ClearDriftDetected(o ConditionedObject) {
	Remove(o, snowplanev1alpha1.TypeDriftDetected)
}

// terminalReasons lists Ready condition reasons that indicate a non-recoverable
// (terminal) error — the controller will not retry automatically.
var terminalReasons = map[string]bool{
	snowplanev1alpha1.ReasonTerminalError:    true,
	snowplanev1alpha1.ReasonValidationFailed: true,
	snowplanev1alpha1.ReasonImmutableField:   true,
	snowplanev1alpha1.ReasonResourceExists:   true,
	snowplanev1alpha1.ReasonDeleteBlocked:    true,
}

// IsTerminal returns true when the object has Ready=False with a terminal
// reason (TerminalError, ValidationFailed, ImmutableField, ResourceAlreadyExists).
func IsTerminal(o ConditionedObject) bool {
	c := Get(o, snowplanev1alpha1.TypeReady)
	if c == nil || c.Status != metav1.ConditionFalse {
		return false
	}
	return terminalReasons[c.Reason]
}

// IsRecoverable returns true when the object has Ready=False with a reason
// that is not terminal, indicating a transient error that will be retried.
func IsRecoverable(o ConditionedObject) bool {
	c := Get(o, snowplanev1alpha1.TypeReady)
	if c == nil || c.Status != metav1.ConditionFalse {
		return false
	}
	return !terminalReasons[c.Reason]
}
