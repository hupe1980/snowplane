package apiintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func TestLateInitialize(t *testing.T) {
	t.Parallel()

	t.Run("NilDetail", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{Detail: nil}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("NilShowOutput", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{
			Detail: &snowflake.APIIntegrationObservation{ShowOutput: nil},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("CommentAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.Comment = nil
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{
			Detail: &snowflake.APIIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{Comment: "adopted comment"},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted comment", *obj.Spec.Comment)
	})

	t.Run("CommentNotOverwritten", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{
			Detail: &snowflake.APIIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{Comment: "sf comment"},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("EmptyCommentNotAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.Comment = nil
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{
			Detail: &snowflake.APIIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{Comment: ""},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("EnabledAdopted", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.Enabled = nil
		obs := &reconciler.Observation[*snowflake.APIIntegrationObservation]{
			Detail: &snowflake.APIIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{Enabled: true},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, true, *obj.Spec.Enabled)
	})
}
