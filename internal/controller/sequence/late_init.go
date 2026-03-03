package sequence

import (
	"strconv"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// LateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Start is immutable and intentionally excluded.
func (a *adapter) LateInitialize(obj *snowplanev1alpha1.Sequence, obs *reconciler.Observation[*snowflake.SequenceObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	// Interval is a string in ShowOutput but *int64 in spec.
	if obj.Spec.Increment == nil && s.Interval != "" {
		if v, err := strconv.ParseInt(s.Interval, 10, 64); err == nil {
			obj.Spec.Increment = &v
			modified = true
		}
	}

	if reconciler.LateInitNonZero(&obj.Spec.Ordering, s.Ordering) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	return modified
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.Sequence, *snowflake.SequenceObservation] = (*adapter)(nil)
