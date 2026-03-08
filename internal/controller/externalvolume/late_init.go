package externalvolume

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
func lateInitialize(obj *snowplanev1alpha1.ExternalVolume, obs *reconciler.Observation[*snowflake.ExternalVolumeObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// Comment from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	// AllowWrites from ShowOutput (bool → *bool, always set).
	if reconciler.LateInit(&obj.Spec.AllowWrites, detail.ShowOutput.AllowWrites) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.ExternalVolume, *snowflake.ExternalVolumeObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.ExternalVolume, Service, *snowflake.ExternalVolumeObservation])(nil)
