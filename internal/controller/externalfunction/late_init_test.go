package externalfunction

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func newExternalFunction() *snowplanev1alpha1.ExternalFunction {
	return &snowplanev1alpha1.ExternalFunction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ef",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ExternalFunctionSpec{
			Name:       "MY_FUNC",
			ReturnType: "VARIANT",
			URL:        "https://example.com/api",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from description", func(t *testing.T) {
		t.Parallel()
		obj := newExternalFunction()
		obs := &reconciler.Observation[*snowflake.ExternalFunctionObservation]{
			Exists: true,
			Detail: &snowflake.ExternalFunctionObservation{
				ShowOutput: &snowplanev1alpha1.ExternalFunctionShowOutput{
					Name:        "MY_FUNC",
					Description: "func description",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "func description", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		t.Parallel()
		obj := newExternalFunction()
		comment := "user comment"
		obj.Spec.Comment = &comment
		obs := &reconciler.Observation[*snowflake.ExternalFunctionObservation]{
			Exists: true,
			Detail: &snowflake.ExternalFunctionObservation{
				ShowOutput: &snowplanev1alpha1.ExternalFunctionShowOutput{
					Description: "snowflake desc",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		t.Parallel()
		obj := newExternalFunction()
		obs := &reconciler.Observation[*snowflake.ExternalFunctionObservation]{
			Exists: true,
			Detail: nil,
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when show output is nil", func(t *testing.T) {
		t.Parallel()
		obj := newExternalFunction()
		obs := &reconciler.Observation[*snowflake.ExternalFunctionObservation]{
			Exists: true,
			Detail: &snowflake.ExternalFunctionObservation{ShowOutput: nil},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when description is empty", func(t *testing.T) {
		t.Parallel()
		obj := newExternalFunction()
		obs := &reconciler.Observation[*snowflake.ExternalFunctionObservation]{
			Exists: true,
			Detail: &snowflake.ExternalFunctionObservation{
				ShowOutput: &snowplanev1alpha1.ExternalFunctionShowOutput{
					Description: "",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
