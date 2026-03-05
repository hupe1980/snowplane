package storageintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newStorageIntegration() *snowplanev1alpha1.StorageIntegration {
	return &snowplanev1alpha1.StorageIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-si",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.StorageIntegrationSpec{
			Name:            "TEST_SI",
			StorageProvider: "S3",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newStorageIntegration()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationShowOutput{
					Comment: "si comment",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "si comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.Enabled)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newStorageIntegration()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.Enabled = ptr(false)

		obs := &reconciler.Observation[*snowflake.StorageIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationShowOutput{
					Comment: "snowflake comment",
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
		obj := newStorageIntegration()

		modified := lateInitialize(obj, &reconciler.Observation[*snowflake.StorageIntegrationObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("skips empty comment but sets enabled", func(t *testing.T) {
		obj := newStorageIntegration()

		obs := &reconciler.Observation[*snowflake.StorageIntegrationObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationShowOutput{
					Comment: "",
					Enabled: false,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Enabled was set (LateInit always sets)
		assert.Nil(t, obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Enabled)
	})
}
