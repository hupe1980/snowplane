package oauthintegrationforcustomclients

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
func lateInitialize(obj *snowplanev1alpha1.OAuthIntegrationForCustomClients, obs *reconciler.Observation[*snowflake.OAuthIntegrationForCustomClientsObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	if reconciler.LateInit(&obj.Spec.Enabled, detail.ShowOutput.Enabled) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.OAuthIntegrationForCustomClients, *snowflake.OAuthIntegrationForCustomClientsObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.OAuthIntegrationForCustomClients, Service, *snowflake.OAuthIntegrationForCustomClientsObservation])(nil)
