package reconciler

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// snowplaneAnnotations lists the user-facing Snowplane annotations that should
// trigger immediate reconciliation when changed. Controller-internal annotations
// (creation-initiated, late-initialized) are excluded — they are set by the
// reconciler itself during a reconcile pass that already handles state changes.
//
// Note: adoption-policy, drift-policy, and use-create-or-alter were promoted
// to spec.managementPolicies fields. Spec changes bump .metadata.generation
// and are handled by GenerationChangedPredicate, so no annotation entry is needed.
var snowplaneAnnotations = []string{
	snowplanev1alpha1.AnnotationForceNew,
	snowplanev1alpha1.AnnotationAllowDangerousGrant,
	snowplanev1alpha1.AnnotationAbandonOnDelete,
}

// AnnotationChangedPredicate triggers reconciliation when any Snowplane-managed
// annotation value changes between old and new objects. Only the annotations in
// [snowplaneAnnotations] are compared — all other annotation changes are ignored.
//
// This complements [predicate.GenerationChangedPredicate]: spec changes bump
// .metadata.generation, but annotation changes do not. Without this predicate,
// annotation-driven features (force-new, allow-dangerous-grant, abandon-on-delete)
// would require waiting for the next periodic resync (up to 5 minutes).
//
// Design decision: Snowplane uses a *watched* annotation set rather than
// Crossplane's *ignored* annotation set. This is more restrictive (fewer
// spurious reconciles) and safer against annotation noise from GitOps tools.
type AnnotationChangedPredicate struct {
	predicate.Funcs
}

// Update returns true when a snowplane-managed annotation value differs between
// the old and new objects of the update event.
func (AnnotationChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldAnn := e.ObjectOld.GetAnnotations()
	newAnn := e.ObjectNew.GetAnnotations()

	for _, key := range snowplaneAnnotations {
		if oldAnn[key] != newAnn[key] {
			return true
		}
	}

	return false
}

// DesiredStateChanged returns a composite predicate that triggers reconciliation
// when the object's desired state has changed. It fires on:
//   - spec changes (via GenerationChangedPredicate — generation is bumped by the
//     API server on spec mutations)
//   - Snowplane annotation changes (via AnnotationChangedPredicate — annotations
//     like force-new, allow-dangerous-grant, abandon-on-delete do not bump generation)
//
// Status-only updates, label changes, and non-Snowplane annotation changes are
// filtered out, preventing self-triggering loops and noise from external tools.
func DesiredStateChanged() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		AnnotationChangedPredicate{},
	)
}
