// Package fieldexport implements the reconciler for FieldExport resources.
// Unlike other Snowplane controllers, FieldExport does not manage a Snowflake
// resource — it reads fields from managed resource statuses and writes them
// to ConfigMaps or Secrets.
package fieldexport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/metrics"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
	"github.com/hupe1980/snowplane/internal/utils/finalizers"
)

const (
	controllerName  = "fieldexport"
	requeueInterval = 2 * time.Minute
	requeueFast     = 15 * time.Second
	finalizerName   = "snowplane.hupe1980.github.io/fieldexport"
)

// Reconciler reconciles FieldExport resources.
type Reconciler struct {
	client   client.Client
	recorder record.EventRecorder
}

// NewReconciler creates a new FieldExport reconciler.
func NewReconciler(c client.Client, recorder record.EventRecorder) *Reconciler {
	return &Reconciler{client: c, recorder: recorder}
}

// Reconcile reads the source resource status, extracts the field at the
// specified path, and writes it to the target ConfigMap or Secret.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	start := time.Now()

	var fe snowplanev1alpha1.FieldExport
	if err := r.client.Get(ctx, req.NamespacedName, &fe); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		metrics.ReconcileDuration.WithLabelValues(controllerName).Observe(time.Since(start).Seconds())
		metrics.RecordReconcile(controllerName, retErr)
	}()

	// Handle deletion — clean up exported key, then remove finalizer.
	if !fe.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &fe)
	}

	// Ensure finalizer.
	if !finalizers.Has(&fe, finalizerName) {
		patchBase := fe.DeepCopy()
		finalizers.Add(&fe, finalizerName)

		if err := r.client.Patch(ctx, &fe, client.MergeFrom(patchBase)); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Defense-in-depth: validate spec even when webhooks are disabled (R8-2).
	if err := fe.Spec.Validate(); err != nil {
		conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		conditions.SetNotSynced(&fe, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonValidationFailed, err.Error())
		r.bestEffortPatchStatus(ctx, &fe)

		return ctrl.Result{}, nil // Terminal — do not requeue.
	}

	// Step 1: Resolve source resource.
	sourceNS := fe.Spec.From.Resource.Namespace
	if sourceNS == "" {
		sourceNS = fe.Namespace
	}

	source, err := r.fetchSourceResource(ctx, fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name, sourceNS)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("source resource %s/%s not found", fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name)
			conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonDependencyNotReady, msg)
			r.bestEffortPatchStatus(ctx, &fe)
			r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady, msg)

			return ctrl.Result{RequeueAfter: requeueFast}, nil
		}

		return ctrl.Result{}, fmt.Errorf("fetching source resource: %w", err)
	}

	// Step 2: Check source is Ready.
	if !isSourceReady(source) {
		msg := fmt.Sprintf("source %s/%s is not Ready", fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name)
		conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonDependencyNotReady, msg)
		r.bestEffortPatchStatus(ctx, &fe)
		r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonDependencyNotReady, msg)

		return ctrl.Result{RequeueAfter: requeueFast}, nil
	}

	// Step 3: Extract value at path.
	value, found, err := ExtractFieldValue(source.Object, fe.Spec.From.Path)
	if err != nil {
		conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonTerminalError,
			fmt.Sprintf("invalid path expression %q: %v", fe.Spec.From.Path, err))
		r.bestEffortPatchStatus(ctx, &fe)
		r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonTerminalError,
			fmt.Sprintf("path %q evaluation error: %v", fe.Spec.From.Path, err))

		return ctrl.Result{}, nil // Terminal — don't requeue.
	}

	if !found {
		msg := fmt.Sprintf("path %q not found in resource %s/%s", fe.Spec.From.Path, fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name)
		conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonTerminalError, msg)
		r.bestEffortPatchStatus(ctx, &fe)
		r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonTerminalError, msg)

		return ctrl.Result{}, nil // Terminal — don't requeue.
	}

	valueStr := formatValue(value)

	// Step 4: Write to target.
	targetNS := fe.Spec.To.Namespace
	if targetNS == "" {
		targetNS = fe.Namespace
	}

	if err := r.writeToTarget(ctx, fe.Spec.To, targetNS, valueStr); err != nil {
		conditions.SetNotReady(&fe, snowplanev1alpha1.ReasonReconcileError,
			fmt.Sprintf("writing to target %s/%s: %v", fe.Spec.To.Kind, fe.Spec.To.Name, err))
		r.bestEffortPatchStatus(ctx, &fe)
		r.recorder.Event(&fe, corev1.EventTypeWarning, snowplanev1alpha1.ReasonReconcileError,
			fmt.Sprintf("failed to write to %s/%s: %v", fe.Spec.To.Kind, fe.Spec.To.Name, err))

		return ctrl.Result{}, err
	}

	// Step 5: Update status.
	hash := sha256.Sum256([]byte(valueStr))
	newHash := hex.EncodeToString(hash[:])

	// Emit event only when the exported value actually changed.
	if fe.Status.LastExportedValueHash != newHash {
		r.recorder.Event(&fe, corev1.EventTypeNormal, snowplanev1alpha1.ReasonReconcileSuccess,
			fmt.Sprintf("exported %s/%s path=%q to %s/%s key=%q",
				fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name,
				fe.Spec.From.Path, fe.Spec.To.Kind, fe.Spec.To.Name, fe.Spec.To.Key))
	}

	fe.Status.LastExportedValueHash = newHash
	conditions.SetReady(&fe, fmt.Sprintf("exported to %s/%s key=%q", fe.Spec.To.Kind, fe.Spec.To.Name, fe.Spec.To.Key))

	if err := r.patchStatus(ctx, &fe); err != nil {
		return ctrl.Result{}, err
	}

	logger.V(1).Info("field exported successfully",
		"source", fmt.Sprintf("%s/%s", fe.Spec.From.Resource.Kind, fe.Spec.From.Resource.Name),
		"path", fe.Spec.From.Path,
		"target", fmt.Sprintf("%s/%s.%s", fe.Spec.To.Kind, fe.Spec.To.Name, fe.Spec.To.Key))

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// reconcileDelete cleans up the exported key from the target ConfigMap/Secret
// and removes the finalizer.
func (r *Reconciler) reconcileDelete(ctx context.Context, fe *snowplanev1alpha1.FieldExport) (ctrl.Result, error) {
	if finalizers.Has(fe, finalizerName) {
		targetNS := fe.Spec.To.Namespace
		if targetNS == "" {
			targetNS = fe.Namespace
		}

		if err := r.removeFromTarget(ctx, fe.Spec.To, targetNS); err != nil {
			// Log but don't block deletion — target may already be gone.
			log.FromContext(ctx).V(1).Info("failed to clean up target, proceeding with finalizer removal",
				"error", err, "target", fmt.Sprintf("%s/%s", fe.Spec.To.Kind, fe.Spec.To.Name))
		}

		r.recorder.Event(fe, corev1.EventTypeNormal, "FinalizerRemoved",
			fmt.Sprintf("cleaned up exported key %q from %s/%s", fe.Spec.To.Key, fe.Spec.To.Kind, fe.Spec.To.Name))

		patchBase := fe.DeepCopy()
		finalizers.Remove(fe, finalizerName)

		if err := r.client.Patch(ctx, fe, client.MergeFrom(patchBase)); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the FieldExport controller with the manager.
// GenerationChangedPredicate prevents status-only updates from triggering
// reconciliation, avoiding self-triggering loops when patchStatus updates
// .status.lastExportedValueHash (H-3 fix).
//
// Source resource watches (H-4): when a source Snowplane managed resource
// transitions to Ready or its status changes, FieldExports referencing it
// are immediately re-reconciled instead of waiting up to 2 minutes.
//
// Target watches (H-4): when a target ConfigMap/Secret managed by
// snowplane-fieldexport is deleted or modified, FieldExports targeting it
// are immediately re-reconciled to restore the exported value.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrent int) error {
	ctx := context.Background()

	// Index FieldExports by source resource for efficient lookup on source changes.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.FieldExport{},
		indexSourceRef, extractSourceRef,
	); err != nil {
		return fmt.Errorf("creating index %s: %w", indexSourceRef, err)
	}

	// Index FieldExports by target for efficient lookup on target changes.
	if err := mgr.GetFieldIndexer().IndexField(ctx, &snowplanev1alpha1.FieldExport{},
		indexTargetRef, extractTargetRef,
	); err != nil {
		return fmt.Errorf("creating index %s: %w", indexTargetRef, err)
	}

	bldr := ctrl.NewControllerManagedBy(mgr).
		For(&snowplanev1alpha1.FieldExport{},
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Watch target ConfigMaps managed by snowplane-fieldexport.
		Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapTargetToFieldExports),
			builder.WithPredicates(managedByFieldExportPredicate())).
		// Watch target Secrets managed by snowplane-fieldexport.
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapTargetToFieldExports),
			builder.WithPredicates(managedByFieldExportPredicate())).
		Named("fieldexport").
		WithOptions(controller.Options{MaxConcurrentReconciles: maxConcurrent})

	// Watch all Snowplane managed resource types as potential sources.
	// Status changes (e.g. Ready condition, showOutput) trigger re-reconciliation
	// of FieldExports that reference the changed resource, reducing propagation
	// latency from 2 minutes to near-instant.
	for _, src := range sourceResourceTypes() {
		bldr.Watches(src,
			handler.EnqueueRequestsFromMapFunc(r.mapSourceToFieldExports))
	}

	return bldr.Complete(r)
}

// fetchSourceResource loads the source Snowplane managed resource as unstructured.
func (r *Reconciler) fetchSourceResource(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	gvk := schema.GroupVersionKind{
		Group:   snowplanev1alpha1.GroupVersion.Group,
		Version: snowplanev1alpha1.GroupVersion.Version,
		Kind:    kind,
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	if err := r.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, u); err != nil {
		return nil, err
	}

	return u, nil
}

// isSourceReady checks if the source resource has Ready=True.
func isSourceReady(u *unstructured.Unstructured) bool {
	conditionsRaw, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditionsRaw {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		if condType, _ := cond["type"].(string); condType == "Ready" {
			status, _ := cond["status"].(string)
			return status == string(metav1.ConditionTrue)
		}
	}

	return false
}

// ExtractFieldValue extracts a value from the unstructured object using a
// dot-notation path. The path starts with a dot, e.g. ".status.showOutput.name".
//
// Supported path syntax: dot-separated field names (e.g. ".status.showOutput.name").
// Array indexing (e.g. ".status.conditions[0].message") is not supported;
// use direct field names for scalar values in status.
func ExtractFieldValue(obj map[string]interface{}, path string) (interface{}, bool, error) {
	// Normalize: strip leading dot, split on dots.
	p := strings.TrimPrefix(path, ".")
	if p == "" {
		return nil, false, fmt.Errorf("empty path")
	}

	parts := strings.Split(p, ".")
	if len(parts) == 0 {
		return nil, false, fmt.Errorf("empty path after split")
	}

	val, found, err := unstructured.NestedFieldNoCopy(obj, parts...)
	if err != nil {
		return nil, false, err
	}

	return val, found, nil
}

// formatValue converts the extracted value to a string representation suitable
// for storing in ConfigMaps/Secrets. Primitive types use simple formatting;
// complex types (maps, slices) are JSON-encoded for fidelity.
func formatValue(v interface{}) string {
	if v == nil {
		return ""
	}

	switch v.(type) {
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}

		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// writeToTarget writes the value to the target ConfigMap or Secret.
// Creates the target if it doesn't exist.
func (r *Reconciler) writeToTarget(ctx context.Context, target snowplanev1alpha1.FieldExportTarget, namespace, value string) error {
	switch target.Kind {
	case snowplanev1alpha1.FieldExportTargetConfigMap:
		return r.writeToConfigMap(ctx, target.Name, namespace, target.Key, value)
	case snowplanev1alpha1.FieldExportTargetSecret:
		return r.writeToSecret(ctx, target.Name, namespace, target.Key, value)
	default:
		return fmt.Errorf("unsupported target kind: %q", target.Kind)
	}
}

func (r *Reconciler) writeToConfigMap(ctx context.Context, name, namespace, key, value string) error {
	var cm corev1.ConfigMap
	nn := types.NamespacedName{Name: name, Namespace: namespace}

	err := r.client.Get(ctx, nn, &cm)
	if apierrors.IsNotFound(err) {
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "snowplane-fieldexport",
				},
			},
			Data: map[string]string{key: value},
		}

		if err := r.client.Create(ctx, &cm); err != nil {
			return fmt.Errorf("creating ConfigMap %s/%s: %w", namespace, name, err)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("getting ConfigMap %s/%s: %w", namespace, name, err)
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	if cm.Data[key] == value {
		return nil // No change needed.
	}

	patchBase := cm.DeepCopy()
	cm.Data[key] = value

	if err := r.client.Patch(ctx, &cm, client.MergeFrom(patchBase)); err != nil {
		return fmt.Errorf("patching ConfigMap %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (r *Reconciler) writeToSecret(ctx context.Context, name, namespace, key, value string) error {
	var secret corev1.Secret
	nn := types.NamespacedName{Name: name, Namespace: namespace}

	err := r.client.Get(ctx, nn, &secret)
	if apierrors.IsNotFound(err) {
		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "snowplane-fieldexport",
				},
			},
			Data: map[string][]byte{key: []byte(value)},
		}

		if err := r.client.Create(ctx, &secret); err != nil {
			return fmt.Errorf("creating Secret %s/%s: %w", namespace, name, err)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("getting Secret %s/%s: %w", namespace, name, err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	if string(secret.Data[key]) == value {
		return nil // No change needed.
	}

	patchBase := secret.DeepCopy()
	secret.Data[key] = []byte(value)

	if err := r.client.Patch(ctx, &secret, client.MergeFrom(patchBase)); err != nil {
		return fmt.Errorf("patching Secret %s/%s: %w", namespace, name, err)
	}

	return nil
}

// removeFromTarget removes the exported key from the target ConfigMap or Secret.
// This is called during deletion to clean up after the FieldExport.
func (r *Reconciler) removeFromTarget(ctx context.Context, target snowplanev1alpha1.FieldExportTarget, namespace string) error {
	switch target.Kind {
	case snowplanev1alpha1.FieldExportTargetConfigMap:
		return r.removeFromConfigMap(ctx, target.Name, namespace, target.Key)
	case snowplanev1alpha1.FieldExportTargetSecret:
		return r.removeFromSecret(ctx, target.Name, namespace, target.Key)
	default:
		return nil
	}
}

func (r *Reconciler) removeFromConfigMap(ctx context.Context, name, namespace, key string) error {
	var cm corev1.ConfigMap
	nn := types.NamespacedName{Name: name, Namespace: namespace}

	if err := r.client.Get(ctx, nn, &cm); err != nil {
		return client.IgnoreNotFound(err)
	}

	if _, exists := cm.Data[key]; !exists {
		return nil
	}

	patchBase := cm.DeepCopy()
	delete(cm.Data, key)

	// If we created and now emptied the ConfigMap, delete it entirely.
	if len(cm.Data) == 0 && cm.Labels["app.kubernetes.io/managed-by"] == "snowplane-fieldexport" {
		return r.client.Delete(ctx, &cm)
	}

	return r.client.Patch(ctx, &cm, client.MergeFrom(patchBase))
}

func (r *Reconciler) removeFromSecret(ctx context.Context, name, namespace, key string) error {
	var secret corev1.Secret
	nn := types.NamespacedName{Name: name, Namespace: namespace}

	if err := r.client.Get(ctx, nn, &secret); err != nil {
		return client.IgnoreNotFound(err)
	}

	if _, exists := secret.Data[key]; !exists {
		return nil
	}

	patchBase := secret.DeepCopy()
	delete(secret.Data, key)

	// If we created and now emptied the Secret, delete it entirely.
	if len(secret.Data) == 0 && secret.Labels["app.kubernetes.io/managed-by"] == "snowplane-fieldexport" {
		return r.client.Delete(ctx, &secret)
	}

	return r.client.Patch(ctx, &secret, client.MergeFrom(patchBase))
}

// patchStatus uses Server-Side Apply (SSA) to update the status subresource.
// SSA eliminates the need for ResourceVersion-based conflict detection —
// the server resolves ownership via managedFields instead (B-2).
func (r *Reconciler) patchStatus(ctx context.Context, fe *snowplanev1alpha1.FieldExport) error {
	// SSA requires TypeMeta in the patch payload.
	fe.SetGroupVersionKind(snowplanev1alpha1.GroupVersion.WithKind("FieldExport"))

	// SSA patch objects must not contain managedFields.
	fe.SetManagedFields(nil)

	return r.client.Status().Patch(ctx, fe, client.Apply, //nolint:staticcheck // TODO: migrate to client.Client.SubResource().Apply()
		client.FieldOwner("snowplane-controller"),
		client.ForceOwnership,
	)
}

// bestEffortPatchStatus patches status and logs any error without
// propagating it. Used when the primary reconcile result (requeue timing
// or a different error) must not be defeated by a status-patch failure (FE-1).
func (r *Reconciler) bestEffortPatchStatus(ctx context.Context, fe *snowplanev1alpha1.FieldExport) {
	if err := r.patchStatus(ctx, fe); err != nil {
		log.FromContext(ctx).Error(err, "best-effort status patch failed",
			"fieldExport", fe.Name, "namespace", fe.Namespace)
	}
}

// ---------------------------------------------------------------------------
// Watch infrastructure (H-4)
// ---------------------------------------------------------------------------

// Field index keys for FieldExport cross-resource watches.
const (
	// indexSourceRef indexes FieldExports by source resource "Kind/Name".
	indexSourceRef = ".spec.from.resource.ref"

	// indexTargetRef indexes FieldExports by target "Kind/Name".
	indexTargetRef = ".spec.to.ref"

	// managedByLabel is the label applied to ConfigMaps/Secrets created by
	// the FieldExport controller.
	managedByLabel = "app.kubernetes.io/managed-by"

	// managedByValue is the value of the managed-by label.
	managedByValue = "snowplane-fieldexport"
)

// extractSourceRef returns the composite index key "Kind/Name" for a FieldExport's
// source resource.
func extractSourceRef(obj client.Object) []string {
	fe, ok := obj.(*snowplanev1alpha1.FieldExport)
	if !ok {
		return nil
	}

	return []string{fe.Spec.From.Resource.Kind + "/" + fe.Spec.From.Resource.Name}
}

// extractTargetRef returns the composite index key "Kind/Name" for a FieldExport's
// target resource.
func extractTargetRef(obj client.Object) []string {
	fe, ok := obj.(*snowplanev1alpha1.FieldExport)
	if !ok {
		return nil
	}

	return []string{string(fe.Spec.To.Kind) + "/" + fe.Spec.To.Name}
}

// sourceResourceTypes returns all typed Snowplane managed resource objects
// that FieldExport can reference as sources.
func sourceResourceTypes() []client.Object {
	return []client.Object{
		&snowplanev1alpha1.Database{},
		&snowplanev1alpha1.Schema{},
		&snowplanev1alpha1.Warehouse{},
		&snowplanev1alpha1.AccountRole{},
		&snowplanev1alpha1.DatabaseRole{},
		&snowplanev1alpha1.User{},
		&snowplanev1alpha1.AccountRoleGrant{},
		&snowplanev1alpha1.DatabaseRoleGrant{},
		&snowplanev1alpha1.ShareGrant{},
		&snowplanev1alpha1.Table{},
		&snowplanev1alpha1.View{},
		&snowplanev1alpha1.Stage{},
	}
}

// mapSourceToFieldExports maps a changed Snowplane managed resource to all
// FieldExports that reference it. This enables near-instant propagation of
// source status changes (e.g. becoming Ready) to FieldExports.
func (r *Reconciler) mapSourceToFieldExports(ctx context.Context, obj client.Object) []reconcile.Request {
	kind := obj.GetObjectKind().GroupVersionKind().Kind

	// If the kind is empty (typed objects in informer cache), resolve from scheme.
	if kind == "" {
		gvks, _, err := r.client.Scheme().ObjectKinds(obj)
		if err != nil {
			log.FromContext(ctx).Error(err, "resolving ObjectKinds for source watch",
				"object", client.ObjectKeyFromObject(obj))
			return nil
		}

		if len(gvks) > 0 {
			kind = gvks[0].Kind
		}
	}

	if kind == "" {
		return nil
	}

	indexKey := kind + "/" + obj.GetName()

	var feList snowplanev1alpha1.FieldExportList
	if err := r.client.List(ctx, &feList,
		client.MatchingFields{indexSourceRef: indexKey},
	); err != nil {
		log.FromContext(ctx).Error(err, "listing FieldExports for source watch",
			"kind", kind, "name", obj.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(feList.Items))

	for i := range feList.Items {
		fe := &feList.Items[i]

		// Only match if the source namespace resolves to this object's namespace.
		sourceNS := fe.Spec.From.Resource.Namespace
		if sourceNS == "" {
			sourceNS = fe.Namespace
		}

		if sourceNS != obj.GetNamespace() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      fe.Name,
				Namespace: fe.Namespace,
			},
		})
	}

	return requests
}

// mapTargetToFieldExports maps a changed ConfigMap or Secret to all
// FieldExports that target it. This enables immediate re-reconciliation
// when a target is accidentally deleted or modified.
func (r *Reconciler) mapTargetToFieldExports(ctx context.Context, obj client.Object) []reconcile.Request {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	if kind == "" {
		gvks, _, err := r.client.Scheme().ObjectKinds(obj)
		if err != nil {
			log.FromContext(ctx).Error(err, "resolving ObjectKinds for target watch",
				"object", client.ObjectKeyFromObject(obj))
			return nil
		}

		if len(gvks) > 0 {
			kind = gvks[0].Kind
		}
	}

	if kind == "" {
		return nil
	}

	indexKey := kind + "/" + obj.GetName()

	var feList snowplanev1alpha1.FieldExportList
	if err := r.client.List(ctx, &feList,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingFields{indexTargetRef: indexKey},
	); err != nil {
		log.FromContext(ctx).Error(err, "listing FieldExports for target watch",
			"kind", kind, "name", obj.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(feList.Items))

	for i := range feList.Items {
		fe := &feList.Items[i]

		targetNS := fe.Spec.To.Namespace
		if targetNS == "" {
			targetNS = fe.Namespace
		}

		if targetNS != obj.GetNamespace() {
			continue
		}

		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      fe.Name,
				Namespace: fe.Namespace,
			},
		})
	}

	return requests
}

// managedByFieldExportPredicate returns a predicate that only accepts
// ConfigMaps/Secrets with the managed-by label set to "snowplane-fieldexport".
// This drastically reduces watch noise from unrelated ConfigMaps/Secrets.
func managedByFieldExportPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		labels := obj.GetLabels()
		return labels != nil && labels[managedByLabel] == managedByValue
	})
}
