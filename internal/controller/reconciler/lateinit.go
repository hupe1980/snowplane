// Package reconciler provides the generic reconciliation loop.
package reconciler

// LateInit sets *target to val if *target is nil.
// Always sets, even for zero values — use when zero is a valid value
// (e.g., int32(0), false).
// Returns true if a value was set.
func LateInit[T any](target **T, val T) bool {
	if *target != nil {
		return false
	}

	*target = &val

	return true
}

// LateInitNonZero sets *target to val if *target is nil and val != zero.
// Use for types where the zero value means "not configured"
// (e.g., empty string).
// Returns true if a value was set.
func LateInitNonZero[T comparable](target **T, val T) bool {
	var zero T
	if *target != nil || val == zero {
		return false
	}

	*target = &val

	return true
}

// LateInitPtr sets *target to a copy of *val if *target is nil and
// val is non-nil.
// Returns true if a value was set.
func LateInitPtr[T any](target **T, val *T) bool {
	if *target != nil || val == nil {
		return false
	}

	v := *val
	*target = &v

	return true
}
