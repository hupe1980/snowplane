package share

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newShare() *snowplanev1alpha1.Share {
	return &snowplanev1alpha1.Share{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-share",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ShareSpec{
			Name: "MY_SHARE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from observation", func(t *testing.T) {
		t.Parallel()
		obj := newShare()
		obs := &reconciler.Observation[*snowflake.ShareObservation]{
			Exists: true,
			Detail: &snowflake.ShareObservation{
				ShowOutput: &snowplanev1alpha1.ShareShowOutput{
					Name:    "MY_SHARE",
					Comment: "share comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "share comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		t.Parallel()
		obj := newShare()
		obj.Spec.Comment = ptr("user comment")
		obs := &reconciler.Observation[*snowflake.ShareObservation]{
			Exists: true,
			Detail: &snowflake.ShareObservation{
				ShowOutput: &snowplanev1alpha1.ShareShowOutput{
					Name:    "MY_SHARE",
					Comment: "snowflake comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		t.Parallel()
		obj := newShare()
		obs := &reconciler.Observation[*snowflake.ShareObservation]{
			Exists: true,
			Detail: nil,
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when show output is nil", func(t *testing.T) {
		t.Parallel()
		obj := newShare()
		obs := &reconciler.Observation[*snowflake.ShareObservation]{
			Exists: true,
			Detail: &snowflake.ShareObservation{ShowOutput: nil},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when comment is empty", func(t *testing.T) {
		t.Parallel()
		obj := newShare()
		obs := &reconciler.Observation[*snowflake.ShareObservation]{
			Exists: true,
			Detail: &snowflake.ShareObservation{
				ShowOutput: &snowplanev1alpha1.ShareShowOutput{
					Name:    "MY_SHARE",
					Comment: "",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
