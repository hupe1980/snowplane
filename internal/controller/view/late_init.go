package view

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.View, obs *reconciler.Observation[*snowflake.ViewObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.ChangeTracking, s.ChangeTracking) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.View, *snowflake.ViewObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation])(nil)
