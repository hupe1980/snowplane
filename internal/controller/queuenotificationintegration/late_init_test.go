package queuenotificationintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newQueueNotificationIntegration() *snowplanev1alpha1.QueueNotificationIntegration {
	return &snowplanev1alpha1.QueueNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.QueueNotificationIntegrationSpec{
			Name:                 "MY_QNI",
			NotificationProvider: "AWS_SNS",
			Direction:            "OUTBOUND",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newQueueNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.QueueNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.QueueNotificationIntegrationShowOutput{
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
		obj := newQueueNotificationIntegration()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.QueueNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.QueueNotificationIntegrationShowOutput{
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
		obj := newQueueNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newQueueNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.QueueNotificationIntegrationObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
