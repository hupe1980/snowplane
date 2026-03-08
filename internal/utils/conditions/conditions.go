// Package conditions provides helpers for managing Kubernetes status conditions.
// Value constructors (ReadyCondition, NotReadyCondition, etc.) live in the API
// package (api/v1alpha1). This package owns the ConditionedObject interface,
// the core CRUD accessors, and sanitisation-aware wrappers that strip SQL
// fragments and credentials before storing messages on CRD statuses.
package conditions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/sanitize"
)

// ConditionedObject can get and set Kubernetes status conditions.
// All Snowplane CRD types satisfy this interface via CommonStatus.
type ConditionedObject interface {
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

// ---------------------------------------------------------------------------
// Core condition accessors
// ---------------------------------------------------------------------------

// Get returns the condition with the given type, or nil.
func Get(o ConditionedObject, conditionType string) *metav1.Condition {
	for i, c := range o.GetConditions() {
		if c.Type == conditionType {
			return &o.GetConditions()[i]
		}
	}

	return nil
}

// IsTrue returns true if the named condition has status True.
func IsTrue(o ConditionedObject, conditionType string) bool {
	c := Get(o, conditionType)
	return c != nil && c.Status == metav1.ConditionTrue
}

// Set sets (or updates) the given condition on the object.
// It preserves LastTransitionTime when the status does not change, and
// auto-populates ObservedGeneration when the object exposes GetGeneration().
func Set(o ConditionedObject, condition metav1.Condition) {
	conditions := o.GetConditions()
	condition.LastTransitionTime = metav1.Now()

	// Populate ObservedGeneration if the object exposes its generation.
	if ga, ok := o.(interface{ GetGeneration() int64 }); ok {
		condition.ObservedGeneration = ga.GetGeneration()
	}

	for i, c := range conditions {
		if c.Type == condition.Type {
			// Preserve LastTransitionTime when status stays the same.
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

// Remove removes the condition with the given type.
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
	Set(o, snowplanev1alpha1.ReadyCondition(message))
}

// SetNotReady sets the Ready condition to False.
// The message is sanitised to strip SQL fragments and credentials
// before being stored in the CRD status (readable via kubectl get).
func SetNotReady(o ConditionedObject, reason, message string) {
	Set(o, snowplanev1alpha1.NotReadyCondition(reason, sanitize.ForCondition(message)))
}

// SetSynced sets the Synced condition to True.
func SetSynced(o ConditionedObject, message string) {
	Set(o, snowplanev1alpha1.SyncedCondition(message))
}

// SetNotSynced sets the Synced condition to False.
// The message is sanitised to strip SQL fragments and credentials
// before being stored in the CRD status (readable via kubectl get).
func SetNotSynced(o ConditionedObject, reason, message string) {
	Set(o, snowplanev1alpha1.NotSyncedCondition(reason, sanitize.ForCondition(message)))
}

// SetReferencesResolved sets the ReferencesResolved condition to True.
func SetReferencesResolved(o ConditionedObject, message string) {
	Set(o, snowplanev1alpha1.ReferencesResolvedCondition(message))
}

// SetReferencesNotResolved sets the ReferencesResolved condition to False.
// The message is sanitised to strip SQL fragments and credentials
// before being stored in the CRD status (readable via kubectl get).
func SetReferencesNotResolved(o ConditionedObject, reason, message string) {
	Set(o, snowplanev1alpha1.ReferencesNotResolvedCondition(reason, sanitize.ForCondition(message)))
}

// SetDriftDetected sets the DriftDetected condition to True with the given message.
// The message is sanitised to strip SQL fragments and credentials
// before being stored in the CRD status (readable via kubectl get).
func SetDriftDetected(o ConditionedObject, message string) {
	Set(o, snowplanev1alpha1.DriftDetectedCondition(sanitize.ForCondition(message)))
}

// ClearDriftDetected removes the DriftDetected condition.
func ClearDriftDetected(o ConditionedObject) {
	Remove(o, snowplanev1alpha1.TypeDriftDetected)
}

// IsTerminal returns true when the object has Ready=False with a
// terminal reason (e.g. TerminalError, ValidationFailed, ImmutableField).
func IsTerminal(o ConditionedObject) bool {
	c := Get(o, snowplanev1alpha1.TypeReady)
	if c == nil || c.Status != metav1.ConditionFalse {
		return false
	}

	return snowplanev1alpha1.TerminalReasons[c.Reason]
}

// IsRecoverable returns true when the object has Ready=False with a
// non-terminal reason, indicating a transient error that will be retried.
func IsRecoverable(o ConditionedObject) bool {
	c := Get(o, snowplanev1alpha1.TypeReady)
	if c == nil || c.Status != metav1.ConditionFalse {
		return false
	}

	return !snowplanev1alpha1.TerminalReasons[c.Reason]
}
