package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newService() *snowplanev1alpha1.Service {
	return &snowplanev1alpha1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ServiceSpec{
			Name:          "MY_SERVICE",
			Specification: "containers:\n- name: main\n  image: my-image",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		t.Parallel()
		obj := newService()
		obs := &reconciler.Observation[*snowflake.ServiceObservation]{
			Exists: true,
			Detail: &snowflake.ServiceObservation{
				ShowOutput: &snowplanev1alpha1.ServiceShowOutput{
					Name:         "MY_SERVICE",
					MinInstances: 2,
					MaxInstances: 5,
					AutoResume:   "true",
					Comment:      "svc comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, int32(2), *obj.Spec.MinInstances)
		assert.Equal(t, int32(5), *obj.Spec.MaxInstances)
		assert.Equal(t, true, *obj.Spec.AutoResume)
		assert.Equal(t, "svc comment", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		t.Parallel()
		obj := newService()
		obj.Spec.MinInstances = ptr(int32(1))
		obj.Spec.MaxInstances = ptr(int32(3))
		obj.Spec.AutoResume = ptr(false)
		obj.Spec.Comment = ptr("user comment")
		obs := &reconciler.Observation[*snowflake.ServiceObservation]{
			Exists: true,
			Detail: &snowflake.ServiceObservation{
				ShowOutput: &snowplanev1alpha1.ServiceShowOutput{
					MinInstances: 10,
					MaxInstances: 20,
					AutoResume:   "true",
					Comment:      "snowflake comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, int32(1), *obj.Spec.MinInstances)
		assert.Equal(t, int32(3), *obj.Spec.MaxInstances)
		assert.Equal(t, false, *obj.Spec.AutoResume)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		t.Parallel()
		obj := newService()
		obs := &reconciler.Observation[*snowflake.ServiceObservation]{
			Exists: true,
			Detail: nil,
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("initializes zero int32 fields", func(t *testing.T) {
		t.Parallel()
		obj := newService()
		obs := &reconciler.Observation[*snowflake.ServiceObservation]{
			Exists: true,
			Detail: &snowflake.ServiceObservation{
				ShowOutput: &snowplanev1alpha1.ServiceShowOutput{
					MinInstances: 0,
					MaxInstances: 0,
					AutoResume:   "",
					Comment:      "",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, int32(0), *obj.Spec.MinInstances)
		assert.Equal(t, int32(0), *obj.Spec.MaxInstances)
	})
}
