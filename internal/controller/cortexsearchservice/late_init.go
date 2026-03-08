package cortexsearchservice

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Ref fields (WarehouseRef, DatabaseRef, SchemaRef) are excluded.
// RefreshMode and Initialize are set-at-create but still useful for adoption
// so the spec faithfully represents the adopted resource.
func lateInitialize(obj *snowplanev1alpha1.CortexSearchService, obs *reconciler.Observation[*snowflake.CortexSearchServiceObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.WarehouseName, s.Warehouse) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.CortexSearchService, *snowflake.CortexSearchServiceObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.CortexSearchService, Service, *snowflake.CortexSearchServiceObservation])(nil)
