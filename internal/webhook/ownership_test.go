package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func makeDB(name, namespace, specName string, uid types.UID, hashLabel string) *snowplanev1alpha1.Database {
	db := &snowplanev1alpha1.Database{
		TypeMeta: metav1.TypeMeta{
			APIVersion: snowplanev1alpha1.GroupVersion.String(),
			Kind:       "Database",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			Name: specName,
		},
	}
	if hashLabel != "" {
		db.Labels = map[string]string{
			snowplanev1alpha1.LabelExternalNameHash: hashLabel,
		}
	}
	return db
}

func makeSchema(name, namespace, specName string, uid types.UID, dbRef *snowplanev1alpha1.ObjectReference, dbName *string) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		TypeMeta: metav1.TypeMeta{
			APIVersion: snowplanev1alpha1.GroupVersion.String(),
			Kind:       "Schema",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name:         specName,
			DatabaseRef:  dbRef,
			DatabaseName: dbName,
		},
	}
}

func rawJSON(t *testing.T, obj interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(obj)
	require.NoError(t, err)
	return b
}

func buildRequest(t *testing.T, op admissionv1.Operation, obj interface{}, namespace string) admission.Request {
	t.Helper()
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
			Namespace: namespace,
			Object: runtime.RawExtension{
				Raw: rawJSON(t, obj),
			},
		},
	}
}

func TestOwnershipValidator_Create_AccountLevel_Allowed(t *testing.T) {
	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	v := &OwnershipValidator{Client: c}

	req := buildRequest(t, admissionv1.Create, makeDB("db-cr", "default", "MY_DB", "", ""), "default")
	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "expected allowed")
}

func TestOwnershipValidator_Create_AccountLevel_Conflict(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-existing", "default", "MY_DB", "uid-1111", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	incoming := makeDB("db-new", "default", "MY_DB", "", "")
	req := buildRequest(t, admissionv1.Create, incoming, "default")
	resp := v.Handle(context.Background(), req)

	assert.False(t, resp.Allowed, "expected denied")
	assert.Contains(t, resp.Result.Message, "ownership conflict")
	// Verify cross-namespace CR identity is NOT leaked in the denial message.
	assert.NotContains(t, resp.Result.Message, "db-existing")
	assert.Contains(t, resp.Result.Message, "another Database")
}

func TestOwnershipValidator_Update_SameUID_Allowed(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-cr", "default", "MY_DB", "uid-same", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	incoming := makeDB("db-cr", "default", "MY_DB", "uid-same", hash)
	req := buildRequest(t, admissionv1.Update, incoming, "default")
	resp := v.Handle(context.Background(), req)

	assert.True(t, resp.Allowed, "self-update should be allowed")
}

func TestOwnershipValidator_Update_DifferentUID_Conflict(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-first", "default", "MY_DB", "uid-first", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	incoming := makeDB("db-second", "default", "MY_DB", "uid-second", "")
	req := buildRequest(t, admissionv1.Update, incoming, "default")
	resp := v.Handle(context.Background(), req)

	assert.False(t, resp.Allowed, "expected denied")
	assert.Contains(t, resp.Result.Message, "ownership conflict")
}

func TestOwnershipValidator_Delete_Skipped(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	req := buildRequest(t, admissionv1.Delete, makeDB("db-cr", "default", "MY_DB", "uid-1", ""), "default")
	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed)
}

func TestOwnershipValidator_DryRun_Skipped(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-existing", "default", "MY_DB", "uid-1111", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	// Build a request that WOULD conflict, but is dry-run.
	incoming := makeDB("db-new", "default", "MY_DB", "", "")
	req := buildRequest(t, admissionv1.Create, incoming, "default")
	dryRun := true
	req.DryRun = &dryRun

	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "dry-run requests should always be allowed")
	assert.Contains(t, resp.Result.Message, "dry-run")
}

func TestOwnershipValidator_SubResource_Skipped(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-existing", "default", "MY_DB", "uid-1111", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	// Build a request that WOULD conflict, but is a sub-resource update.
	incoming := makeDB("db-existing", "default", "MY_DB", "uid-1111", hash)
	req := buildRequest(t, admissionv1.Update, incoming, "default")
	req.SubResource = "status"

	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "sub-resource updates should be skipped")
	assert.Contains(t, resp.Result.Message, "sub-resource")
}

func TestOwnershipValidator_DenialMessage_NoInfoLeak(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)

	// Existing CR is in a DIFFERENT namespace.
	existing := makeDB("db-existing", "other-team", "MY_DB", "uid-other", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	incoming := makeDB("db-new", "team-a", "MY_DB", "", "")
	req := buildRequest(t, admissionv1.Create, incoming, "team-a")
	resp := v.Handle(context.Background(), req)

	assert.False(t, resp.Allowed)
	// Denial message should NOT contain the other namespace or CR name.
	assert.NotContains(t, resp.Result.Message, "other-team")
	assert.NotContains(t, resp.Result.Message, "db-existing")
	// It SHOULD still contain the kind and the FQN.
	assert.Contains(t, resp.Result.Message, "Database")
	assert.Contains(t, resp.Result.Message, "MY_DB")
}

func TestOwnershipValidator_MissingSpecName_Allowed(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	raw := []byte(`{"apiVersion":"snowplane.hupe1980.github.io/v1alpha1","kind":"Database","metadata":{"name":"x","namespace":"default"},"spec":{}}`)
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Namespace: "default",
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "should allow when FQN cannot be determined")
}

func TestOwnershipValidator_UnresolvableRef_Allowed(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	obj := map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Schema",
		"metadata":   map[string]interface{}{"name": "my-schema", "namespace": "default"},
		"spec": map[string]interface{}{
			"name":        "MY_SCHEMA",
			"databaseRef": map[string]interface{}{"name": "nonexistent-db"},
		},
	}
	raw, err := json.Marshal(obj)
	require.NoError(t, err)

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Namespace: "default",
			Object:    runtime.RawExtension{Raw: raw},
		},
	}
	resp := v.Handle(context.Background(), req)
	assert.True(t, resp.Allowed, "should allow when ref cannot be resolved")
}

func TestOwnershipValidator_CrossNamespace_Conflict(t *testing.T) {
	scheme := testutil.TestScheme()
	hash := reconciler.ComputeExternalNameHash(`"MY_DB"`)
	existing := makeDB("db-ns-a", "namespace-a", "MY_DB", "uid-a", hash)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	v := &OwnershipValidator{Client: c}

	incoming := makeDB("db-ns-b", "namespace-b", "MY_DB", "", "")
	req := buildRequest(t, admissionv1.Create, incoming, "namespace-b")
	resp := v.Handle(context.Background(), req)

	assert.False(t, resp.Allowed, "cross-namespace conflict should be denied")
	assert.Contains(t, resp.Result.Message, "ownership conflict")
	// Redacted: must NOT contain the conflicting CR's namespace or name.
	assert.NotContains(t, resp.Result.Message, "db-ns-a")
	assert.NotContains(t, resp.Result.Message, "namespace-a")
	assert.Contains(t, resp.Result.Message, "another Database")
}

func TestComputeFQN_AccountLevel(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Database",
		"spec":       map[string]interface{}{"name": "MY_DB"},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"MY_DB"`, fqn)
}

func TestComputeFQN_DatabaseLevel_InlineName(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Schema",
		"spec": map[string]interface{}{
			"name":         "MY_SCHEMA",
			"databaseName": "MY_DB",
		},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"MY_DB"."MY_SCHEMA"`, fqn)
}

func TestComputeFQN_DatabaseLevel_Ref(t *testing.T) {
	scheme := testutil.TestScheme()
	dbCR := makeDB("my-db-cr", "default", "MY_DB", "uid-db", "")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dbCR).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Schema",
		"spec": map[string]interface{}{
			"name":        "MY_SCHEMA",
			"databaseRef": map[string]interface{}{"name": "my-db-cr"},
		},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"MY_DB"."MY_SCHEMA"`, fqn)
}

func TestComputeFQN_SchemaLevel_InlineNames(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Table",
		"spec": map[string]interface{}{
			"name":         "MY_TABLE",
			"databaseName": "MY_DB",
			"schemaName":   "MY_SCHEMA",
		},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"MY_DB"."MY_SCHEMA"."MY_TABLE"`, fqn)
}

func TestComputeFQN_SchemaLevel_Refs(t *testing.T) {
	scheme := testutil.TestScheme()
	dbCR := makeDB("my-db-cr", "default", "MY_DB", "uid-db", "")
	schemaCR := makeSchema("my-schema-cr", "default", "MY_SCHEMA", "uid-schema", nil, nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dbCR, schemaCR).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Table",
		"spec": map[string]interface{}{
			"name":        "MY_TABLE",
			"databaseRef": map[string]interface{}{"name": "my-db-cr"},
			"schemaRef":   map[string]interface{}{"name": "my-schema-cr"},
		},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"MY_DB"."MY_SCHEMA"."MY_TABLE"`, fqn)
}

func TestComputeFQN_UnresolvableRef_Error(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testutil.TestScheme()).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Schema",
		"spec": map[string]interface{}{
			"name":        "MY_SCHEMA",
			"databaseRef": map[string]interface{}{"name": "nonexistent-db"},
		},
	}}
	_, err := v.computeFQN(context.Background(), obj, "default")
	assert.Error(t, err)
}

func TestComputeFQN_CrossNamespaceRef(t *testing.T) {
	scheme := testutil.TestScheme()
	dbCR := makeDB("remote-db", "other-ns", "REMOTE_DB", "uid-remote", "")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dbCR).Build()
	v := &OwnershipValidator{Client: c}

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "snowplane.hupe1980.github.io/v1alpha1",
		"kind":       "Schema",
		"spec": map[string]interface{}{
			"name":        "MY_SCHEMA",
			"databaseRef": map[string]interface{}{"name": "remote-db", "namespace": "other-ns"},
		},
	}}
	fqn, err := v.computeFQN(context.Background(), obj, "default")
	require.NoError(t, err)
	assert.Equal(t, `"REMOTE_DB"."MY_SCHEMA"`, fqn)
}
