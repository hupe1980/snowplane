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
