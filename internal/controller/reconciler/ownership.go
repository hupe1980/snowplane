package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// externalNameHashLength is the number of hex characters used from the
// SHA-256 digest. 16 hex chars = 8 bytes = 2^64 combinations — more than
// sufficient to avoid label-value collisions across Snowflake FQNs within
// a single cluster.
const externalNameHashLength = 16

// ComputeExternalNameHash returns a truncated SHA-256 hex digest of the
// Snowflake fully-qualified name. The output is safe for use as a Kubernetes
// label value (alphanumeric, ≤63 chars).
func ComputeExternalNameHash(fqn string) string {
	h := sha256.Sum256([]byte(fqn))
	return hex.EncodeToString(h[:])[:externalNameHashLength]
}

// setExternalNameLabel stamps the external-name-hash label on the object's
// metadata. This label is used for fast same-cluster ownership lookups.
func setExternalNameLabel[T ManagedResource](obj T, hash string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[snowplanev1alpha1.LabelExternalNameHash] = hash
	obj.SetLabels(labels)
}

// checkOwnershipConflict lists all CRs of the same GVK that carry the same
// external-name-hash label. If any CR with a different UID is found, it means
// another CR in this cluster already manages the same Snowflake resource.
//
// Returns true when a conflict is detected (conditions and events are set on
// obj before returning). Returns false when no conflict exists.
func (r *GenericReconciler[T, S, D]) checkOwnershipConflict(ctx context.Context, obj T, hash string) (bool, error) {
	logger := log.FromContext(ctx)

	// Build the List GVK from the resource GVK.
	listGVK := schema.GroupVersionKind{
		Group:   r.GVK.Group,
		Version: r.GVK.Version,
		Kind:    r.GVK.Kind + "List",
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(listGVK)

	if err := r.Client.List(ctx, list, client.MatchingLabels{
		snowplanev1alpha1.LabelExternalNameHash: hash,
	}); err != nil {
		return false, fmt.Errorf("listing resources for ownership check: %w", err)
	}

	for _, item := range list.Items {
		if item.GetUID() != obj.GetUID() {
			resName := r.Adapter.ResourceName()
			msg := fmt.Sprintf(
				"ownership conflict: %s %s/%s (UID %s) already manages the same Snowflake resource; "+
					"remove the conflicting CR or change its spec to target a different resource",
				resName, item.GetNamespace(), item.GetName(), item.GetUID(),
			)
			logger.Info("adoption blocked by ownership conflict",
				"resource", resName,
				"conflicting_namespace", item.GetNamespace(),
				"conflicting_name", item.GetName(),
				"conflicting_uid", item.GetUID(),
			)

			conditions.SetNotReady(obj, snowplanev1alpha1.ReasonConflictDetected, msg)
			conditions.SetNotSynced(obj, snowplanev1alpha1.ReasonConflictDetected, msg)
			r.Recorder.Event(obj, corev1.EventTypeWarning, snowplanev1alpha1.ReasonConflictDetected, msg)
			metrics.RecordOwnershipConflict(resName)
			r.bestEffortPatchStatus(ctx, obj)

			return true, nil
		}
	}

	return false, nil
}
