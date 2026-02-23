package finalizers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newObj(finalizers ...string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetFinalizers(finalizers)
	return obj
}

func TestHas(t *testing.T) {
	t.Parallel()

	assert.True(t, Has(newObj("a", "b"), "a"))
	assert.True(t, Has(newObj("a", "b"), "b"))
	assert.False(t, Has(newObj("a", "b"), "c"))
	assert.False(t, Has(newObj(), "a"))
}

func TestAdd(t *testing.T) {
	t.Parallel()

	obj := newObj()
	assert.True(t, Add(obj, "my-finalizer"))
	assert.Equal(t, []string{"my-finalizer"}, obj.GetFinalizers())

	// Adding again is a no-op.
	assert.False(t, Add(obj, "my-finalizer"))
	assert.Equal(t, []string{"my-finalizer"}, obj.GetFinalizers())
}

func TestAdd_NoDuplicates(t *testing.T) {
	t.Parallel()

	obj := newObj()
	Add(obj, "f1")
	Add(obj, "f2")
	Add(obj, "f1")
	assert.Equal(t, []string{"f1", "f2"}, obj.GetFinalizers())
}

func TestRemove(t *testing.T) {
	t.Parallel()

	obj := newObj("a", "b", "c")
	assert.True(t, Remove(obj, "b"))
	assert.Equal(t, []string{"a", "c"}, obj.GetFinalizers())

	// Removing non-existent finalizer is a no-op.
	assert.False(t, Remove(obj, "x"))
	assert.Equal(t, []string{"a", "c"}, obj.GetFinalizers())
}

func TestRemove_Empty(t *testing.T) {
	t.Parallel()

	obj := newObj()
	assert.False(t, Remove(obj, "a"))
	assert.Empty(t, obj.GetFinalizers())
}
