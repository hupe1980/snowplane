package emailnotificationintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newEmailNotificationIntegration() *snowplanev1alpha1.EmailNotificationIntegration {
	return &snowplanev1alpha1.EmailNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.EmailNotificationIntegrationSpec{
			Name:              "MY_EMAIL_NI",
			AllowedRecipients: []string{"user@example.com"},
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newEmailNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.EmailNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.EmailNotificationIntegrationShowOutput{
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
		obj := newEmailNotificationIntegration()
		obj.Spec.Comment = testutil.Ptr("user comment")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.EmailNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.EmailNotificationIntegrationShowOutput{
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
		obj := newEmailNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]{
			Exists: true, Detail: nil,
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newEmailNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.EmailNotificationIntegrationObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("skips zero-value comment but sets enabled false", func(t *testing.T) {
		obj := newEmailNotificationIntegration()
		obs := &reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.EmailNotificationIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.EmailNotificationIntegrationShowOutput{
					Comment: "",
					Enabled: false,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Nil(t, obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})
}
