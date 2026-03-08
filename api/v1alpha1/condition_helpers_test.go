package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// Condition value constructors
// ---------------------------------------------------------------------------

func TestReadyCondition(t *testing.T) {
	t.Parallel()
	c := ReadyCondition("all good")
	assert.Equal(t, TypeReady, c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, ReasonAvailable, c.Reason)
	assert.Equal(t, "all good", c.Message)
}

func TestNotReadyCondition(t *testing.T) {
	t.Parallel()
	c := NotReadyCondition(ReasonReconcileError, "something broke")
	assert.Equal(t, TypeReady, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonReconcileError, c.Reason)
	assert.Equal(t, "something broke", c.Message)
}

func TestSyncedCondition(t *testing.T) {
	t.Parallel()
	c := SyncedCondition("synced ok")
	assert.Equal(t, TypeSynced, c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, ReasonReconcileSuccess, c.Reason)
	assert.Equal(t, "synced ok", c.Message)
}

func TestNotSyncedCondition(t *testing.T) {
	t.Parallel()
	c := NotSyncedCondition(ReasonReconcileError, "out of sync")
	assert.Equal(t, TypeSynced, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonReconcileError, c.Reason)
	assert.Equal(t, "out of sync", c.Message)
}

func TestReferencesResolvedCondition(t *testing.T) {
	t.Parallel()
	c := ReferencesResolvedCondition("refs found")
	assert.Equal(t, TypeReferencesResolved, c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, ReasonAvailable, c.Reason)
}

func TestReferencesNotResolvedCondition(t *testing.T) {
	t.Parallel()
	c := ReferencesNotResolvedCondition(ReasonRefResolutionFailed, "db not found")
	assert.Equal(t, TypeReferencesResolved, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonRefResolutionFailed, c.Reason)
}

func TestDriftDetectedCondition(t *testing.T) {
	t.Parallel()
	c := DriftDetectedCondition("comment changed")
	assert.Equal(t, TypeDriftDetected, c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, ReasonDriftDetected, c.Reason)
}

func TestCreatingCondition(t *testing.T) {
	t.Parallel()
	c := CreatingCondition()
	assert.Equal(t, TypeReady, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonCreating, c.Reason)
}

func TestDeletingCondition(t *testing.T) {
	t.Parallel()
	c := DeletingCondition()
	assert.Equal(t, TypeReady, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonDeleting, c.Reason)
}

func TestReconcilePausedCondition(t *testing.T) {
	t.Parallel()
	c := ReconcilePausedCondition("paused by annotation")
	assert.Equal(t, TypeSynced, c.Type)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ReasonReconcilePaused, c.Reason)
}
