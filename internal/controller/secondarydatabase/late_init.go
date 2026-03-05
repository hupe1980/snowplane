package secondarydatabase

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills spec fields from the observed Snowflake state during adoption.
func lateInitialize(obj *snowplanev1alpha1.SecondaryDatabase, obs *reconciler.Observation[*snowflake.SecondaryDatabaseObservation]) bool {
	if obs.Detail == nil || obs.Detail.ShowOutput == nil {
		return false
	}

	show := obs.Detail.ShowOutput
	modified := false

	modified = reconciler.LateInitNonZero(&obj.Spec.Comment, show.Comment) || modified

	if obs.Detail.Parameters != nil {
		modified = reconciler.LateInitPtr(&obj.Spec.DataRetentionTimeInDays, obs.Detail.Parameters.DataRetentionTimeInDays) || modified
		modified = reconciler.LateInitPtr(&obj.Spec.MaxDataExtensionTimeInDays, obs.Detail.Parameters.MaxDataExtensionTimeInDays) || modified
	}

	return modified
}
