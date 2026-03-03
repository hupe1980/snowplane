package resourcemonitor

import (
	"math"
	"strconv"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.ResourceMonitor, obs *reconciler.Observation[*snowflake.ResourceMonitorObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// CreditQuota from ShowOutput (string → *int32, parse and skip invalid/empty).
	if obj.Spec.CreditQuota == nil && detail.ShowOutput.CreditQuota != "" {
		// ShowOutput returns CreditQuota as string (e.g., "100" or "100.00").
		// Parse as float to handle decimal format, then round to int32.
		if v, err := strconv.ParseFloat(detail.ShowOutput.CreditQuota, 64); err == nil {
			iv := int32(math.Round(v))
			obj.Spec.CreditQuota = &iv
			modified = true
		}
	}

	// Frequency from ShowOutput (string → *ResourceMonitorFrequency, skip empty).
	if obj.Spec.Frequency == nil && detail.ShowOutput.Frequency != "" {
		f := snowplanev1alpha1.ResourceMonitorFrequency(detail.ShowOutput.Frequency)
		obj.Spec.Frequency = &f
		modified = true
	}

	// StartTimestamp from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.StartTimestamp, detail.ShowOutput.StartTime) {
		modified = true
	}

	// EndTimestamp from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.EndTimestamp, detail.ShowOutput.EndTime) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.ResourceMonitor, *snowflake.ResourceMonitorObservation] = (*adapter)(nil)
