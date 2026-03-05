package stage

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.Stage, obs *reconciler.Observation[*snowflake.StageObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// Comment from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	// URL from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.URL, detail.ShowOutput.URL) {
		modified = true
	}

	// StorageIntegration from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.StorageIntegration, detail.ShowOutput.StorageIntegration) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Stage, *snowflake.StageObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation])(nil)
