package externalstage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newExternalStage() *snowplanev1alpha1.ExternalStage {
	return &snowplanev1alpha1.ExternalStage{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.ExternalStageSpec{
			Name: "MY_STAGE",
			URL:  "s3://bucket/path/",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newExternalStage()
		obs := &reconciler.Observation[*snowflake.ExternalStageObservation]{
			Exists: true,
			Detail: &snowflake.ExternalStageObservation{
				ShowOutput: &snowplanev1alpha1.ExternalStageShowOutput{
					Comment:            "adopted",
					StorageIntegration: "MY_SI",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted", *obj.Spec.Comment)
		assert.Equal(t, "MY_SI", *obj.Spec.StorageIntegration)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newExternalStage()
		obj.Spec.Comment = testutil.Ptr("user comment")
		obj.Spec.StorageIntegration = testutil.Ptr("USER_SI")

		obs := &reconciler.Observation[*snowflake.ExternalStageObservation]{
			Exists: true,
			Detail: &snowflake.ExternalStageObservation{
				ShowOutput: &snowplanev1alpha1.ExternalStageShowOutput{
					Comment:            "sf comment",
					StorageIntegration: "SF_SI",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "USER_SI", *obj.Spec.StorageIntegration)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newExternalStage()
		obs := &reconciler.Observation[*snowflake.ExternalStageObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newExternalStage()
		obs := &reconciler.Observation[*snowflake.ExternalStageObservation]{
			Exists: true,
			Detail: &snowflake.ExternalStageObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("skips zero-value strings", func(t *testing.T) {
		obj := newExternalStage()
		obs := &reconciler.Observation[*snowflake.ExternalStageObservation]{
			Exists: true,
			Detail: &snowflake.ExternalStageObservation{
				ShowOutput: &snowplanev1alpha1.ExternalStageShowOutput{
					Comment:            "",
					StorageIntegration: "",
				},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
