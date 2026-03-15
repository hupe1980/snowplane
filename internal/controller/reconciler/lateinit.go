// Package reconciler provides the generic reconciliation loop.
package reconciler

import (
	"strconv"
	"strings"
)

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

// LateInitFromMap sets *target from a string map value if *target is nil and
// the key exists with a non-empty value. Returns true if a value was set.
func LateInitFromMap(target **string, m map[string]string, key string) bool {
	if *target != nil {
		return false
	}

	v, ok := m[key]
	if !ok || v == "" {
		return false
	}

	*target = &v

	return true
}

// LateInitBoolFromMap sets *target from a string map value ("true"/"false")
// if *target is nil and the key exists. Returns true if a value was set.
func LateInitBoolFromMap(target **bool, m map[string]string, key string) bool {
	if *target != nil {
		return false
	}

	v, ok := m[key]
	if !ok || v == "" {
		return false
	}

	b := strings.EqualFold(v, "true")
	*target = &b

	return true
}

// LateInitInt64FromMap sets *target from a string map value (parsed as int64)
// if *target is nil and the key exists with a valid integer string. Returns
// true if a value was set.
func LateInitInt64FromMap(target **int64, m map[string]string, key string) bool {
	if *target != nil {
		return false
	}

	v, ok := m[key]
	if !ok || v == "" {
		return false
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return false
	}

	*target = &n

	return true
}
