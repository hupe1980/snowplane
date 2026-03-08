package computepool

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.ComputePool, obs *reconciler.Observation[*snowflake.ComputePoolObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// AutoResume from ShowOutput (string → *bool, skip empty).
	if obj.Spec.AutoResume == nil && detail.ShowOutput.AutoResume != "" {
		v := detail.ShowOutput.AutoResume == "true"
		obj.Spec.AutoResume = &v
		modified = true
	}

	// AutoSuspendSecs from ShowOutput (int32 → *int32).
	if obj.Spec.AutoSuspendSecs == nil {
		v := detail.ShowOutput.AutoSuspend
		obj.Spec.AutoSuspendSecs = &v
		modified = true
	}

	// Comment from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.ComputePool, *snowflake.ComputePoolObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.ComputePool, Service, *snowflake.ComputePoolObservation])(nil)
