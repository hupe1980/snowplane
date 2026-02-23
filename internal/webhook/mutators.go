package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// DefaultsMutator injects defaults into Snowplane CRDs at admission time.
type DefaultsMutator struct {
	decoder admission.Decoder
}

// NewDefaultsMutator creates a new DefaultsMutator.
func NewDefaultsMutator(scheme *runtime.Scheme) *DefaultsMutator {
	return &DefaultsMutator{decoder: admission.NewDecoder(scheme)}
}

// Handle applies defaults to Snowplane CRDs based on the resource kind.
func (m *DefaultsMutator) Handle(_ context.Context, req admission.Request) admission.Response {
	switch req.Kind.Kind {
	case "Database":
		db := &snowplanev1alpha1.Database{}
		return m.mutate(req, db, &db.Spec.CommonSpec, "database")
	case "Schema":
		s := &snowplanev1alpha1.Schema{}
		return m.mutate(req, s, &s.Spec.CommonSpec, "schema")
	case "Warehouse":
		wh := &snowplanev1alpha1.Warehouse{}
		return m.mutate(req, wh, &wh.Spec.CommonSpec, "warehouse")
	case "AccountRole":
		role := &snowplanev1alpha1.AccountRole{}
		return m.mutate(req, role, &role.Spec.CommonSpec, "account role")
	case "DatabaseRole":
		role := &snowplanev1alpha1.DatabaseRole{}
		return m.mutate(req, role, &role.Spec.CommonSpec, "database role")
	case "User":
		u := &snowplanev1alpha1.User{}
		return m.mutate(req, u, &u.Spec.CommonSpec, "user", func() {
			if u.Spec.Type == nil {
				personType := snowplanev1alpha1.UserTypePerson
				u.Spec.Type = &personType
			}
		})
	case "AccountRoleGrant":
		grant := &snowplanev1alpha1.AccountRoleGrant{}
		return m.mutate(req, grant, &grant.Spec.CommonSpec, "accountrolegrant")
	case "DatabaseRoleGrant":
		grant := &snowplanev1alpha1.DatabaseRoleGrant{}
		return m.mutate(req, grant, &grant.Spec.CommonSpec, "databaserolegrant")
	case "ShareGrant":
		grant := &snowplanev1alpha1.ShareGrant{}
		return m.mutate(req, grant, &grant.Spec.CommonSpec, "sharegrant")
	case "Table":
		table := &snowplanev1alpha1.Table{}
		return m.mutate(req, table, &table.Spec.CommonSpec, "table")
	case "View":
		view := &snowplanev1alpha1.View{}
		return m.mutate(req, view, &view.Spec.CommonSpec, "view")
	case "Stage":
		stage := &snowplanev1alpha1.Stage{}
		return m.mutate(req, stage, &stage.Spec.CommonSpec, "stage")
	case "Task":
		task := &snowplanev1alpha1.Task{}
		return m.mutate(req, task, &task.Spec.CommonSpec, "task")
	case "Stream":
		stream := &snowplanev1alpha1.Stream{}
		return m.mutate(req, stream, &stream.Spec.CommonSpec, "stream")
	case "Tag":
		tag := &snowplanev1alpha1.Tag{}
		return m.mutate(req, tag, &tag.Spec.CommonSpec, "tag")
	case "NetworkPolicy":
		np := &snowplanev1alpha1.NetworkPolicy{}
		return m.mutate(req, np, &np.Spec.CommonSpec, "networkpolicy")
	case "ResourceMonitor":
		rm := &snowplanev1alpha1.ResourceMonitor{}
		return m.mutate(req, rm, &rm.Spec.CommonSpec, "resourcemonitor")
	case "MaskingPolicy":
		mp := &snowplanev1alpha1.MaskingPolicy{}
		return m.mutate(req, mp, &mp.Spec.CommonSpec, "maskingpolicy")
	case "RowAccessPolicy":
		rap := &snowplanev1alpha1.RowAccessPolicy{}
		return m.mutate(req, rap, &rap.Spec.CommonSpec, "rowaccesspolicy")
	case "GrantOwnership":
		gow := &snowplanev1alpha1.GrantOwnership{}
		return m.mutate(req, gow, &gow.Spec.CommonSpec, "grantownership")
	case "ProviderConfig":
		// ProviderConfig has no CommonSpec defaults to inject; allow as-is.
		return admission.Allowed("")
	default:
		return admission.Allowed("")
	}
}

// mutate decodes a CRD, applies CommonSpec defaults, runs optional post-default
// hooks, and returns a JSON patch response.
func (m *DefaultsMutator) mutate(
	req admission.Request,
	obj runtime.Object,
	common *snowplanev1alpha1.CommonSpec,
	label string,
	hooks ...func(),
) admission.Response {
	if err := m.decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decoding %s: %w", label, err))
	}

	applyCommonDefaults(common)

	for _, hook := range hooks {
		hook()
	}

	return patchResponse(req, obj)
}

// applyCommonDefaults sets common field defaults.
func applyCommonDefaults(spec *snowplanev1alpha1.CommonSpec) {
	if spec.DeletionPolicy == "" {
		spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyDelete
	}

	if spec.ProviderRef.Name == "" {
		spec.ProviderRef.Name = "default"
	}
}

// patchResponse creates a JSON patch response by comparing original and mutated objects.
func patchResponse(req admission.Request, mutated runtime.Object) admission.Response {
	marshaledMutated, err := json.Marshal(mutated)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("marshaling mutated object: %w", err))
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledMutated)
}
