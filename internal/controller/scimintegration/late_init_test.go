package scimintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newSCIMIntegration() *snowplanev1alpha1.SCIMIntegration {
	return &snowplanev1alpha1.SCIMIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.SCIMIntegrationSpec{
			Name:       "MY_SCIM",
			SCIMClient: "GENERIC_SCIM_PROVISIONER",
			RunAsRole:  "GENERIC_SCIM_PROVISIONER",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newSCIMIntegration()
		obs := &reconciler.Observation[*snowflake.SCIMIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SCIMIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.SCIMIntegrationShowOutput{
					Comment: "adopted",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.Enabled)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newSCIMIntegration()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.SCIMIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SCIMIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.SCIMIntegrationShowOutput{
					Comment: "sf",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newSCIMIntegration()
		obs := &reconciler.Observation[*snowflake.SCIMIntegrationObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newSCIMIntegration()
		obs := &reconciler.Observation[*snowflake.SCIMIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.SCIMIntegrationObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
