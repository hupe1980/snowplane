package computepool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newComputePool() *snowplanev1alpha1.ComputePool {
	return &snowplanev1alpha1.ComputePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cp",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ComputePoolSpec{
			Name:           "MY_CP",
			MinNodes:       1,
			MaxNodes:       3,
			InstanceFamily: "CPU_X64_XS",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		t.Parallel()
		obj := newComputePool()
		obs := &reconciler.Observation[*snowflake.ComputePoolObservation]{
			Exists: true,
			Detail: &snowflake.ComputePoolObservation{
				ShowOutput: &snowplanev1alpha1.ComputePoolShowOutput{
					Name:           "MY_CP",
					AutoResume:     "true",
					AutoSuspend:    600,
					Comment:        "cp comment",
					InstanceFamily: "CPU_X64_XS",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, true, *obj.Spec.AutoResume)
		assert.Equal(t, int32(600), *obj.Spec.AutoSuspendSecs)
		assert.Equal(t, "cp comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		t.Parallel()
		obj := newComputePool()
		obj.Spec.AutoResume = ptr(false)
		obj.Spec.AutoSuspendSecs = ptr(int32(300))
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.ComputePoolObservation]{
			Exists: true,
			Detail: &snowflake.ComputePoolObservation{
				ShowOutput: &snowplanev1alpha1.ComputePoolShowOutput{
					AutoResume:  "true",
					AutoSuspend: 600,
					Comment:     "snowflake comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, false, *obj.Spec.AutoResume)
		assert.Equal(t, int32(300), *obj.Spec.AutoSuspendSecs)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		t.Parallel()
		obj := newComputePool()
		obs := &reconciler.Observation[*snowflake.ComputePoolObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("initializes zero AutoSuspend", func(t *testing.T) {
		t.Parallel()
		obj := newComputePool()
		obs := &reconciler.Observation[*snowflake.ComputePoolObservation]{
			Exists: true,
			Detail: &snowflake.ComputePoolObservation{
				ShowOutput: &snowplanev1alpha1.ComputePoolShowOutput{
					AutoResume:  "",
					AutoSuspend: 0,
					Comment:     "",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, int32(0), *obj.Spec.AutoSuspendSecs)
	})
}
