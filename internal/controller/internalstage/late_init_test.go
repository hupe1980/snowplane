package internalstage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newInternalStage() *snowplanev1alpha1.InternalStage {
	return &snowplanev1alpha1.InternalStage{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.InternalStageSpec{
			Name: "MY_STAGE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills comment from observation", func(t *testing.T) {
		obj := newInternalStage()
		obs := &reconciler.Observation[*snowflake.InternalStageObservation]{
			Exists: true,
			Detail: &snowflake.InternalStageObservation{
				ShowOutput: &snowplanev1alpha1.InternalStageShowOutput{
					Comment: "adopted",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted", *obj.Spec.Comment)
	})

	t.Run("does not overwrite existing comment", func(t *testing.T) {
		obj := newInternalStage()
		obj.Spec.Comment = testutil.Ptr("user comment")

		obs := &reconciler.Observation[*snowflake.InternalStageObservation]{
			Exists: true,
			Detail: &snowflake.InternalStageObservation{
				ShowOutput: &snowplanev1alpha1.InternalStageShowOutput{
					Comment: "sf comment",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newInternalStage()
		obs := &reconciler.Observation[*snowflake.InternalStageObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newInternalStage()
		obs := &reconciler.Observation[*snowflake.InternalStageObservation]{
			Exists: true,
			Detail: &snowflake.InternalStageObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("skips empty comment", func(t *testing.T) {
		obj := newInternalStage()
		obs := &reconciler.Observation[*snowflake.InternalStageObservation]{
			Exists: true,
			Detail: &snowflake.InternalStageObservation{
				ShowOutput: &snowplanev1alpha1.InternalStageShowOutput{Comment: ""},
			},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
