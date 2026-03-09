package externalvolume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newExternalVolume() *snowplanev1alpha1.ExternalVolume {
	return &snowplanev1alpha1.ExternalVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.ExternalVolumeSpec{
			Name: "MY_VOL",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newExternalVolume()
		obs := &reconciler.Observation[*snowflake.ExternalVolumeObservation]{
			Exists: true,
			Detail: &snowflake.ExternalVolumeObservation{
				ShowOutput: &snowplanev1alpha1.ExternalVolumeShowOutput{
					Comment:     "adopted",
					AllowWrites: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.AllowWrites)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newExternalVolume()
		obj.Spec.Comment = testutil.Ptr("user comment")
		obj.Spec.AllowWrites = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.ExternalVolumeObservation]{
			Exists: true,
			Detail: &snowflake.ExternalVolumeObservation{
				ShowOutput: &snowplanev1alpha1.ExternalVolumeShowOutput{
					Comment:     "sf comment",
					AllowWrites: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.AllowWrites)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newExternalVolume()
		obs := &reconciler.Observation[*snowflake.ExternalVolumeObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newExternalVolume()
		obs := &reconciler.Observation[*snowflake.ExternalVolumeObservation]{
			Exists: true,
			Detail: &snowflake.ExternalVolumeObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("sets AllowWrites false when zero-value", func(t *testing.T) {
		obj := newExternalVolume()
		obs := &reconciler.Observation[*snowflake.ExternalVolumeObservation]{
			Exists: true,
			Detail: &snowflake.ExternalVolumeObservation{
				ShowOutput: &snowplanev1alpha1.ExternalVolumeShowOutput{
					Comment:     "",
					AllowWrites: false,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Nil(t, obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.AllowWrites)
	})
}
