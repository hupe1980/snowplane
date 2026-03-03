package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestAssertCondition(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetReady(db, "ok")

	AssertCondition(t, db, snowplanev1alpha1.TypeReady, metav1.ConditionTrue, snowplanev1alpha1.ReasonAvailable)
}

func TestAssertReady(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetReady(db, "ok")

	AssertReady(t, db)
}

func TestAssertNotReady(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetNotReady(db, snowplanev1alpha1.ReasonReconcileError, "boom")

	AssertNotReady(t, db, snowplanev1alpha1.ReasonReconcileError)
}

func TestAssertSynced(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetSynced(db, "ok")

	AssertSynced(t, db)
}

func TestAssertNotSynced(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetNotSynced(db, snowplanev1alpha1.ReasonReconcileError, "boom")

	AssertNotSynced(t, db, snowplanev1alpha1.ReasonReconcileError)
}

func TestAssertTerminal(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}
	conditions.SetNotReady(db, snowplanev1alpha1.ReasonTerminalError, "unrecoverable")

	assertTerminal(t, db, snowplanev1alpha1.ReasonTerminalError)
}

func TestAssertNoCondition(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{}

	assertNoCondition(t, db, snowplanev1alpha1.TypeDriftDetected)
}

func TestPtr(t *testing.T) {
	t.Parallel()

	s := Ptr("hello")
	assert.Equal(t, "hello", *s)

	i := Ptr(int32(42))
	assert.Equal(t, int32(42), *i)

	b := Ptr(true)
	assert.True(t, *b)
}

func TestContainsEvent(t *testing.T) {
	t.Parallel()

	events := []string{"Normal Created db", "Warning DriftDetected something"}

	assert.True(t, ContainsEvent(events, "DriftDetected"))
	assert.False(t, ContainsEvent(events, "Deleted"))
}

func TestDrainEvents(t *testing.T) {
	t.Parallel()

	rec := newTestRecorder()
	rec.Events <- "Normal Created test event"
	rec.Events <- "Warning Error bad thing"

	events := DrainEvents(rec)
	assert.Len(t, events, 2)
}
