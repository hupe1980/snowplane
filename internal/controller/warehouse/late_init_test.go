package warehouse

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newWarehouse() *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-wh",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			Name: "TEST_WH",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newWarehouse()
		obs := &reconciler.Observation[*snowflake.WarehouseObservation]{
			Exists: true,
			Detail: &snowflake.WarehouseObservation{
				ShowOutput: &snowflake.WarehouseShowOutput{
					Type:            "STANDARD",
					Size:            "X-SMALL",
					MinClusterCount: 1,
					MaxClusterCount: 3,
					ScalingPolicy:   "STANDARD",
					AutoSuspend:     600,
					AutoResume:      true,
					ResourceMonitor: "MY_MONITOR",
					Comment:         "wh comment",
				},
				Parameters: &snowflake.WarehouseParameters{
					EnableQueryAcceleration:         ptr(true),
					QueryAccelerationMaxScaleFactor: ptr(int32(8)),
					MaxConcurrencyLevel:             ptr(int32(8)),
					StatementQueuedTimeoutInSeconds: ptr(int32(0)),
					StatementTimeoutInSeconds:       ptr(int32(172800)),
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, snowplanev1alpha1.WarehouseType("STANDARD"), *obj.Spec.WarehouseType)
		assert.Equal(t, snowplanev1alpha1.WarehouseSize("X-SMALL"), *obj.Spec.WarehouseSize)
		assert.Equal(t, int32(1), *obj.Spec.MinClusterCount)
		assert.Equal(t, int32(3), *obj.Spec.MaxClusterCount)
		assert.Equal(t, snowplanev1alpha1.ScalingPolicy("STANDARD"), *obj.Spec.ScalingPolicy)
		assert.Equal(t, int32(600), *obj.Spec.AutoSuspend)
		assert.Equal(t, true, *obj.Spec.AutoResume)
		assert.Equal(t, "MY_MONITOR", *obj.Spec.ResourceMonitor)
		assert.Equal(t, "wh comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.EnableQueryAcceleration)
		assert.Equal(t, int32(8), *obj.Spec.QueryAccelerationMaxScaleFactor)
		assert.Equal(t, int32(8), *obj.Spec.MaxConcurrencyLevel)
		assert.Equal(t, int32(0), *obj.Spec.StatementQueuedTimeoutInSeconds)
		assert.Equal(t, int32(172800), *obj.Spec.StatementTimeoutInSeconds)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newWarehouse()
		wt := snowplanev1alpha1.WarehouseType("SNOWPARK-OPTIMIZED")
		obj.Spec.WarehouseType = &wt
		obj.Spec.Comment = ptr("user comment")

		obs := &reconciler.Observation[*snowflake.WarehouseObservation]{
			Exists: true,
			Detail: &snowflake.WarehouseObservation{
				ShowOutput: &snowflake.WarehouseShowOutput{
					Type:    "STANDARD",
					Size:    "LARGE",
					Comment: "snowflake comment",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // Size was set

		assert.Equal(t, snowplanev1alpha1.WarehouseType("SNOWPARK-OPTIMIZED"), *obj.Spec.WarehouseType)
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, snowplanev1alpha1.WarehouseSize("LARGE"), *obj.Spec.WarehouseSize)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newWarehouse()
		wt := snowplanev1alpha1.WarehouseType("STANDARD")
		obj.Spec.WarehouseType = &wt
		ws := snowplanev1alpha1.WarehouseSize("SMALL")
		obj.Spec.WarehouseSize = &ws
		obj.Spec.MinClusterCount = ptr(int32(1))
		obj.Spec.MaxClusterCount = ptr(int32(1))
		sp := snowplanev1alpha1.ScalingPolicy("STANDARD")
		obj.Spec.ScalingPolicy = &sp
		obj.Spec.AutoSuspend = ptr(int32(300))
		obj.Spec.AutoResume = ptr(true)
		obj.Spec.ResourceMonitor = ptr("rm")
		obj.Spec.Comment = ptr("c")
		obj.Spec.EnableQueryAcceleration = ptr(false)
		obj.Spec.QueryAccelerationMaxScaleFactor = ptr(int32(8))
		obj.Spec.MaxConcurrencyLevel = ptr(int32(8))
		obj.Spec.StatementQueuedTimeoutInSeconds = ptr(int32(0))
		obj.Spec.StatementTimeoutInSeconds = ptr(int32(172800))

		obs := &reconciler.Observation[*snowflake.WarehouseObservation]{
			Exists: true,
			Detail: &snowflake.WarehouseObservation{
				ShowOutput: &snowflake.WarehouseShowOutput{Comment: "other", Size: "XLARGE"},
				Parameters: &snowflake.WarehouseParameters{
					MaxConcurrencyLevel: ptr(int32(99)),
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newWarehouse()
		obs := &reconciler.Observation[*snowflake.WarehouseObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output and parameters", func(t *testing.T) {
		obj := newWarehouse()
		obs := &reconciler.Observation[*snowflake.WarehouseObservation]{
			Exists: true,
			Detail: &snowflake.WarehouseObservation{
				ShowOutput: nil,
				Parameters: nil,
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
