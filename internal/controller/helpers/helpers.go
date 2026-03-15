package helpers

import (
	"strings"

	"github.com/hupe1980/snowplane/internal/drift"
)

// DescribeValue returns the value for a key from a describe output map.
// Returns "" if the map is nil or the key is not present.
func DescribeValue(m map[string]string, key string) string {
	if m == nil {
		return ""
	}

	return m[key]
}

// StringValueOrEmpty dereferences a string pointer, returning "" if nil.
func StringValueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// BoolToString returns "true" or "false" for a boolean value.
func BoolToString(b bool) string {
	if b {
		return "true"
	}

	return "false"
}

// ParseCommaList splits a comma-separated string into a trimmed, non-empty slice.
func ParseCommaList(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// ParseCommaListFromMap extracts a comma-separated value from a map and splits it.
func ParseCommaListFromMap(m map[string]string, key string) []string {
	v, ok := m[key]
	if !ok || v == "" {
		return nil
	}

	return ParseCommaList(v)
}

// CompareListFromDescribeMap parses a comma-separated describe output value and
// compares it against a desired slice using order-independent, case-insensitive
// set comparison via the drift detector.
func CompareListFromDescribeMap(d *drift.Detector, field string, desired []string, m map[string]string, immutable bool) {
	if m == nil {
		return
	}

	actual := ParseCommaListFromMap(m, field)
	d.CompareStringSliceFold(field, desired, actual, immutable)
}

// StringSlicesEqualFold performs order-independent, case-insensitive comparison
// of two string slices using frequency-map counting. Handles duplicates correctly.
func StringSlicesEqualFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	freq := make(map[string]int, len(a))
	for _, v := range a {
		freq[strings.ToUpper(strings.TrimSpace(v))]++
	}

	for _, v := range b {
		k := strings.ToUpper(strings.TrimSpace(v))
		freq[k]--

		if freq[k] < 0 {
			return false
		}
	}

	return true
}
