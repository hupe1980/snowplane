package notificationintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newNotificationIntegration() *snowplanev1alpha1.NotificationIntegration {
	return &snowplanev1alpha1.NotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ni",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.NotificationIntegrationSpec{
			Name: "TEST_NI",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.NotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.NotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
					Comment: "ni comment",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "ni comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.Enabled)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newNotificationIntegration()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.Enabled = ptr(false)

		obs := &reconciler.Observation[*snowflake.NotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.NotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
					Comment: "sf comment",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newNotificationIntegration()

		modified := lateInitialize(obj, &reconciler.Observation[*snowflake.NotificationIntegrationObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty comment but sets enabled", func(t *testing.T) {
		obj := newNotificationIntegration()

		obs := &reconciler.Observation[*snowflake.NotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.NotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
					Comment: "",
					Enabled: false,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Enabled was set
		assert.Nil(t, obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})
}
