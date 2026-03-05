package secondarydatabase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func TestLateInitialize(t *testing.T) {
	t.Parallel()

	t.Run("NilDetail", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{Detail: nil}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("NilShowOutput", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{ShowOutput: nil},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("CommentAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.Comment = nil
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Comment: "adopted comment"},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted comment", *obj.Spec.Comment)
	})

	t.Run("CommentNotOverwritten", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		// Spec.Comment is already set from newTestSecondaryDatabase.
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Comment: "other comment"},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("EmptyCommentNotAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.Comment = nil
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Comment: ""},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("RetentionAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = nil
		retention := int32(7)
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Name: "x"},
				Parameters: &snowflake.DatabaseParameters{
					DataRetentionTimeInDays: &retention,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		require.NotNil(t, obj.Spec.DataRetentionTimeInDays)
		assert.Equal(t, int32(7), *obj.Spec.DataRetentionTimeInDays)
	})

	t.Run("MaxExtensionAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.MaxDataExtensionTimeInDays = nil
		maxExt := int32(14)
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Name: "x"},
				Parameters: &snowflake.DatabaseParameters{
					MaxDataExtensionTimeInDays: &maxExt,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		require.NotNil(t, obj.Spec.MaxDataExtensionTimeInDays)
		assert.Equal(t, int32(14), *obj.Spec.MaxDataExtensionTimeInDays)
	})

	t.Run("RetentionNotOverwritten", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(10))
		retention := int32(7)
		obs := &reconciler.Observation[*snowflake.SecondaryDatabaseObservation]{
			Detail: &snowflake.SecondaryDatabaseObservation{
				ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{Name: "x"},
				Parameters: &snowflake.DatabaseParameters{
					DataRetentionTimeInDays: &retention,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, int32(10), *obj.Spec.DataRetentionTimeInDays)
	})
}
