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

func TestIsCreateOrAlter_ManagementPolicies(t *testing.T) {
	t.Parallel()

	// Defaults to true when nil.
	assert.True(t, ManagementPolicies{}.IsCreateOrAlter())
	assert.True(t, ManagementPolicies{CreateOrAlter: ptr(true)}.IsCreateOrAlter())
	assert.False(t, ManagementPolicies{CreateOrAlter: ptr(false)}.IsCreateOrAlter())
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

func TestAmbiguousBoolAnnotations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		annotations map[string]string
		wantCount   int
	}{
		{"nil annotations", nil, 0},
		{"empty annotations", map[string]string{}, 0},
		{"canonical true", map[string]string{AnnotationForceNew: "true"}, 0},
		{"uppercase True triggers warning", map[string]string{AnnotationForceNew: "True"}, 1},
		{"all-caps TRUE triggers warning", map[string]string{AnnotationForceNew: "TRUE"}, 1},
		{"yes triggers warning", map[string]string{AnnotationForceNew: "yes"}, 1},
		{"1 triggers warning", map[string]string{AnnotationForceNew: "1"}, 1},
		{"false triggers warning", map[string]string{AnnotationForceNew: "false"}, 1},
		{"random value does not warn", map[string]string{AnnotationForceNew: "maybe"}, 0},
		{"multiple annotations with issues", map[string]string{
			AnnotationForceNew:        "True",
			AnnotationAbandonOnDelete: "YES",
		}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			warnings := AmbiguousBoolAnnotations(tt.annotations)
			assert.Len(t, warnings, tt.wantCount)
		})
	}
}
