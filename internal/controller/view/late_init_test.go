package view

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newView() *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-view",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ViewSpec{
			Name: "TEST_VIEW",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newView()
		obs := &reconciler.Observation[*snowflake.ViewObservation]{
			Exists: true,
			Detail: &snowflake.ViewObservation{
				ShowOutput: &snowflake.ViewShowOutput{
					Comment:        "view comment",
					ChangeTracking: true,
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "view comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.ChangeTracking)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newView()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.ViewObservation]{
			Exists: true,
			Detail: &snowflake.ViewObservation{
				ShowOutput: &snowflake.ViewShowOutput{
					Comment:        "snowflake comment",
					ChangeTracking: true,
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // ChangeTracking was set

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.ChangeTracking)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newView()
		obj.Spec.Comment = ptr("c")
		obj.Spec.ChangeTracking = ptr(false)

		obs := &reconciler.Observation[*snowflake.ViewObservation]{
			Exists: true,
			Detail: &snowflake.ViewObservation{
				ShowOutput: &snowflake.ViewShowOutput{
					Comment: "other",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newView()
		obs := &reconciler.Observation[*snowflake.ViewObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
