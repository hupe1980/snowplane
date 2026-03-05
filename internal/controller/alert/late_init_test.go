package alert

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func ptr[T any](v T) *T { return &v }

func newAlert() *snowplanev1alpha1.Alert {
	return &snowplanev1alpha1.Alert{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-alert",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AlertSpec{
			Name: "TEST_ALERT",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newAlert()
		obs := &reconciler.Observation[*snowflake.AlertObservation]{
			Exists: true,
			Detail: &snowflake.AlertObservation{
				ShowOutput: &snowplanev1alpha1.AlertShowOutput{
					Warehouse: "COMPUTE_WH",
					Schedule:  "USING CRON 0 9 * * * UTC",
					Comment:   "alert comment",
					State:     "suspended",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "COMPUTE_WH", *obj.Spec.WarehouseName)
		assert.Equal(t, "USING CRON 0 9 * * * UTC", *obj.Spec.Schedule)
		assert.Equal(t, "alert comment", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.Suspend)
	})

	t.Run("suspend is false when state is started", func(t *testing.T) {
		obj := newAlert()
		obs := &reconciler.Observation[*snowflake.AlertObservation]{
			Exists: true,
			Detail: &snowflake.AlertObservation{
				ShowOutput: &snowplanev1alpha1.AlertShowOutput{
					State: "started",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, false, *obj.Spec.Suspend)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newAlert()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.Suspend = ptr(false)

		obs := &reconciler.Observation[*snowflake.AlertObservation]{
			Exists: true,
			Detail: &snowflake.AlertObservation{
				ShowOutput: &snowplanev1alpha1.AlertShowOutput{
					Comment:   "snowflake comment",
					State:     "suspended",
					Warehouse: "WH",
				},
			},
		}

		modified := lateInitialize(obj, obs)
		assert.True(t, modified) // Warehouse was set

		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Suspend)
		assert.Equal(t, "WH", *obj.Spec.WarehouseName)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newAlert()
		obs := &reconciler.Observation[*snowflake.AlertObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := lateInitialize(obj, obs)
		assert.False(t, modified)
	})
}
