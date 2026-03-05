package alert

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Ref fields (WarehouseRef, etc.) are excluded.
func lateInitialize(obj *snowplanev1alpha1.Alert, obs *reconciler.Observation[*snowflake.AlertObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.WarehouseName, s.Warehouse) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.Schedule, s.Schedule) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	// Suspend: derive from state string
	if obj.Spec.Suspend == nil {
		suspended := s.State == "suspended"
		obj.Spec.Suspend = &suspended
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Alert, *snowflake.AlertObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation])(nil)
