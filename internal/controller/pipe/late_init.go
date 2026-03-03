package pipe

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.Pipe, obs *reconciler.Observation[*snowflake.PipeObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// Comment from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	// ErrorIntegrationName from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.ErrorIntegrationName, detail.ShowOutput.ErrorIntegration) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Pipe, *snowflake.PipeObservation] = (*adapter)(nil)
