package warehouse

import (
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// normalizeWarehouseSize converts Snowflake's display format (e.g. "X-Small",
// "2X-Large") to the CRD enum format (e.g. "XSMALL", "2XLARGE").
func normalizeWarehouseSize(raw string) snowplanev1alpha1.WarehouseSize {
	return snowplanev1alpha1.WarehouseSize(strings.ToUpper(strings.ReplaceAll(raw, "-", "")))
}

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.Warehouse, obs *reconciler.Observation[*snowflake.WarehouseObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// ShowOutput fields
	s := detail.ShowOutput

	if s.Type != "" && obj.Spec.WarehouseType == nil {
		v := snowplanev1alpha1.WarehouseType(s.Type)
		obj.Spec.WarehouseType = &v
		modified = true
	}

	if s.Size != "" && obj.Spec.WarehouseSize == nil {
		v := normalizeWarehouseSize(s.Size)
		obj.Spec.WarehouseSize = &v
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.MinClusterCount, s.MinClusterCount) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.MaxClusterCount, s.MaxClusterCount) {
		modified = true
	}

	if s.ScalingPolicy != "" && obj.Spec.ScalingPolicy == nil {
		v := snowplanev1alpha1.ScalingPolicy(s.ScalingPolicy)
		obj.Spec.ScalingPolicy = &v
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.AutoSuspend, s.AutoSuspend) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.AutoResume, s.AutoResume) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.ResourceMonitor, s.ResourceMonitor) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	// Parameters fields
	if detail.Parameters != nil {
		p := detail.Parameters

		if reconciler.LateInitPtr(&obj.Spec.EnableQueryAcceleration, p.EnableQueryAcceleration) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.QueryAccelerationMaxScaleFactor, p.QueryAccelerationMaxScaleFactor) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.MaxConcurrencyLevel, p.MaxConcurrencyLevel) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.StatementQueuedTimeoutInSeconds, p.StatementQueuedTimeoutInSeconds) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.StatementTimeoutInSeconds, p.StatementTimeoutInSeconds) {
			modified = true
		}
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Warehouse, *snowflake.WarehouseObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation])(nil)
