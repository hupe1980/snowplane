package imagerepository

import (
	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// lateInitialize fills nil spec fields from the observed Snowflake state.
// Only called during adoption (adoptionPolicy=adopt, first reconcile).
// Image repositories have no optional spec fields, so this is a no-op.
func lateInitialize(_ *snowplanev1alpha1.ImageRepository, _ *reconciler.Observation[*snowflake.ImageRepositoryObservation]) bool {
	return false
}

var _ reconciler.LateInitializer[*snowplanev1alpha1.ImageRepository, *snowflake.ImageRepositoryObservation] = (*reconciler.BaseAdapter[*snowplanev1alpha1.ImageRepository, Service, *snowflake.ImageRepositoryObservation])(nil)
