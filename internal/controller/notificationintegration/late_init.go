package notificationintegration

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
func lateInitialize(obj *snowplanev1alpha1.NotificationIntegration, obs *reconciler.Observation[*snowflake.NotificationIntegrationObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	var modified bool

	// Comment from ShowOutput (string → *string, skip empty).
	if reconciler.LateInitNonZero(&obj.Spec.Comment, detail.ShowOutput.Comment) {
		modified = true
	}

	// Enabled from ShowOutput (bool → *bool, always set).
	if reconciler.LateInit(&obj.Spec.Enabled, detail.ShowOutput.Enabled) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.NotificationIntegration, *snowflake.NotificationIntegrationObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation])(nil)
