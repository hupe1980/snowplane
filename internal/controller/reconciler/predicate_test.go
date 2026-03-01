package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// objWithAnnotations returns an unstructured object with the given annotation map.
func objWithAnnotations(ann map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetName("test")
	u.SetNamespace("default")
	u.SetAnnotations(ann)

	return u
}

func TestAnnotationChangedPredicate_Update(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		oldAnn map[string]string
		newAnn map[string]string
		want   bool
	}{
		"NoAnnotations_NoChange": {
			oldAnn: nil,
			newAnn: nil,
			want:   false,
		},
		"EmptyAnnotations_NoChange": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{},
			want:   false,
		},
		"NonSnowplaneAnnotation_Ignored": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "{}"},
			want:   false,
		},
		"ArgoAnnotation_Ignored": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{"argocd.argoproj.io/tracking-id": "some-id"},
			want:   false,
		},
		"ForceNew_Added": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"},
			want:   true,
		},
		"ForceNew_Removed": {
			oldAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"},
			newAnn: map[string]string{},
			want:   true,
		},
		"ForceNew_Changed": {
			oldAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "false"},
			want:   true,
		},
		"ForceNew_Unchanged": {
			oldAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"},
			want:   false,
		},
		"AllowDangerousGrant_Changed": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationAllowDangerousGrant: "true"},
			want:   true,
		},
		"AbandonOnDelete_Changed": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationAbandonOnDelete: "true"},
			want:   true,
		},
		"CreationInitiated_Internal_Ignored": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationCreationInitiated: "true"},
			want:   false,
		},
		"LateInitialized_Internal_Ignored": {
			oldAnn: map[string]string{},
			newAnn: map[string]string{snowplanev1alpha1.AnnotationLateInitialized: "true"},
			want:   false,
		},
		"MultipleSnowplane_OneChanged": {
			oldAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew:            "true",
				snowplanev1alpha1.AnnotationAllowDangerousGrant: "false",
			},
			newAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew:            "true",
				snowplanev1alpha1.AnnotationAllowDangerousGrant: "true",
			},
			want: true,
		},
		"MultipleSnowplane_NoneChanged": {
			oldAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew:            "true",
				snowplanev1alpha1.AnnotationAllowDangerousGrant: "true",
			},
			newAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew:            "true",
				snowplanev1alpha1.AnnotationAllowDangerousGrant: "true",
			},
			want: false,
		},
		"MixedAnnotations_OnlyNonSnowplaneChanged": {
			oldAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew: "true",
				"some-other/annotation":              "old",
			},
			newAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew: "true",
				"some-other/annotation":              "new",
			},
			want: false,
		},
		"MixedAnnotations_SnowplaneChanged": {
			oldAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew: "true",
				"some-other/annotation":              "old",
			},
			newAnn: map[string]string{
				snowplanev1alpha1.AnnotationForceNew: "false",
				"some-other/annotation":              "old",
			},
			want: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := AnnotationChangedPredicate{}
			got := p.Update(event.UpdateEvent{
				ObjectOld: objWithAnnotations(tc.oldAnn),
				ObjectNew: objWithAnnotations(tc.newAnn),
			})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAnnotationChangedPredicate_Update_NilObjects(t *testing.T) {
	t.Parallel()

	p := AnnotationChangedPredicate{}

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: nil,
		ObjectNew: objWithAnnotations(map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"}),
	}), "nil ObjectOld should return false")

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: objWithAnnotations(map[string]string{}),
		ObjectNew: nil,
	}), "nil ObjectNew should return false")

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: nil,
		ObjectNew: nil,
	}), "both nil should return false")
}

func TestAnnotationChangedPredicate_Create(t *testing.T) {
	t.Parallel()

	p := AnnotationChangedPredicate{}

	// Create events inherit from predicate.Funcs which defaults to true.
	assert.True(t, p.Create(event.CreateEvent{
		Object: objWithAnnotations(nil),
	}))
}

func TestAnnotationChangedPredicate_Delete(t *testing.T) {
	t.Parallel()

	p := AnnotationChangedPredicate{}

	// Delete events inherit from predicate.Funcs which defaults to true.
	assert.True(t, p.Delete(event.DeleteEvent{
		Object: objWithAnnotations(nil),
	}))
}

func TestAnnotationChangedPredicate_Generic(t *testing.T) {
	t.Parallel()

	p := AnnotationChangedPredicate{}

	// Generic events inherit from predicate.Funcs which defaults to true.
	assert.True(t, p.Generic(event.GenericEvent{
		Object: objWithAnnotations(nil),
	}))
}

func TestDesiredStateChanged_SpecChange(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	oldObj := objWithAnnotations(nil)
	oldObj.SetGeneration(1)

	newObj := objWithAnnotations(nil)
	newObj.SetGeneration(2)

	assert.True(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}), "generation change should trigger")
}

func TestDesiredStateChanged_AnnotationChange(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	oldObj := objWithAnnotations(map[string]string{})
	oldObj.SetGeneration(1)

	newObj := objWithAnnotations(map[string]string{snowplanev1alpha1.AnnotationAllowDangerousGrant: "true"})
	newObj.SetGeneration(1) // same generation — annotation-only change

	assert.True(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}), "snowplane annotation change without generation bump should trigger")
}

func TestDesiredStateChanged_StatusOnly(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	// Simulate a status-only update: same generation, same annotations.
	oldObj := objWithAnnotations(map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"})
	oldObj.SetGeneration(5)

	newObj := objWithAnnotations(map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"})
	newObj.SetGeneration(5)

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}), "status-only update should not trigger")
}

func TestDesiredStateChanged_NonSnowplaneAnnotation(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	oldObj := objWithAnnotations(map[string]string{})
	oldObj.SetGeneration(3)

	newObj := objWithAnnotations(map[string]string{"app.kubernetes.io/managed-by": "argocd"})
	newObj.SetGeneration(3)

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}), "non-snowplane annotation change should not trigger")
}

func TestDesiredStateChanged_InternalAnnotation(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	oldObj := objWithAnnotations(map[string]string{})
	oldObj.SetGeneration(3)

	newObj := objWithAnnotations(map[string]string{snowplanev1alpha1.AnnotationCreationInitiated: "true"})
	newObj.SetGeneration(3)

	assert.False(t, p.Update(event.UpdateEvent{
		ObjectOld: oldObj,
		ObjectNew: newObj,
	}), "controller-internal annotation change should not trigger")
}

func TestSnowplaneAnnotations_Completeness(t *testing.T) {
	t.Parallel()

	// Verify that all user-facing annotations are in the watched set.
	// Lifecycle policies (adoption, drift, createOrAlter) were promoted to
	// spec.managementPolicies — spec changes bump generation and are handled
	// by GenerationChangedPredicate, no annotation entry needed.
	expected := map[string]bool{
		snowplanev1alpha1.AnnotationForceNew:            true,
		snowplanev1alpha1.AnnotationAllowDangerousGrant: true,
		snowplanev1alpha1.AnnotationAbandonOnDelete:     true,
	}

	actual := make(map[string]bool, len(snowplaneAnnotations))
	for _, a := range snowplaneAnnotations {
		actual[a] = true
	}

	assert.Equal(t, expected, actual, "snowplaneAnnotations should contain exactly the user-facing annotations")

	// Internal annotations MUST NOT be in the watched set.
	for _, a := range snowplaneAnnotations {
		assert.NotEqual(t, snowplanev1alpha1.AnnotationCreationInitiated, a,
			"creation-initiated is controller-internal and must not trigger reconciliation")
		assert.NotEqual(t, snowplanev1alpha1.AnnotationLateInitialized, a,
			"late-initialized is controller-internal and must not trigger reconciliation")
	}
}

func TestDesiredStateChanged_CreateEvent(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	// Create events should always pass through (new resources need reconciliation).
	assert.True(t, p.Create(event.CreateEvent{
		Object: &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		},
	}))
}

func TestDesiredStateChanged_DeleteEvent(t *testing.T) {
	t.Parallel()

	p := DesiredStateChanged()

	// Delete events should always pass through (finalizer processing).
	assert.True(t, p.Delete(event.DeleteEvent{
		Object: &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		},
	}))
}
