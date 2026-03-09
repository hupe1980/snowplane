package storageintegrationazure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newStorageIntegrationAzure() *snowplanev1alpha1.StorageIntegrationAzure {
	return &snowplanev1alpha1.StorageIntegrationAzure{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.StorageIntegrationAzureSpec{
			Name:                    "MY_SI",
			StorageAllowedLocations: []string{"azure://account.blob.core.windows.net/container/"},
			AzureTenantID:           "tenant-id",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newStorageIntegrationAzure()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAzureObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAzureObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationAzureShowOutput{
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
		obj := newStorageIntegrationAzure()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.StorageIntegrationAzureObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAzureObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationAzureShowOutput{
					Comment: "sf",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newStorageIntegrationAzure()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAzureObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newStorageIntegrationAzure()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAzureObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAzureObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
