package storageintegrationgcs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newStorageIntegrationGCS() *snowplanev1alpha1.StorageIntegrationGCS {
	return &snowplanev1alpha1.StorageIntegrationGCS{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.StorageIntegrationGCSSpec{
			Name:                    "MY_SI",
			StorageAllowedLocations: []string{"gcs://bucket/"},
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newStorageIntegrationGCS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationGCSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationGCSObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationGCSShowOutput{
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
		obj := newStorageIntegrationGCS()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.StorageIntegrationGCSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationGCSObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationGCSShowOutput{
					Comment: "sf",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newStorageIntegrationGCS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationGCSObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newStorageIntegrationGCS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationGCSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationGCSObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
