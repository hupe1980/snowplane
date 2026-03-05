package externaloauthintegration

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
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("NilShowOutput", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{
			Detail: &snowflake.ExternalOAuthIntegrationObservation{},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("FillsCommentAndEnabled", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{
			Detail: &snowflake.ExternalOAuthIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
					Comment: "from snowflake",
					Enabled: true,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.NotNil(t, obj.Spec.Comment)
		assert.Equal(t, "from snowflake", *obj.Spec.Comment)
		assert.NotNil(t, obj.Spec.Enabled)
		assert.True(t, *obj.Spec.Enabled)
	})

	t.Run("SkipsNonZeroComment", func(t *testing.T) {
		t.Parallel()
		existing := "already set"
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obj.Spec.Comment = &existing
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{
			Detail: &snowflake.ExternalOAuthIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
					Comment: "from snowflake",
					Enabled: false,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Enabled was filled
		assert.Equal(t, "already set", *obj.Spec.Comment)
		assert.NotNil(t, obj.Spec.Enabled)
		assert.False(t, *obj.Spec.Enabled)
	})

	t.Run("SkipsEmptyComment", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{
			Detail: &snowflake.ExternalOAuthIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
					Comment: "",
					Enabled: true,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Enabled was filled
		assert.Nil(t, obj.Spec.Comment)
		assert.NotNil(t, obj.Spec.Enabled)
	})

	t.Run("AlreadySetNoChange", func(t *testing.T) {
		t.Parallel()
		c := "existing"
		e := true
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obj.Spec.Comment = &c
		obj.Spec.Enabled = &e
		obs := &reconciler.Observation[*snowflake.ExternalOAuthIntegrationObservation]{
			Detail: &snowflake.ExternalOAuthIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
					Comment: "other",
					Enabled: false,
				},
			},
		}
		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "existing", *obj.Spec.Comment)
		assert.True(t, *obj.Spec.Enabled) // unchanged
	})
}
