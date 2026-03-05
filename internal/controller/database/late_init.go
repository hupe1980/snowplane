package database

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.Database, obs *reconciler.Observation[*snowflake.DatabaseObservation]) bool {
	detail := obs.Detail
	if detail == nil {
		return false
	}

	var modified bool

	// ShowOutput fields
	if detail.ShowOutput != nil {
		if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
			modified = true
		}
	}

	// Parameters fields
	if detail.Parameters != nil {
		p := detail.Parameters

		if reconciler.LateInitPtr(&obj.Spec.DataRetentionTimeInDays, p.DataRetentionTimeInDays) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.MaxDataExtensionTimeInDays, p.MaxDataExtensionTimeInDays) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.ReplaceInvalidCharacters, p.ReplaceInvalidCharacters) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.DefaultDDLCollation, p.DefaultDDLCollation) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.Catalog, p.Catalog) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.ExternalVolume, p.ExternalVolume) {
			modified = true
		}

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

		if p.MetricLevel != "" && obj.Spec.MetricLevel == nil {
			v := snowplanev1alpha1.MetricLevel(p.MetricLevel)
			obj.Spec.MetricLevel = &v
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

var _ reconciler.LateInitializer[*snowplanev1alpha1.Database, *snowflake.DatabaseObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation])(nil)
