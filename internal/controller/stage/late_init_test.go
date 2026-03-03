package stage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newStage() *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-stage",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.StageSpec{
			Name: "TEST_STAGE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newStage()
		obs := &reconciler.Observation[*snowflake.StageObservation]{
			Exists: true,
			Detail: &snowflake.StageObservation{
				ShowOutput: &snowflake.StageShowOutput{
					Comment:            "stage comment",
					URL:                "s3://my-bucket/path/",
					StorageIntegration: "MY_INTEGRATION",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "stage comment", *obj.Spec.Comment)
		assert.Equal(t, "s3://my-bucket/path/", *obj.Spec.URL)
		assert.Equal(t, "MY_INTEGRATION", *obj.Spec.StorageIntegration)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newStage()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.URL = ptr("s3://user-bucket/")
		obj.Spec.StorageIntegration = ptr("USER_INT")

		obs := &reconciler.Observation[*snowflake.StageObservation]{
			Exists: true,
			Detail: &snowflake.StageObservation{
				ShowOutput: &snowflake.StageShowOutput{
					Comment:            "snowflake comment",
					URL:                "s3://other-bucket/",
					StorageIntegration: "OTHER_INT",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "s3://user-bucket/", *obj.Spec.URL)
		assert.Equal(t, "USER_INT", *obj.Spec.StorageIntegration)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newStage()

		modified := a.LateInitialize(obj, &reconciler.Observation[*snowflake.StageObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty string fields", func(t *testing.T) {
		obj := newStage()

		obs := &reconciler.Observation[*snowflake.StageObservation]{
			Exists: true,
			Detail: &snowflake.StageObservation{
				ShowOutput: &snowflake.StageShowOutput{
					Comment:            "",
					URL:                "",
					StorageIntegration: "",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
