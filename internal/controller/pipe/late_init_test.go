package pipe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newPipe() *snowplanev1alpha1.Pipe {
	return &snowplanev1alpha1.Pipe{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipe",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.PipeSpec{
			Name:          "TEST_PIPE",
			CopyStatement: "COPY INTO my_table FROM @my_stage",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newPipe()
		obs := &reconciler.Observation[*snowflake.PipeObservation]{
			Exists: true,
			Detail: &snowflake.PipeObservation{
				ShowOutput: &snowflake.PipeShowOutput{
					Comment:          "pipe comment",
					ErrorIntegration: "MY_ERROR_INT",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "pipe comment", *obj.Spec.Comment)
		assert.Equal(t, "MY_ERROR_INT", *obj.Spec.ErrorIntegrationName)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newPipe()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.ErrorIntegrationName = ptr("USER_ERR_INT")

		obs := &reconciler.Observation[*snowflake.PipeObservation]{
			Exists: true,
			Detail: &snowflake.PipeObservation{
				ShowOutput: &snowflake.PipeShowOutput{
					Comment:          "snowflake comment",
					ErrorIntegration: "SF_ERR_INT",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "USER_ERR_INT", *obj.Spec.ErrorIntegrationName)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newPipe()

		modified := a.LateInitialize(obj, &reconciler.Observation[*snowflake.PipeObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty string fields", func(t *testing.T) {
		obj := newPipe()

		obs := &reconciler.Observation[*snowflake.PipeObservation]{
			Exists: true,
			Detail: &snowflake.PipeObservation{
				ShowOutput: &snowflake.PipeShowOutput{
					Comment:          "",
					ErrorIntegration: "",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
