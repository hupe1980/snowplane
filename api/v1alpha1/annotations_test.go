package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsForceNew(t *testing.T) {
	t.Parallel()

	assert.True(t, IsForceNew(map[string]string{AnnotationForceNew: "true"}))
	assert.False(t, IsForceNew(map[string]string{AnnotationForceNew: "false"}))
	assert.False(t, IsForceNew(map[string]string{AnnotationForceNew: ""}))
	assert.False(t, IsForceNew(map[string]string{}))
	assert.False(t, IsForceNew(nil))
}

func TestIsCreateOrAlter(t *testing.T) {
	t.Parallel()

	// Enabled by default (annotation absent).
	assert.True(t, IsCreateOrAlter(map[string]string{}))
	assert.True(t, IsCreateOrAlter(nil))
	assert.True(t, IsCreateOrAlter(map[string]string{AnnotationUseCreateOrAlter: "true"}))
	// Opt-out.
	assert.False(t, IsCreateOrAlter(map[string]string{AnnotationUseCreateOrAlter: "false"}))
}
