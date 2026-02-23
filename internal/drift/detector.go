// Package drift provides drift detection and correction for Snowflake resources.
package drift

import (
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

const (
	// DriftPolicyDetectOnly reports drift but does not correct it.
	DriftPolicyDetectOnly = "detect-only"

	// DriftPolicyCorrect is the default: detect and correct drift.
	DriftPolicyCorrect = "correct"
)

// FieldChange describes a single field-level difference between desired and observed state.
type FieldChange struct {
	// Field is the Snowflake parameter or attribute name.
	Field string

	// Desired is the value from the Kubernetes spec.
	Desired string

	// Actual is the value observed in Snowflake.
	Actual string

	// Immutable indicates the field cannot be changed after creation.
	Immutable bool
}

// Result holds the outcome of a drift detection pass.
type Result struct {
	// Changes contains all detected field differences.
	Changes []FieldChange

	// HasDrift is true when any non-immutable field differs.
	HasDrift bool

	// HasImmutableViolation is true when an immutable field differs.
	HasImmutableViolation bool
}

// Summary returns a human-readable one-line summary of all changes.
func (r *Result) Summary() string {
	if len(r.Changes) == 0 {
		return "no drift detected"
	}

	parts := make([]string, 0, len(r.Changes))
	for _, c := range r.Changes {
		parts = append(parts, fmt.Sprintf("%s: expected %q, found %q", c.Field, c.Desired, c.Actual))
	}

	return strings.Join(parts, "; ")
}

// FieldDiffs returns only the non-immutable field changes.
func (r *Result) FieldDiffs() []FieldChange {
	var diffs []FieldChange

	for _, c := range r.Changes {
		if !c.Immutable {
			diffs = append(diffs, c)
		}
	}

	return diffs
}

// ImmutableDiffs returns only the immutable field changes.
func (r *Result) ImmutableDiffs() []FieldChange {
	var diffs []FieldChange

	for _, c := range r.Changes {
		if c.Immutable {
			diffs = append(diffs, c)
		}
	}

	return diffs
}

// ImmutableSummary returns a human-readable summary of immutable field violations.
func (r *Result) ImmutableSummary() string {
	immutable := r.ImmutableDiffs()
	if len(immutable) == 0 {
		return "no immutable violations"
	}

	parts := make([]string, 0, len(immutable))
	for _, c := range immutable {
		parts = append(parts, fmt.Sprintf("%s: expected %q, found %q", c.Field, c.Desired, c.Actual))
	}

	return strings.Join(parts, "; ")
}

// Detector is a builder for constructing a drift detection pass.
type Detector struct {
	changes []FieldChange
}

// New creates a new Detector.
func New() *Detector {
	return &Detector{}
}

// CompareString compares a string pointer (spec) vs observed string value.
// If spec is nil, the field is unmanaged and skipped.
func (d *Detector) CompareString(field string, desired *string, actual string, immutable bool) *Detector {
	if desired != nil && *desired != actual {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   *desired,
			Actual:    actual,
			Immutable: immutable,
		})
	}

	return d
}

// CompareStringFold compares a string pointer (spec) vs observed string value
// using case-insensitive comparison. Use this for Snowflake identifiers and
// enum-type fields, since Snowflake uppercases unquoted identifiers.
func (d *Detector) CompareStringFold(field string, desired *string, actual string, immutable bool) *Detector {
	if desired != nil && !strings.EqualFold(*desired, actual) {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   *desired,
			Actual:    actual,
			Immutable: immutable,
		})
	}

	return d
}

// CompareInt32 compares an int32 pointer (spec) vs observed int32 pointer.
func (d *Detector) CompareInt32(field string, desired *int32, actual *int32, immutable bool) *Detector {
	if desired == nil {
		return d
	}

	actualStr := "<unset>"
	actualVal := int32(0)

	if actual != nil {
		actualVal = *actual
		actualStr = fmt.Sprintf("%d", *actual)
	}

	if *desired != actualVal {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   fmt.Sprintf("%d", *desired),
			Actual:    actualStr,
			Immutable: immutable,
		})
	}

	return d
}

// CompareBool compares a bool pointer (spec) vs observed bool pointer.
func (d *Detector) CompareBool(field string, desired *bool, actual *bool, immutable bool) *Detector {
	if desired == nil {
		return d
	}

	actualStr := "<unset>"
	actualVal := false

	if actual != nil {
		actualVal = *actual
		actualStr = fmt.Sprintf("%v", *actual)
	}

	if *desired != actualVal {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   fmt.Sprintf("%v", *desired),
			Actual:    actualStr,
			Immutable: immutable,
		})
	}

	return d
}

// CompareStringValue compares a non-pointer string (spec) vs observed string.
// Both are considered present (no nil-means-unmanaged).
func (d *Detector) CompareStringValue(field, desired, actual string, immutable bool) *Detector {
	if desired != actual {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   desired,
			Actual:    actual,
			Immutable: immutable,
		})
	}

	return d
}

// CompareStringValueFold compares a non-pointer string (spec) vs observed string
// using case-insensitive comparison. Use this for Snowflake identifiers and
// enum-type fields.
func (d *Detector) CompareStringValueFold(field, desired, actual string, immutable bool) *Detector {
	if !strings.EqualFold(desired, actual) {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   desired,
			Actual:    actual,
			Immutable: immutable,
		})
	}

	return d
}

// CompareBoolValue compares non-pointer bool values.
func (d *Detector) CompareBoolValue(field string, desired, actual bool, immutable bool) *Detector {
	if desired != actual {
		d.changes = append(d.changes, FieldChange{
			Field:     field,
			Desired:   fmt.Sprintf("%v", desired),
			Actual:    fmt.Sprintf("%v", actual),
			Immutable: immutable,
		})
	}

	return d
}

// Result builds the final drift result.
func (d *Detector) Result() *Result {
	r := &Result{
		Changes: d.changes,
	}

	for _, c := range d.changes {
		if c.Immutable {
			r.HasImmutableViolation = true
		} else {
			r.HasDrift = true
		}
	}

	return r
}

// IsDetectOnly checks the annotation on a resource and returns true if
// drift should be reported but not corrected.
func IsDetectOnly(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	return annotations[snowplanev1alpha1.AnnotationDriftPolicy] == DriftPolicyDetectOnly
}

// PtrStringFrom converts a pointer to a string-based enum type to *string.
// This eliminates the need for per-type conversion helpers (e.g., ptrStringFromSSP)
// when comparing custom type aliases against drift observations.
func PtrStringFrom[T ~string](v *T) *string {
	if v == nil {
		return nil
	}

	s := string(*v)

	return &s
}
