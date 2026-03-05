package shareddatabase

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills spec fields from the observed Snowflake state during adoption.
func lateInitialize(obj *snowplanev1alpha1.SharedDatabase, obs *reconciler.Observation[*snowflake.SharedDatabaseObservation]) bool {
	if obs.Detail == nil || obs.Detail.ShowOutput == nil {
		return false
	}

	show := obs.Detail.ShowOutput
	modified := false

	modified = reconciler.LateInitNonZero(&obj.Spec.Comment, show.Comment) || modified

	if obs.Detail.Parameters != nil {
		p := obs.Detail.Parameters
		modified = reconciler.LateInitNonZero(&obj.Spec.ExternalVolume, p.ExternalVolume) || modified
		modified = reconciler.LateInitNonZero(&obj.Spec.Catalog, p.Catalog) || modified
		modified = reconciler.LateInitNonZero(&obj.Spec.DefaultDDLCollation, p.DefaultDDLCollation) || modified
		modified = reconciler.LateInitPtr(&obj.Spec.ReplaceInvalidCharacters, p.ReplaceInvalidCharacters) || modified

		if p.StorageSerializationPolicy != "" && obj.Spec.StorageSerializationPolicy == nil {
			v := snowplanev1alpha1.StorageSerializationPolicy(p.StorageSerializationPolicy)
			obj.Spec.StorageSerializationPolicy = &v
			modified = true
		}

		if p.LogLevel != "" && obj.Spec.LogLevel == nil {
			v := snowplanev1alpha1.LogLevel(p.LogLevel)
			obj.Spec.LogLevel = &v
			modified = true
		}

		if p.TraceLevel != "" && obj.Spec.TraceLevel == nil {
			v := snowplanev1alpha1.TraceLevel(p.TraceLevel)
			obj.Spec.TraceLevel = &v
			modified = true
		}
	}

	return modified
}
