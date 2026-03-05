// Package webhook provides Kubernetes admission webhook handlers for the
// Snowplane operator.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
)

// OwnershipValidator is a validating admission webhook handler that prevents
// ownership conflicts. It rejects CREATE/UPDATE requests when another Custom
// Resource (different UID) already manages the same Snowflake object.
//
// The handler computes the Snowflake fully-qualified name (FQN) from spec
// fields, resolving databaseRef/schemaRef via API reads when necessary, and
// checks for an existing CR that carries the same external-name-hash label
// stamped by the reconciler.
//
// If the FQN cannot be determined at admission time (e.g. the referenced CR
// does not exist yet), the webhook allows the request and defers conflict
// detection to the reconciler.
type OwnershipValidator struct {
	Client client.Client
}

// Compile-time check: OwnershipValidator implements admission.Handler.
var _ admission.Handler = &OwnershipValidator{}

// apiCallTimeout bounds the total time spent on Kubernetes API calls
// (ref resolution + label list) within a single admission request.
const apiCallTimeout = 5 * time.Second

// Handle processes an admission request and denies it when an ownership
// conflict is detected.
func (v *OwnershipValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Skip non-mutating operations.
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	// Skip sub-resource updates (e.g. /status, /scale) — spec hasn't changed.
	if req.SubResource != "" {
		return admission.Allowed("sub-resource update")
	}

	// Skip dry-run requests — avoid expensive API calls for simulated ops.
	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("dry-run")
	}

	logger := log.FromContext(ctx).WithValues(
		"webhook", "ownership",
		"operation", req.Operation,
		"namespace", req.Namespace,
		"name", req.Name,
	)

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(req.Object.Raw, &obj.Object); err != nil {
		logger.Error(err, "failed to unmarshal admission object")
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("unmarshal object: %w", err))
	}

	// Scope API calls with a timeout shorter than the webhook's own timeout.
	apiCtx, apiCancel := context.WithTimeout(ctx, apiCallTimeout)
	defer apiCancel()

	fqn, err := v.computeFQN(apiCtx, obj, req.Namespace)
	if err != nil {
		logger.V(1).Info("skipping ownership check: cannot compute FQN", "reason", err.Error())
		return admission.Allowed("cannot determine FQN at admission time; deferring to reconciler")
	}

	hash := reconciler.ComputeExternalNameHash(fqn)

	listGVK := obj.GroupVersionKind()
	listGVK.Kind += "List"

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(listGVK)

	if err := v.Client.List(apiCtx, list,
		client.MatchingLabels{snowplanev1alpha1.LabelExternalNameHash: hash},
	); err != nil {
		logger.Error(err, "failed to list CRs for ownership check")
		return admission.Allowed("list failed; deferring to reconciler")
	}

	incomingUID := obj.GetUID()

	for i := range list.Items {
		existing := &list.Items[i]

		if incomingUID == "" || existing.GetUID() != incomingUID {
			// Log full details for operators but redact cross-namespace CR
			// identity from the user-facing denial to prevent info disclosure.
			logger.Info("ownership conflict detected",
				"conflictingNamespace", existing.GetNamespace(),
				"conflictingName", existing.GetName(),
				"fqn", fqn,
			)

			msg := fmt.Sprintf(
				"ownership conflict: another %s already manages Snowflake resource %q",
				existing.GetKind(), fqn,
			)

			return admission.Denied(msg)
		}
	}

	return admission.Allowed("")
}

// computeFQN determines the Snowflake fully-qualified name from spec fields.
func (v *OwnershipValidator) computeFQN(
	ctx context.Context,
	obj *unstructured.Unstructured,
	namespace string,
) (string, error) {
	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("missing spec")
	}

	name, _ := spec["name"].(string)
	if name == "" {
		return "", fmt.Errorf("missing spec.name")
	}

	dbName, err := v.resolveDatabaseName(ctx, spec, namespace)
	if err != nil {
		return "", fmt.Errorf("resolve database: %w", err)
	}

	schemaName, err := v.resolveSchemaName(ctx, spec, namespace)
	if err != nil {
		return "", fmt.Errorf("resolve schema: %w", err)
	}

	switch {
	case dbName != "" && schemaName != "":
		return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, name).FullyQualifiedName(), nil
	case dbName != "":
		return snowflake.NewDatabaseObjectIdentifier(dbName, name).FullyQualifiedName(), nil
	default:
		return snowflake.NewAccountObjectIdentifier(name).FullyQualifiedName(), nil
	}
}

// resolveDatabaseName resolves the database name from spec.databaseName
// (inline) or spec.databaseRef (CR reference).
func (v *OwnershipValidator) resolveDatabaseName(
	ctx context.Context,
	spec map[string]interface{},
	namespace string,
) (string, error) {
	if dbName, ok := spec["databaseName"].(string); ok && dbName != "" {
		return dbName, nil
	}

	dbRef, ok := spec["databaseRef"].(map[string]interface{})
	if !ok {
		return "", nil
	}

	refName, _ := dbRef["name"].(string)
	if refName == "" {
		return "", fmt.Errorf("empty databaseRef.name")
	}

	refNS, _ := dbRef["namespace"].(string)
	if refNS == "" {
		refNS = namespace
	}

	db := &snowplanev1alpha1.Database{}
	if err := v.Client.Get(ctx, client.ObjectKey{Name: refName, Namespace: refNS}, db); err != nil {
		return "", fmt.Errorf("resolve databaseRef %s/%s: %w", refNS, refName, err)
	}

	return db.Spec.Name, nil
}

// resolveSchemaName resolves the schema name from spec.schemaName (inline) or
// spec.schemaRef (CR reference).
func (v *OwnershipValidator) resolveSchemaName(
	ctx context.Context,
	spec map[string]interface{},
	namespace string,
) (string, error) {
	if schemaName, ok := spec["schemaName"].(string); ok && schemaName != "" {
		return schemaName, nil
	}

	schemaRef, ok := spec["schemaRef"].(map[string]interface{})
	if !ok {
		return "", nil
	}

	refName, _ := schemaRef["name"].(string)
	if refName == "" {
		return "", fmt.Errorf("empty schemaRef.name")
	}

	refNS, _ := schemaRef["namespace"].(string)
	if refNS == "" {
		refNS = namespace
	}

	s := &snowplanev1alpha1.Schema{}
	if err := v.Client.Get(ctx, client.ObjectKey{Name: refName, Namespace: refNS}, s); err != nil {
		return "", fmt.Errorf("resolve schemaRef %s/%s: %w", refNS, refName, err)
	}

	return s.Spec.Name, nil
}
