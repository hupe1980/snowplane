package primaryconnection

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func lateInitialize(obj *snowplanev1alpha1.PrimaryConnection, obs *reconciler.Observation[*snowflake.PrimaryConnectionObservation]) bool {
	if obs.Detail == nil || obs.Detail.ShowOutput == nil {
		return false
	}

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, obs.Detail.ShowOutput.Comment) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.PrimaryConnection, *snowflake.PrimaryConnectionObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.PrimaryConnection, Service, *snowflake.PrimaryConnectionObservation])(nil)
