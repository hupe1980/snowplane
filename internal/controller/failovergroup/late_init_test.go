package failovergroup

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func TestLateInitialize(t *testing.T) {
	t.Parallel()

	t.Run("NilDetail", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obs := &reconciler.Observation[*snowflake.FailoverGroupObservation]{
			Detail: nil,
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("NilShowOutput", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obs := &reconciler.Observation[*snowflake.FailoverGroupObservation]{
			Detail: &snowflake.FailoverGroupObservation{
				Exists:     true,
				ShowOutput: nil,
			},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("CommentAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obs := &reconciler.Observation[*snowflake.FailoverGroupObservation]{
			Detail: &snowflake.FailoverGroupObservation{
				Exists: true,
				ShowOutput: &snowplanev1alpha1.FailoverGroupShowOutput{
					Name:    "MY_FG",
					Comment: "adopted comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.NotNil(t, obj.Spec.Comment)
		assert.Equal(t, "adopted comment", *obj.Spec.Comment)
	})

	t.Run("CommentNotOverwritten", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obj.Spec.Comment = testutil.Ptr("original")
		obs := &reconciler.Observation[*snowflake.FailoverGroupObservation]{
			Detail: &snowflake.FailoverGroupObservation{
				Exists: true,
				ShowOutput: &snowplanev1alpha1.FailoverGroupShowOutput{
					Name:    "MY_FG",
					Comment: "adopted comment",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "original", *obj.Spec.Comment)
	})

	t.Run("EmptyCommentNotAdopted", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obs := &reconciler.Observation[*snowflake.FailoverGroupObservation]{
			Detail: &snowflake.FailoverGroupObservation{
				Exists: true,
				ShowOutput: &snowplanev1alpha1.FailoverGroupShowOutput{
					Name:    "MY_FG",
					Comment: "",
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
	})
}
