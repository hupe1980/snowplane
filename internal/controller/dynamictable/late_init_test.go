package dynamictable

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newDynamicTable() *snowplanev1alpha1.DynamicTable {
	return &snowplanev1alpha1.DynamicTable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dt",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.DynamicTableSpec{
			Name:      "TEST_DT",
			TargetLag: "1 MINUTE",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newDynamicTable()
		obs := &reconciler.Observation[*snowflake.DynamicTableObservation]{
			Exists: true,
			Detail: &snowflake.DynamicTableObservation{
				ShowOutput: &snowplanev1alpha1.DynamicTableShowOutput{
					Comment:     "dt comment",
					Warehouse:   "COMPUTE_WH",
					RefreshMode: "INCREMENTAL",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "dt comment", *obj.Spec.Comment)
		assert.Equal(t, "COMPUTE_WH", *obj.Spec.WarehouseName)
		assert.Equal(t, snowplanev1alpha1.DynamicTableRefreshMode("INCREMENTAL"), *obj.Spec.RefreshMode)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newDynamicTable()
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.DynamicTableObservation]{
			Exists: true,
			Detail: &snowflake.DynamicTableObservation{
				ShowOutput: &snowplanev1alpha1.DynamicTableShowOutput{
					Comment:     "snowflake comment",
					Warehouse:   "WH",
					RefreshMode: "FULL",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Warehouse and RefreshMode set

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, "WH", *obj.Spec.WarehouseName)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newDynamicTable()
		obs := &reconciler.Observation[*snowflake.DynamicTableObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when show output is nil", func(t *testing.T) {
		obj := newDynamicTable()
		obs := &reconciler.Observation[*snowflake.DynamicTableObservation]{
			Exists: true,
			Detail: &snowflake.DynamicTableObservation{
				ShowOutput: nil,
			},
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
