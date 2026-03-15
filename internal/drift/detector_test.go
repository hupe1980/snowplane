package drift

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/testutil"
)

func TestDetector_NoDrift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", testutil.Ptr("hello"), "hello", false).
		CompareInt32("MAX_CONCURRENCY_LEVEL", testutil.Ptr(int32(8)), testutil.Ptr(int32(8)), false).
		CompareBool("AUTO_RESUME", testutil.Ptr(true), testutil.Ptr(true), false).
		Result()

	assert.False(t, r.HasDrift)
	assert.False(t, r.HasImmutableViolation)
	assert.Empty(t, r.Changes)
	assert.Equal(t, "no drift detected", r.Summary())
}

func TestDetector_StringDrift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", testutil.Ptr("new"), "old", false).
		Result()

	require.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "COMMENT", r.Changes[0].Field)
	assert.Equal(t, "new", r.Changes[0].Desired)
	assert.Equal(t, "old", r.Changes[0].Actual)
	assert.False(t, r.Changes[0].Immutable)
}

func TestDetector_NilSpecSkipped(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", nil, "something", false).
		CompareInt32("SIZE", nil, testutil.Ptr(int32(42)), false).
		CompareBool("FLAG", nil, testutil.Ptr(true), false).
		Result()

	assert.False(t, r.HasDrift, "nil spec fields should be skipped")
	assert.Empty(t, r.Changes)
}

func TestDetector_Int32Drift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareInt32("MAX_CONCURRENCY_LEVEL", testutil.Ptr(int32(16)), testutil.Ptr(int32(8)), false).
		Result()

	require.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "MAX_CONCURRENCY_LEVEL", r.Changes[0].Field)
	assert.Equal(t, "16", r.Changes[0].Desired)
	assert.Equal(t, "8", r.Changes[0].Actual)
}

func TestDetector_Int32Drift_ActualNil(t *testing.T) {
	t.Parallel()

	r := New().
		CompareInt32("TIMEOUT", testutil.Ptr(int32(30)), nil, false).
		Result()

	require.True(t, r.HasDrift)
	assert.Equal(t, "<unset>", r.Changes[0].Actual)
}

func TestDetector_BoolDrift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareBool("AUTO_RESUME", testutil.Ptr(true), testutil.Ptr(false), false).
		Result()

	require.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "AUTO_RESUME", r.Changes[0].Field)
	assert.Equal(t, "true", r.Changes[0].Desired)
	assert.Equal(t, "false", r.Changes[0].Actual)
}

func TestDetector_BoolDrift_ActualNil(t *testing.T) {
	t.Parallel()

	r := New().
		CompareBool("FLAG", testutil.Ptr(true), nil, false).
		Result()

	require.True(t, r.HasDrift)
	assert.Equal(t, "<unset>", r.Changes[0].Actual)
}

func TestDetector_ImmutableViolation(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("WAREHOUSE_TYPE", "STANDARD", "SNOWPARK-OPTIMIZED", true).
		Result()

	assert.True(t, r.HasImmutableViolation)
	assert.False(t, r.HasDrift, "immutable violations should not count as mutable drift")
	require.Len(t, r.Changes, 1)
	assert.True(t, r.Changes[0].Immutable)
}

func TestDetector_MixedDriftAndImmutable(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("NAME", "WH1", "WH2", true).
		CompareString("COMMENT", testutil.Ptr("new"), "old", false).
		Result()

	assert.True(t, r.HasDrift)
	assert.True(t, r.HasImmutableViolation)
	assert.Len(t, r.Changes, 2)
	assert.Len(t, r.FieldDiffs(), 1) // Only non-immutable
}

func TestDetector_CompareStringValue_NoDrift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("NAME", "WH1", "WH1", false).
		Result()

	assert.False(t, r.HasDrift)
	assert.Empty(t, r.Changes)
}

func TestDetector_CompareBoolValue(t *testing.T) {
	t.Parallel()

	r := New().
		CompareBoolValue("TRANSIENT", true, false, true).
		Result()

	assert.True(t, r.HasImmutableViolation)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "true", r.Changes[0].Desired)
	assert.Equal(t, "false", r.Changes[0].Actual)
}

func TestDetector_CompareBoolValue_NoDrift(t *testing.T) {
	t.Parallel()

	r := New().
		CompareBoolValue("TRANSIENT", true, true, true).
		Result()

	assert.False(t, r.HasDrift)
	assert.False(t, r.HasImmutableViolation)
}

func TestResult_Summary_MultipleChanges(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", testutil.Ptr("new"), "old", false).
		CompareInt32("SIZE", testutil.Ptr(int32(10)), testutil.Ptr(int32(5)), false).
		Result()

	summary := r.Summary()
	assert.Contains(t, summary, `COMMENT: expected "new", found "old"`)
	assert.Contains(t, summary, `SIZE: expected "10", found "5"`)
}

func TestResult_FieldDiffs_ExcludesImmutable(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("NAME", "A", "B", true).
		CompareString("COMMENT", testutil.Ptr("x"), "y", false).
		CompareString("SIZE", testutil.Ptr("L"), "M", false).
		Result()

	diffs := r.FieldDiffs()
	require.Len(t, diffs, 2)
	assert.Equal(t, "COMMENT", diffs[0].Field)
	assert.Equal(t, "SIZE", diffs[1].Field)
}

func TestResult_ImmutableDiffs_IncludesOnlyImmutable(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("NAME", "A", "B", true).
		CompareString("COMMENT", testutil.Ptr("x"), "y", false).
		CompareStringValue("OWNER", "ROLE_A", "ROLE_B", true).
		Result()

	immutable := r.ImmutableDiffs()
	require.Len(t, immutable, 2)
	assert.Equal(t, "NAME", immutable[0].Field)
	assert.Equal(t, "OWNER", immutable[1].Field)
}

func TestResult_ImmutableDiffs_EmptyWhenNoImmutable(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", testutil.Ptr("x"), "y", false).
		Result()

	assert.Empty(t, r.ImmutableDiffs())
}

func TestResult_ImmutableSummary(t *testing.T) {
	t.Parallel()

	r := New().
		CompareStringValue("NAME", "WH1", "WH2", true).
		CompareString("COMMENT", testutil.Ptr("new"), "old", false).
		CompareStringValue("OWNER", "SYSADMIN", "ACCOUNTADMIN", true).
		Result()

	summary := r.ImmutableSummary()
	assert.Contains(t, summary, `NAME: expected "WH1", found "WH2"`)
	assert.Contains(t, summary, `OWNER: expected "SYSADMIN", found "ACCOUNTADMIN"`)
	assert.NotContains(t, summary, "COMMENT", "ImmutableSummary must not include mutable fields")
}

func TestResult_ImmutableSummary_NoViolations(t *testing.T) {
	t.Parallel()

	r := New().
		CompareString("COMMENT", testutil.Ptr("new"), "old", false).
		Result()

	assert.Equal(t, "no immutable violations", r.ImmutableSummary())
}

// --------------------------------------------------------------------------
// Tests: PtrStringFrom generic helper
// --------------------------------------------------------------------------

type testEnum string

const testEnumA testEnum = "VALUE_A"

func TestPtrStringFrom_Nil(t *testing.T) {
	t.Parallel()

	result := PtrStringFrom[testEnum](nil)
	assert.Nil(t, result)
}

func TestPtrStringFrom_NonNil(t *testing.T) {
	t.Parallel()

	v := testEnumA
	result := PtrStringFrom(&v)
	require.NotNil(t, result)
	assert.Equal(t, "VALUE_A", *result)
}

func TestDetector_CompareStringFold_CaseInsensitiveMatch(t *testing.T) {
	t.Parallel()

	desired := "xsmall"
	d := New().CompareStringFold("SIZE", &desired, "XSMALL", false)
	r := d.Result()
	assert.False(t, r.HasDrift, "case-insensitive match should not report drift")
	assert.Empty(t, r.Changes)
}

func TestDetector_CompareStringFold_Drift(t *testing.T) {
	t.Parallel()

	desired := "SMALL"
	d := New().CompareStringFold("SIZE", &desired, "MEDIUM", false)
	r := d.Result()
	assert.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "SIZE", r.Changes[0].Field)
}

func TestDetector_CompareStringFold_NilSkipped(t *testing.T) {
	t.Parallel()

	d := New().CompareStringFold("SIZE", nil, "XSMALL", false)
	r := d.Result()
	assert.False(t, r.HasDrift)
	assert.Empty(t, r.Changes)
}

func TestDetector_CompareStringValueFold_CaseInsensitiveMatch(t *testing.T) {
	t.Parallel()

	d := New().CompareStringValueFold("RESOURCE_MONITOR", "my_monitor", "MY_MONITOR", false)
	r := d.Result()
	assert.False(t, r.HasDrift, "case-insensitive match should not report drift")
	assert.Empty(t, r.Changes)
}

func TestDetector_CompareStringValueFold_Drift(t *testing.T) {
	t.Parallel()

	d := New().CompareStringValueFold("RESOURCE_MONITOR", "monitor_a", "MONITOR_B", false)
	r := d.Result()
	assert.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "RESOURCE_MONITOR", r.Changes[0].Field)
}

// --------------------------------------------------------------------------
// Tests: SafeSummary
// --------------------------------------------------------------------------

func TestResult_SafeSummary_NoDrift(t *testing.T) {
	t.Parallel()
	r := New().Result()
	assert.Equal(t, "no drift detected", r.SafeSummary())
}

func TestResult_SafeSummary_SingleField(t *testing.T) {
	t.Parallel()
	r := New().
		CompareString("COMMENT", testutil.Ptr("secret-value"), "old-secret", false).
		Result()

	safe := r.SafeSummary()
	assert.Equal(t, "drifted fields: COMMENT", safe)
	assert.NotContains(t, safe, "secret-value")
	assert.NotContains(t, safe, "old-secret")
}

func TestResult_SafeSummary_MultipleFields(t *testing.T) {
	t.Parallel()
	r := New().
		CompareString("COMMENT", testutil.Ptr("a"), "b", false).
		CompareString("WAREHOUSE", testutil.Ptr("WH_NEW"), "WH_OLD", false).
		Result()

	safe := r.SafeSummary()
	assert.Equal(t, "drifted fields: COMMENT, WAREHOUSE", safe)
	assert.NotContains(t, safe, "WH_NEW")
	assert.NotContains(t, safe, "WH_OLD")
}

func TestResult_SafeSummary_VsSummary(t *testing.T) {
	t.Parallel()
	r := New().
		CompareString("SCIM_TOKEN", testutil.Ptr("secret-token"), "old-token", false).
		Result()

	// Summary includes values (for debug logs).
	assert.Contains(t, r.Summary(), "secret-token")
	assert.Contains(t, r.Summary(), "old-token")

	// SafeSummary strips values (for status conditions).
	assert.NotContains(t, r.SafeSummary(), "secret-token")
	assert.NotContains(t, r.SafeSummary(), "old-token")
	assert.Contains(t, r.SafeSummary(), "SCIM_TOKEN")
}

// --------------------------------------------------------------------------
// Tests: CompareStringSliceFold
// --------------------------------------------------------------------------

func TestDetector_CompareStringSliceFold_Match(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{"PASSWORD", "SAML"}, []string{"PASSWORD", "SAML"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift)
	assert.Empty(t, r.Changes)
}

func TestDetector_CompareStringSliceFold_CaseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{"PASSWORD", "SAML"}, []string{"password", "saml"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift)
}

func TestDetector_CompareStringSliceFold_OrderIndependent(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{"SAML", "PASSWORD"}, []string{"PASSWORD", "SAML"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift)
}

func TestDetector_CompareStringSliceFold_Drift_DifferentValues(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{"PASSWORD", "SAML"}, []string{"PASSWORD", "OAUTH"}, false)
	r := d.Result()
	assert.True(t, r.HasDrift)
	require.Len(t, r.Changes, 1)
	assert.Equal(t, "AUTH_METHODS", r.Changes[0].Field)
}

func TestDetector_CompareStringSliceFold_Drift_DifferentLength(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{"PASSWORD", "SAML"}, []string{"PASSWORD"}, false)
	r := d.Result()
	assert.True(t, r.HasDrift)
}

func TestDetector_CompareStringSliceFold_NilDesiredSkipped(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", nil, []string{"PASSWORD"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift)
}

func TestDetector_CompareStringSliceFold_EmptyDesiredSkipped(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("AUTH_METHODS", []string{}, []string{"PASSWORD"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift)
}

func TestDetector_CompareStringSliceFold_DuplicateDesired_DetectsDrift(t *testing.T) {
	t.Parallel()
	// desired=["A","A"] actual=["A","B"] — same length, but different multisets.
	d := New().CompareStringSliceFold("TAGS", []string{"A", "A"}, []string{"A", "B"}, false)
	r := d.Result()
	assert.True(t, r.HasDrift, "duplicate desired values with same-length actual should detect drift")
	assert.Len(t, r.Changes, 1)
}

func TestDetector_CompareStringSliceFold_DuplicateBothSides_Match(t *testing.T) {
	t.Parallel()
	d := New().CompareStringSliceFold("TAGS", []string{"A", "A"}, []string{"A", "A"}, false)
	r := d.Result()
	assert.False(t, r.HasDrift, "identical multisets should not report drift")
}
