package task

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Ref fields (WarehouseRef, ErrorIntegrationRef, etc.) are excluded — they
// are K8s references, not Snowflake values.
func lateInitialize(obj *snowplanev1alpha1.Task, obs *reconciler.Observation[*snowflake.TaskObservation]) bool {
	detail := obs.Detail
	if detail == nil {
		return false
	}

	var modified bool

	if detail.ShowOutput != nil {
		s := detail.ShowOutput

		if reconciler.LateInitNonZero(&obj.Spec.Schedule, s.Schedule) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
			modified = true
		}

		if reconciler.LateInit(&obj.Spec.AllowOverlappingExecution, s.AllowOverlappingExecution) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.ErrorIntegrationName, s.ErrorIntegration) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.When, s.Condition) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.WarehouseName, s.Warehouse) {
			modified = true
		}

		if reconciler.LateInitNonZero(&obj.Spec.Config, s.Config) {
			modified = true
		}

		// Suspend: derive from state string
		if obj.Spec.Suspend == nil {
			suspended := s.State == "suspended"
			obj.Spec.Suspend = &suspended
			modified = true
		}
	}

	if detail.Parameters != nil {
		p := detail.Parameters

		if reconciler.LateInitPtr(&obj.Spec.UserTaskTimeoutMs, p.UserTaskTimeoutMs) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.SuspendTaskAfterNumFailures, p.SuspendTaskAfterNumFailures) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.TaskAutoRetryAttempts, p.TaskAutoRetryAttempts) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.UserTaskMinimumTriggerIntervalInSeconds, p.UserTaskMinimumTriggerIntervalInSeconds) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.TargetCompletionInterval, p.TargetCompletionInterval) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.UserTaskManagedInitialWarehouseSize, p.UserTaskManagedInitialWarehouseSize) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.ServerlessTaskMinStatementSize, p.ServerlessTaskMinStatementSize) {
			modified = true
		}

		if reconciler.LateInitPtr(&obj.Spec.ServerlessTaskMaxStatementSize, p.ServerlessTaskMaxStatementSize) {
			modified = true
		}

		if p.LogLevel != "" && obj.Spec.LogLevel == nil {
			v := snowplanev1alpha1.LogLevel(p.LogLevel)
			obj.Spec.LogLevel = &v
			modified = true
		}
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Task, *snowflake.TaskObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation])(nil)
