package resourcemonitor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newResourceMonitor() *snowplanev1alpha1.ResourceMonitor {
	return &snowplanev1alpha1.ResourceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rm",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			Name: "TEST_RM",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newResourceMonitor()
		obs := &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: &snowflake.ResourceMonitorObservation{
				ShowOutput: &snowflake.ResourceMonitorShowOutput{
					CreditQuota: "100",
					Frequency:   "MONTHLY",
					StartTime:   "2025-01-01 00:00",
					EndTime:     "2025-12-31 23:59",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, int32(100), *obj.Spec.CreditQuota)
		assert.Equal(t, snowplanev1alpha1.ResourceMonitorFrequencyMonthly, *obj.Spec.Frequency)
		assert.Equal(t, "2025-01-01 00:00", *obj.Spec.StartTimestamp)
		assert.Equal(t, "2025-12-31 23:59", *obj.Spec.EndTimestamp)
	})

	t.Run("does not overwrite existing fields", func(t *testing.T) {
		obj := newResourceMonitor()
		freq := snowplanev1alpha1.ResourceMonitorFrequencyWeekly
		obj.Spec.CreditQuota = ptr(int32(50))
		obj.Spec.Frequency = &freq
		obj.Spec.StartTimestamp = ptr("2024-06-01 00:00")

		obs := &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: &snowflake.ResourceMonitorObservation{
				ShowOutput: &snowflake.ResourceMonitorShowOutput{
					CreditQuota: "200",
					Frequency:   "MONTHLY",
					StartTime:   "2025-01-01 00:00",
					EndTime:     "2025-12-31 23:59",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // EndTime was set
		assert.Equal(t, int32(50), *obj.Spec.CreditQuota)
		assert.Equal(t, snowplanev1alpha1.ResourceMonitorFrequencyWeekly, *obj.Spec.Frequency)
		assert.Equal(t, "2024-06-01 00:00", *obj.Spec.StartTimestamp)
		assert.Equal(t, "2025-12-31 23:59", *obj.Spec.EndTimestamp)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newResourceMonitor()

		modified := a.LateInitialize(obj, &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: nil,
		})
		assert.False(t, modified)
	})

	t.Run("handles decimal credit quota string", func(t *testing.T) {
		obj := newResourceMonitor()

		obs := &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: &snowflake.ResourceMonitorObservation{
				ShowOutput: &snowflake.ResourceMonitorShowOutput{
					CreditQuota: "100.00",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, int32(100), *obj.Spec.CreditQuota)
	})

	t.Run("skips empty string fields", func(t *testing.T) {
		obj := newResourceMonitor()

		obs := &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: &snowflake.ResourceMonitorObservation{
				ShowOutput: &snowflake.ResourceMonitorShowOutput{
					CreditQuota: "",
					Frequency:   "",
					StartTime:   "",
					EndTime:     "",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("skips invalid credit quota", func(t *testing.T) {
		obj := newResourceMonitor()

		obs := &reconciler.Observation[*snowflake.ResourceMonitorObservation]{
			Exists: true,
			Detail: &snowflake.ResourceMonitorObservation{
				ShowOutput: &snowflake.ResourceMonitorShowOutput{
					CreditQuota: "not-a-number",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
		assert.Nil(t, obj.Spec.CreditQuota)
	})
}
