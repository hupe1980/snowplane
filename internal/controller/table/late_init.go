package table

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.Table, obs *reconciler.Observation[*snowflake.TableObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.DataRetentionTimeInDays, s.RetentionTime) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.ChangeTracking, s.ChangeTracking) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.EnableSchemaEvolution, s.EnableSchemaEvolution) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Table, *snowflake.TableObservation] = (*adapter)(nil)
