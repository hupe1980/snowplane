package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func ptrBool(b bool) *bool { return &b }

func TestIsForceNew(t *testing.T) {
	t.Parallel()

	assert.True(t, IsForceNew(map[string]string{AnnotationForceNew: "true"}))
	assert.False(t, IsForceNew(map[string]string{AnnotationForceNew: "false"}))
	assert.False(t, IsForceNew(map[string]string{AnnotationForceNew: ""}))
	assert.False(t, IsForceNew(map[string]string{}))
	assert.False(t, IsForceNew(nil))
}

func TestIsCreateOrAlter_ManagementPolicies(t *testing.T) {
	t.Parallel()

	// Defaults to true when nil.
	assert.True(t, ManagementPolicies{}.IsCreateOrAlter())
	assert.True(t, ManagementPolicies{CreateOrAlter: ptrBool(true)}.IsCreateOrAlter())
	assert.False(t, ManagementPolicies{CreateOrAlter: ptrBool(false)}.IsCreateOrAlter())
}

func TestIsDetectOnly_ManagementPolicies(t *testing.T) {
	t.Parallel()

	assert.False(t, ManagementPolicies{}.IsDetectOnly())
	assert.False(t, ManagementPolicies{DriftPolicy: DriftPolicyCorrect}.IsDetectOnly())
	assert.True(t, ManagementPolicies{DriftPolicy: DriftPolicyDetectOnly}.IsDetectOnly())
}

func TestIsAbandonOnDelete(t *testing.T) {
	t.Parallel()

	assert.True(t, IsAbandonOnDelete(map[string]string{AnnotationAbandonOnDelete: "true"}))
	assert.False(t, IsAbandonOnDelete(map[string]string{AnnotationAbandonOnDelete: "false"}))
	assert.False(t, IsAbandonOnDelete(map[string]string{AnnotationAbandonOnDelete: ""}))
	assert.False(t, IsAbandonOnDelete(map[string]string{}))
	assert.False(t, IsAbandonOnDelete(nil))
}
