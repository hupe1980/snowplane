package cortexsearchservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func newCortexSearchService() *snowplanev1alpha1.CortexSearchService {
	return &snowplanev1alpha1.CortexSearchService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: snowplanev1alpha1.CortexSearchServiceSpec{
			Name: "MY_CSS",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills nil fields from observation", func(t *testing.T) {
		obj := newCortexSearchService()
		obs := &reconciler.Observation[*snowflake.CortexSearchServiceObservation]{
			Exists: true,
			Detail: &snowflake.CortexSearchServiceObservation{
				ShowOutput: &snowplanev1alpha1.CortexSearchServiceShowOutput{
					Comment:   "adopted comment",
					Warehouse: "MY_WH",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, "adopted comment", *obj.Spec.Comment)
		assert.Equal(t, "MY_WH", *obj.Spec.WarehouseName)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newCortexSearchService()
		obj.Spec.Comment = testutil.Ptr("user comment")
		obj.Spec.WarehouseName = testutil.Ptr("USER_WH")

		obs := &reconciler.Observation[*snowflake.CortexSearchServiceObservation]{
			Exists: true,
			Detail: &snowflake.CortexSearchServiceObservation{
				ShowOutput: &snowplanev1alpha1.CortexSearchServiceShowOutput{
					Comment:   "sf comment",
					Warehouse: "SF_WH",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "USER_WH", *obj.Spec.WarehouseName)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newCortexSearchService()
		obs := &reconciler.Observation[*snowflake.CortexSearchServiceObservation]{
			Exists: true,
			Detail: nil,
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("returns false when ShowOutput is nil", func(t *testing.T) {
		obj := newCortexSearchService()
		obs := &reconciler.Observation[*snowflake.CortexSearchServiceObservation]{
			Exists: true,
			Detail: &snowflake.CortexSearchServiceObservation{ShowOutput: nil},
		}
		assert.False(t, lateInitialize(obj, obs))
	})

	t.Run("skips zero-value strings", func(t *testing.T) {
		obj := newCortexSearchService()
		obs := &reconciler.Observation[*snowflake.CortexSearchServiceObservation]{
			Exists: true,
			Detail: &snowflake.CortexSearchServiceObservation{
				ShowOutput: &snowplanev1alpha1.CortexSearchServiceShowOutput{
					Comment:   "",
					Warehouse: "",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.Comment)
		assert.Nil(t, obj.Spec.WarehouseName)
	})
}
