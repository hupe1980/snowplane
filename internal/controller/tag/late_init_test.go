package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newTag() *snowplanev1alpha1.Tag {
	return &snowplanev1alpha1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tag",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TagSpec{
			Name: "TEST_TAG",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from observation", func(t *testing.T) {
		obj := newTag()
		obs := &reconciler.Observation[*snowflake.TagObservation]{
			Exists: true,
			Detail: &snowflake.TagObservation{
				ShowOutput: &snowplanev1alpha1.TagShowOutput{
					Comment: "tag comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "tag comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		obj := newTag()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.TagObservation]{
			Exists: true,
			Detail: &snowflake.TagObservation{
				ShowOutput: &snowplanev1alpha1.TagShowOutput{
					Comment: "snowflake comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when observation is nil", func(t *testing.T) {
		obj := newTag()
		obs := &reconciler.Observation[*snowflake.TagObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("skips empty comment", func(t *testing.T) {
		obj := newTag()
		obs := &reconciler.Observation[*snowflake.TagObservation]{
			Exists: true,
			Detail: &snowflake.TagObservation{
				ShowOutput: &snowplanev1alpha1.TagShowOutput{
					Comment: "",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
	})
}
