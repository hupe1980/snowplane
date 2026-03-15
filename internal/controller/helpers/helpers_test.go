package helpers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hupe1980/snowplane/internal/controller/helpers"
	"github.com/hupe1980/snowplane/internal/drift"
)

func TestDescribeValue(t *testing.T) {
	t.Parallel()

	t.Run("nil map", func(t *testing.T) {
		assert.Equal(t, "", helpers.DescribeValue(nil, "KEY"))
	})

	t.Run("key exists", func(t *testing.T) {
		m := map[string]string{"KEY": "value"}
		assert.Equal(t, "value", helpers.DescribeValue(m, "KEY"))
	})

	t.Run("key missing", func(t *testing.T) {
		m := map[string]string{"OTHER": "value"}
		assert.Equal(t, "", helpers.DescribeValue(m, "KEY"))
	})
}

func TestStringValueOrEmpty(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		assert.Equal(t, "", helpers.StringValueOrEmpty(nil))
	})

	t.Run("non-nil", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", helpers.StringValueOrEmpty(&s))
	})
}

func TestBoolToString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "true", helpers.BoolToString(true))
	assert.Equal(t, "false", helpers.BoolToString(false))
}

func TestParseCommaList(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, helpers.ParseCommaList(""))
	})

	t.Run("single", func(t *testing.T) {
		assert.Equal(t, []string{"ROLE_A"}, helpers.ParseCommaList("ROLE_A"))
	})

	t.Run("multiple", func(t *testing.T) {
		assert.Equal(t, []string{"A", "B", "C"}, helpers.ParseCommaList("A,B,C"))
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		assert.Equal(t, []string{"A", "B"}, helpers.ParseCommaList(" A , B "))
	})

	t.Run("empty parts skipped", func(t *testing.T) {
		assert.Equal(t, []string{"A", "B"}, helpers.ParseCommaList("A,,B"))
	})
}

func TestParseCommaListFromMap(t *testing.T) {
	t.Parallel()

	t.Run("key missing", func(t *testing.T) {
		m := map[string]string{"OTHER": "val"}
		assert.Nil(t, helpers.ParseCommaListFromMap(m, "KEY"))
	})

	t.Run("key empty", func(t *testing.T) {
		m := map[string]string{"KEY": ""}
		assert.Nil(t, helpers.ParseCommaListFromMap(m, "KEY"))
	})

	t.Run("key present", func(t *testing.T) {
		m := map[string]string{"KEY": "A, B, C"}
		assert.Equal(t, []string{"A", "B", "C"}, helpers.ParseCommaListFromMap(m, "KEY"))
	})
}

func TestCompareListFromDescribeMap(t *testing.T) {
	t.Parallel()

	t.Run("nil map no panic", func(t *testing.T) {
		d := drift.New()
		helpers.CompareListFromDescribeMap(d, "FIELD", []string{"A"}, nil, false)
		assert.False(t, d.Result().HasDrift)
	})

	t.Run("matching lists no drift", func(t *testing.T) {
		d := drift.New()
		m := map[string]string{"ROLES": "ROLE_A, ROLE_B"}
		helpers.CompareListFromDescribeMap(d, "ROLES", []string{"ROLE_B", "ROLE_A"}, m, false)
		assert.False(t, d.Result().HasDrift)
	})

	t.Run("different lists detect drift", func(t *testing.T) {
		d := drift.New()
		m := map[string]string{"ROLES": "ROLE_A, ROLE_C"}
		helpers.CompareListFromDescribeMap(d, "ROLES", []string{"ROLE_A", "ROLE_B"}, m, false)
		assert.True(t, d.Result().HasDrift)
	})

	t.Run("immutable flag propagated", func(t *testing.T) {
		d := drift.New()
		m := map[string]string{"TYPE": "DIFFERENT"}
		helpers.CompareListFromDescribeMap(d, "TYPE", []string{"EXPECTED"}, m, true)
		r := d.Result()
		assert.True(t, r.HasImmutableViolation)
	})
}

func TestStringSlicesEqualFold(t *testing.T) {
	t.Parallel()

	t.Run("equal same order", func(t *testing.T) {
		assert.True(t, helpers.StringSlicesEqualFold([]string{"A", "B"}, []string{"A", "B"}))
	})

	t.Run("equal different order", func(t *testing.T) {
		assert.True(t, helpers.StringSlicesEqualFold([]string{"B", "A"}, []string{"A", "B"}))
	})

	t.Run("case insensitive", func(t *testing.T) {
		assert.True(t, helpers.StringSlicesEqualFold([]string{"role_a"}, []string{"ROLE_A"}))
	})

	t.Run("different lengths", func(t *testing.T) {
		assert.False(t, helpers.StringSlicesEqualFold([]string{"A"}, []string{"A", "B"}))
	})

	t.Run("different values", func(t *testing.T) {
		assert.False(t, helpers.StringSlicesEqualFold([]string{"A", "B"}, []string{"A", "C"}))
	})

	t.Run("duplicates handled", func(t *testing.T) {
		assert.False(t, helpers.StringSlicesEqualFold([]string{"A", "A"}, []string{"A", "B"}))
	})

	t.Run("both nil", func(t *testing.T) {
		assert.True(t, helpers.StringSlicesEqualFold(nil, nil))
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		assert.True(t, helpers.StringSlicesEqualFold([]string{" A "}, []string{"A"}))
	})
}
