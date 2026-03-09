package webhooknotificationintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newWebhookNotificationIntegration() *snowplanev1alpha1.WebhookNotificationIntegration {
	return &snowplanev1alpha1.WebhookNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.WebhookNotificationIntegrationSpec{
			Name:       "MY_WEBHOOK",
			WebhookURL: "https://example.com/hook",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newWebhookNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.WebhookNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.WebhookNotificationIntegrationShowOutput{
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
		obj := newWebhookNotificationIntegration()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.WebhookNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.WebhookNotificationIntegrationShowOutput{
					Comment: "sf",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newWebhookNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newWebhookNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.WebhookNotificationIntegrationObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
