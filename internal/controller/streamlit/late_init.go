package streamlit

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func lateInitialize(obj *snowplanev1alpha1.Streamlit, obs *reconciler.Observation[*snowflake.StreamlitObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.Title, s.Title) {
		modified = true
	}

	if obj.Spec.WarehouseName == nil && obj.Spec.WarehouseRef == nil && s.QueryWarehouse != "" {
		obj.Spec.WarehouseName = &s.QueryWarehouse
		modified = true
	}

	// Late-initialize from describe output.
	if detail.DescribeOutput != nil {
		if reconciler.LateInitNonZero(&obj.Spec.MainFile, detail.DescribeOutput.MainFile) {
			modified = true
		}
	}

	return modified
}

// Compile-time check that BaseAdapter implements LateInitializer.
var _ reconciler.LateInitializer[*snowplanev1alpha1.Streamlit, *snowflake.StreamlitObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.Streamlit, Service, *snowflake.StreamlitObservation])(nil)
