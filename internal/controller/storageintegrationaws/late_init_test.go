package storageintegrationaws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newStorageIntegrationAWS() *snowplanev1alpha1.StorageIntegrationAWS {
	return &snowplanev1alpha1.StorageIntegrationAWS{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.StorageIntegrationAWSSpec{
			Name:                    "MY_SI",
			StorageAllowedLocations: []string{"s3://bucket/"},
			StorageAWSRoleARN:       "arn:aws:iam::role/test",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newStorageIntegrationAWS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAWSObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationAWSShowOutput{
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
		obj := newStorageIntegrationAWS()
		obj.Spec.Comment = testutil.Ptr("user")
		obj.Spec.Enabled = testutil.Ptr(false)

		obs := &reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAWSObservation{
				ShowOutput: &snowplanev1alpha1.StorageIntegrationAWSShowOutput{
					Comment: "sf",
					Enabled: true,
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newStorageIntegrationAWS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]{Exists: true, Detail: nil}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newStorageIntegrationAWS()
		obs := &reconciler.Observation[*snowflake.StorageIntegrationAWSObservation]{
			Exists: true,
			Detail: &snowflake.StorageIntegrationAWSObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})
}
