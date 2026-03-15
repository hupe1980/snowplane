package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLateInit(t *testing.T) {
	t.Run("int32: sets nil target", func(t *testing.T) {
		var target *int32
		assert.True(t, LateInit(&target, int32(42)))
		assert.Equal(t, int32(42), *target)
	})

	t.Run("int32: sets nil target from zero", func(t *testing.T) {
		var target *int32
		assert.True(t, LateInit(&target, int32(0)))
		assert.Equal(t, int32(0), *target)
	})

	t.Run("int32: does not overwrite existing", func(t *testing.T) {
		existing := int32(10)
		target := &existing
		assert.False(t, LateInit(&target, int32(42)))
		assert.Equal(t, int32(10), *target)
	})

	t.Run("int64: sets nil target", func(t *testing.T) {
		var target *int64
		assert.True(t, LateInit(&target, int64(99)))
		assert.Equal(t, int64(99), *target)
	})

	t.Run("bool: sets nil target from true", func(t *testing.T) {
		var target *bool
		assert.True(t, LateInit(&target, true))
		assert.Equal(t, true, *target)
	})

	t.Run("bool: sets nil target from false", func(t *testing.T) {
		var target *bool
		assert.True(t, LateInit(&target, false))
		assert.Equal(t, false, *target)
	})

	t.Run("bool: does not overwrite existing", func(t *testing.T) {
		existing := true
		target := &existing
		assert.False(t, LateInit(&target, false))
		assert.Equal(t, true, *target)
	})
}

func TestLateInitNonZero(t *testing.T) {
	t.Run("string: sets nil target from non-empty value", func(t *testing.T) {
		var target *string
		assert.True(t, LateInitNonZero(&target, "hello"))
		assert.Equal(t, "hello", *target)
	})

	t.Run("string: does not overwrite existing", func(t *testing.T) {
		existing := "existing"
		target := &existing
		assert.False(t, LateInitNonZero(&target, "new"))
		assert.Equal(t, "existing", *target)
	})

	t.Run("string: does not set from empty value", func(t *testing.T) {
		var target *string
		assert.False(t, LateInitNonZero(&target, ""))
		assert.Nil(t, target)
	})

	t.Run("int32: does not set from zero", func(t *testing.T) {
		var target *int32
		assert.False(t, LateInitNonZero(&target, int32(0)))
		assert.Nil(t, target)
	})

	t.Run("int32: sets from non-zero", func(t *testing.T) {
		var target *int32
		assert.True(t, LateInitNonZero(&target, int32(5)))
		assert.Equal(t, int32(5), *target)
	})
}

func TestLateInitPtr(t *testing.T) {
	t.Run("string: sets nil target from non-nil value", func(t *testing.T) {
		var target *string
		val := "hello"
		assert.True(t, LateInitPtr(&target, &val))
		assert.Equal(t, "hello", *target)
		assert.True(t, target != &val) // copy
	})

	t.Run("string: does not overwrite existing", func(t *testing.T) {
		existing := "existing"
		target := &existing
		val := "new"
		assert.False(t, LateInitPtr(&target, &val))
		assert.Equal(t, "existing", *target)
	})

	t.Run("string: does not set from nil", func(t *testing.T) {
		var target *string
		assert.False(t, LateInitPtr[string](&target, nil))
		assert.Nil(t, target)
	})

	t.Run("int32: sets nil target from non-nil value", func(t *testing.T) {
		var target *int32
		val := int32(42)
		assert.True(t, LateInitPtr(&target, &val))
		assert.Equal(t, int32(42), *target)
		assert.True(t, target != &val)
	})

	t.Run("int32: does not overwrite existing", func(t *testing.T) {
		existing := int32(10)
		target := &existing
		val := int32(42)
		assert.False(t, LateInitPtr(&target, &val))
		assert.Equal(t, int32(10), *target)
	})

	t.Run("int32: does not set from nil", func(t *testing.T) {
		var target *int32
		assert.False(t, LateInitPtr[int32](&target, nil))
		assert.Nil(t, target)
	})

	t.Run("bool: sets nil target from non-nil value", func(t *testing.T) {
		var target *bool
		val := true
		assert.True(t, LateInitPtr(&target, &val))
		assert.Equal(t, true, *target)
		assert.True(t, target != &val)
	})

	t.Run("bool: does not overwrite existing", func(t *testing.T) {
		existing := true
		target := &existing
		val := false
		assert.False(t, LateInitPtr(&target, &val))
		assert.Equal(t, true, *target)
	})

	t.Run("bool: does not set from nil", func(t *testing.T) {
		var target *bool
		assert.False(t, LateInitPtr[bool](&target, nil))
		assert.Nil(t, target)
	})
}

func TestLateInitFromMap(t *testing.T) {
	t.Run("sets nil target from map value", func(t *testing.T) {
		var target *string
		m := map[string]string{"KEY": "value"}
		assert.True(t, LateInitFromMap(&target, m, "KEY"))
		assert.Equal(t, "value", *target)
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		existing := "existing"
		target := &existing
		m := map[string]string{"KEY": "new"}
		assert.False(t, LateInitFromMap(&target, m, "KEY"))
		assert.Equal(t, "existing", *target)
	})

	t.Run("does not set from missing key", func(t *testing.T) {
		var target *string
		m := map[string]string{"OTHER": "value"}
		assert.False(t, LateInitFromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("does not set from empty value", func(t *testing.T) {
		var target *string
		m := map[string]string{"KEY": ""}
		assert.False(t, LateInitFromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})
}

func TestLateInitBoolFromMap(t *testing.T) {
	t.Run("sets true", func(t *testing.T) {
		var target *bool
		m := map[string]string{"KEY": "true"}
		assert.True(t, LateInitBoolFromMap(&target, m, "KEY"))
		assert.Equal(t, true, *target)
	})

	t.Run("sets false", func(t *testing.T) {
		var target *bool
		m := map[string]string{"KEY": "false"}
		assert.True(t, LateInitBoolFromMap(&target, m, "KEY"))
		assert.Equal(t, false, *target)
	})

	t.Run("case insensitive", func(t *testing.T) {
		var target *bool
		m := map[string]string{"KEY": "TRUE"}
		assert.True(t, LateInitBoolFromMap(&target, m, "KEY"))
		assert.Equal(t, true, *target)
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		existing := true
		target := &existing
		m := map[string]string{"KEY": "false"}
		assert.False(t, LateInitBoolFromMap(&target, m, "KEY"))
		assert.Equal(t, true, *target)
	})

	t.Run("does not set from missing key", func(t *testing.T) {
		var target *bool
		m := map[string]string{}
		assert.False(t, LateInitBoolFromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})
}

func TestLateInitInt64FromMap(t *testing.T) {
	t.Run("sets valid int64", func(t *testing.T) {
		var target *int64
		m := map[string]string{"KEY": "86400"}
		assert.True(t, LateInitInt64FromMap(&target, m, "KEY"))
		assert.Equal(t, int64(86400), *target)
	})

	t.Run("does not overwrite existing", func(t *testing.T) {
		existing := int64(100)
		target := &existing
		m := map[string]string{"KEY": "200"}
		assert.False(t, LateInitInt64FromMap(&target, m, "KEY"))
		assert.Equal(t, int64(100), *target)
	})

	t.Run("does not set from non-numeric value", func(t *testing.T) {
		var target *int64
		m := map[string]string{"KEY": "abc"}
		assert.False(t, LateInitInt64FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("does not set from missing key", func(t *testing.T) {
		var target *int64
		m := map[string]string{}
		assert.False(t, LateInitInt64FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})

	t.Run("does not set from empty value", func(t *testing.T) {
		var target *int64
		m := map[string]string{"KEY": ""}
		assert.False(t, LateInitInt64FromMap(&target, m, "KEY"))
		assert.Nil(t, target)
	})
}
