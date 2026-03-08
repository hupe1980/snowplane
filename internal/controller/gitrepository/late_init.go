package gitrepository

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

func lateInitialize(obj *snowplanev1alpha1.GitRepository, obs *reconciler.Observation[*snowflake.GitRepositoryObservation]) bool {
	detail := obs.Detail
	if detail == nil || detail.ShowOutput == nil {
		return false
	}

	s := detail.ShowOutput

	var modified bool

	if reconciler.LateInitNonZero(&obj.Spec.Comment, s.Comment) {
		modified = true
	}

	if reconciler.LateInitNonZero(&obj.Spec.GitCredentials, s.GitCredentials) {
		modified = true
	}

	return modified
}

// Compile-time check that BaseAdapter implements LateInitializer.
var _ reconciler.LateInitializer[*snowplanev1alpha1.GitRepository, *snowflake.GitRepositoryObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.GitRepository, Service, *snowflake.GitRepositoryObservation])(nil)
