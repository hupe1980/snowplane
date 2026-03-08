package emailnotificationintegration

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
func lateInitialize(obj *snowplanev1alpha1.EmailNotificationIntegration, obs *reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]) bool {
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

var _ reconciler.LateInitializer[*snowplanev1alpha1.EmailNotificationIntegration, *snowflake.EmailNotificationIntegrationObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation])(nil)
