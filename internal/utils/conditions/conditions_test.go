package conditions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeConditioned implements ConditionedObject.
type fakeConditioned struct {
	conditions []metav1.Condition
}

func (f *fakeConditioned) GetConditions() []metav1.Condition  { return f.conditions }
func (f *fakeConditioned) SetConditions(c []metav1.Condition) { f.conditions = c }

func TestSetAndGet(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK", Message: "ready"})

	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "OK", c.Reason)
}

func TestSet_UpdateExisting(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "NotReady", Message: "starting"})
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK", Message: "done"})

	require.Len(t, obj.GetConditions(), 1)
	c := Get(obj, "Ready")
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "OK", c.Reason)
}

func TestSet_PreservesTransitionTime(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"})
	firstTime := Get(obj, "Ready").LastTransitionTime

	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "StillOK"})
	secondTime := Get(obj, "Ready").LastTransitionTime

	assert.Equal(t, firstTime, secondTime, "transition time should not change when status stays the same")
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	assert.Nil(t, Get(obj, "Missing"))
}

func TestIsTrue(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	assert.False(t, IsTrue(obj, "Ready"))

	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "No"})
	assert.False(t, IsTrue(obj, "Ready"))

	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Yes"})
	assert.True(t, IsTrue(obj, "Ready"))
}

func TestRemove(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"})
	Set(obj, metav1.Condition{Type: "Synced", Status: metav1.ConditionTrue, Reason: "OK"})

	Remove(obj, "Ready")
	assert.Nil(t, Get(obj, "Ready"))
	assert.NotNil(t, Get(obj, "Synced"))
}

func TestRemove_NotExisting(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Remove(obj, "DoesNotExist")
	assert.Empty(t, obj.GetConditions())
}

func TestSetReady(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetReady(obj, "all good")
	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "Available", c.Reason)
	assert.Equal(t, "all good", c.Message)
}

func TestSetNotReady(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetNotReady(obj, "Initializing", "not ready yet")
	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, "Initializing", c.Reason)
}

func TestSetSynced(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetSynced(obj, "synced")
	c := Get(obj, "Synced")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "ReconcileSuccess", c.Reason)
}

func TestSetNotSynced(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetNotSynced(obj, "ReconcileError", "failed")
	c := Get(obj, "Synced")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
}

// fakeGenerationAware implements ConditionedObject and GetGeneration().
type fakeGenerationAware struct {
	fakeConditioned
	generation int64
}

func (f *fakeGenerationAware) GetGeneration() int64 { return f.generation }

func TestSet_PopulatesObservedGeneration(t *testing.T) {
	t.Parallel()
	obj := &fakeGenerationAware{generation: 5}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"})

	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, int64(5), c.ObservedGeneration, "condition should reflect object generation")
}

func TestSet_NoObservedGeneration_WithoutInterface(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	Set(obj, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"})

	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, int64(0), c.ObservedGeneration, "condition should not have generation without interface")
}

// ── Convenience function tests ──────────────────────────────────────────

func TestSetDriftDetectedAndClear(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetDriftDetected(obj, "field comment changed")
	c := Get(obj, "DriftDetected")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, "DriftDetected", c.Reason)

	ClearDriftDetected(obj)
	assert.Nil(t, Get(obj, "DriftDetected"))
}

func TestSetReferencesResolvedAndNotResolved(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	SetReferencesResolved(obj, "all refs found")
	c := Get(obj, "ReferencesResolved")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)

	SetReferencesNotResolved(obj, "NotFound", "database ref missing")
	c = Get(obj, "ReferencesResolved")
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, "NotFound", c.Reason)
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	// No condition → not terminal.
	assert.False(t, IsTerminal(obj))

	// Ready=True → not terminal.
	SetReady(obj, "ok")
	assert.False(t, IsTerminal(obj))

	// Ready=False with TerminalError → terminal.
	SetNotReady(obj, "TerminalError", "bad")
	assert.True(t, IsTerminal(obj))

	// Ready=False with ValidationFailed → terminal.
	SetNotReady(obj, "ValidationFailed", "bad")
	assert.True(t, IsTerminal(obj))

	// Ready=False with ImmutableField → terminal.
	SetNotReady(obj, "ImmutableField", "bad")
	assert.True(t, IsTerminal(obj))

	// Ready=False with ResourceAlreadyExists → terminal.
	SetNotReady(obj, "ResourceAlreadyExists", "bad")
	assert.True(t, IsTerminal(obj))

	// Ready=False with ReconcileError → NOT terminal.
	SetNotReady(obj, "ReconcileError", "retry")
	assert.False(t, IsTerminal(obj))
}

func TestIsRecoverable(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}

	// No condition → not recoverable.
	assert.False(t, IsRecoverable(obj))

	// Ready=True → not recoverable.
	SetReady(obj, "ok")
	assert.False(t, IsRecoverable(obj))

	// Ready=False with ReconcileError → recoverable.
	SetNotReady(obj, "ReconcileError", "retry")
	assert.True(t, IsRecoverable(obj))

	// Ready=False with ClientCreationFailed → recoverable.
	SetNotReady(obj, "ClientCreationFailed", "conn timeout")
	assert.True(t, IsRecoverable(obj))

	// Ready=False with TerminalError → NOT recoverable.
	SetNotReady(obj, "TerminalError", "bad")
	assert.False(t, IsRecoverable(obj))
}

// ── Sanitization tests ──────────────────────────────────────────────────

func TestSetNotReady_SanitizesSQL(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetNotReady(obj, "ReconcileError", `CREATE TABLE "secret_data" (id INT) failed: privilege error`)

	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.NotContains(t, c.Message, "CREATE TABLE")
	assert.NotContains(t, c.Message, "secret_data")
	assert.Contains(t, c.Message, "[SQL redacted]")
}

func TestSetNotSynced_SanitizesDSN(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetNotSynced(obj, "ReconcileError", `error connecting to user@acme.snowflakecomputing.com:443`)

	c := Get(obj, "Synced")
	require.NotNil(t, c)
	assert.NotContains(t, c.Message, "user@acme")
	assert.Contains(t, c.Message, "[connection redacted]")
}

func TestSetDriftDetected_SanitizesSQL(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetDriftDetected(obj, `comment: expected "DROP TABLE evil", found "old"`)

	c := Get(obj, "DriftDetected")
	require.NotNil(t, c)
	assert.NotContains(t, c.Message, "DROP TABLE")
	assert.Contains(t, c.Message, "[SQL redacted]")
}

func TestSetReferencesNotResolved_SanitizesPassword(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	SetReferencesNotResolved(obj, "RefError", `password=SuperSecret123 in ref`)

	c := Get(obj, "ReferencesResolved")
	require.NotNil(t, c)
	assert.NotContains(t, c.Message, "SuperSecret123")
	assert.Contains(t, c.Message, "[REDACTED]")
}

func TestSetReady_NoSanitization(t *testing.T) {
	t.Parallel()
	obj := &fakeConditioned{}
	// SetReady messages are controller-generated, not error-sourced.
	SetReady(obj, "Database created successfully")

	c := Get(obj, "Ready")
	require.NotNil(t, c)
	assert.Equal(t, "Database created successfully", c.Message)
}
