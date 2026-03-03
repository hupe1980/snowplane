package databaserole

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.DatabaseRole, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.DatabaseRole, *snowflake.DatabaseRoleObservation] = (*adapter)(nil)
