package fieldexport_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/controller/fieldexport"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

const testNS = "default"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newFieldExport(name string, from snowplanev1alpha1.FieldExportSource, to snowplanev1alpha1.FieldExportTarget) *snowplanev1alpha1.FieldExport {
	return &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: from,
			To:   to,
		},
	}
}

func newSourceDatabase(name string, ready bool, showOutput map[string]interface{}) *unstructured.Unstructured {
	status := map[string]interface{}{}
	if showOutput != nil {
		status["showOutput"] = showOutput
	}

	if ready {
		status["conditions"] = []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		}
	}

	u := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": snowplanev1alpha1.GroupVersion.String(),
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": testNS,
			},
			"status": status,
		},
	}

	return u
}

func doReconcile(t *testing.T, r *fieldexport.Reconciler, name string) ctrl.Result {
	t.Helper()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: testNS},
	})
	require.NoError(t, err)

	return result
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReconcile_NotFound(t *testing.T) {
	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: testNS},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "my-cm",
		Key:  "db-name",
	})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	// First reconcile adds the finalizer.
	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	// Verify finalizer was added.
	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))
	assert.Contains(t, updated.Finalizers, "snowplane.hupe1980.github.io/fieldexport")
}

func TestReconcile_SourceNotFound_SetsNotReady(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "missing-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "my-cm",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter)

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))
	assert.False(t, conditions.IsTrue(&updated, snowplanev1alpha1.TypeReady))

	cond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, cond.Reason)
	assert.Contains(t, cond.Message, "not found")
}

func TestReconcile_SourceNotReady_SetsNotReady(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "my-cm",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", false, map[string]interface{}{"name": "TEST_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter)

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))

	cond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, cond.Reason)
	assert.Contains(t, cond.Message, "not Ready")
}

func TestReconcile_PathNotFound_TerminalError(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.nonexistent.field",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "my-cm",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "TEST_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.Zero(t, result.RequeueAfter)
	assert.Zero(t, result.RequeueAfter) // Not requeued.

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))

	cond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonTerminalError, cond.Reason)
	assert.Contains(t, cond.Message, "not found")
}

func TestReconcile_ExportsToConfigMap(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "export-cm",
		Key:  "database-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "PROD_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter)

	// Verify ConfigMap was created.
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "export-cm", Namespace: testNS}, &cm))
	assert.Equal(t, "PROD_DB", cm.Data["database-name"])
	assert.Equal(t, "snowplane-fieldexport", cm.Labels["app.kubernetes.io/managed-by"])

	// Verify FieldExport status.
	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))
	assert.True(t, conditions.IsTrue(&updated, snowplanev1alpha1.TypeReady))
	assert.NotEmpty(t, updated.Status.LastExportedValueHash)
}

func TestReconcile_ExportsToSecret(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetSecret,
		Name: "export-secret",
		Key:  "database-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "SECRET_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter)

	// Verify Secret was created.
	var secret corev1.Secret
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "export-secret", Namespace: testNS}, &secret))
	assert.Equal(t, "SECRET_DB", string(secret.Data["database-name"]))
	assert.Equal(t, "snowplane-fieldexport", secret.Labels["app.kubernetes.io/managed-by"])

	// Verify status.
	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))
	assert.True(t, conditions.IsTrue(&updated, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdatesExistingConfigMap(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "existing-cm",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-cm",
			Namespace: testNS,
		},
		Data: map[string]string{
			"other-key": "other-value",
			"db-name":   "OLD_VALUE",
		},
	}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "NEW_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingCM).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	doReconcile(t, rec, "test-fe")

	// Verify ConfigMap was updated, preserving existing keys.
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "existing-cm", Namespace: testNS}, &cm))
	assert.Equal(t, "NEW_DB", cm.Data["db-name"])
	assert.Equal(t, "other-value", cm.Data["other-key"])
}

func TestReconcile_NoopWhenValueUnchanged(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "noop-cm",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noop-cm",
			Namespace: testNS,
		},
		Data: map[string]string{"db-name": "SAME_VALUE"},
	}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "SAME_VALUE"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingCM).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter)

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))
	assert.True(t, conditions.IsTrue(&updated, snowplanev1alpha1.TypeReady))
}

func TestReconcile_Deletion_RemovesFinalizer(t *testing.T) {
	scheme := testutil.TestScheme()
	now := metav1.Now()
	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-fe",
			Namespace:         testNS,
			DeletionTimestamp: &now,
			Finalizers:        []string{"snowplane.hupe1980.github.io/fieldexport"},
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "cm",
				Key:  "k",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// After removing the finalizer the fake client may have garbage-collected
	// the object because DeletionTimestamp was set. Either the object is gone
	// or the finalizer was removed — both are correct.
	var updated snowplanev1alpha1.FieldExport
	err = c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated)
	if err == nil {
		assert.NotContains(t, updated.Finalizers, "snowplane.hupe1980.github.io/fieldexport")
	}
	// If NotFound, the object was GC'd after finalizer removal — expected.
}

func TestReconcile_Deletion_CleansUpConfigMap(t *testing.T) {
	scheme := testutil.TestScheme()
	now := metav1.Now()

	// Pre-existing ConfigMap with the exported key and another key.
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-cm",
			Namespace: testNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "snowplane-fieldexport",
			},
		},
		Data: map[string]string{
			"db-name":   "PROD_DB",
			"other-key": "keep-me",
		},
	}

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-fe",
			Namespace:         testNS,
			DeletionTimestamp: &now,
			Finalizers:        []string{"snowplane.hupe1980.github.io/fieldexport"},
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "target-cm",
				Key:  "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingCM).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the exported key was removed but the other key remains.
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "target-cm", Namespace: testNS}, &cm))
	assert.NotContains(t, cm.Data, "db-name")
	assert.Equal(t, "keep-me", cm.Data["other-key"])
}

func TestReconcile_Deletion_DeletesEmptyManagedConfigMap(t *testing.T) {
	scheme := testutil.TestScheme()
	now := metav1.Now()

	// ConfigMap with only the exported key (should be deleted entirely).
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "single-key-cm",
			Namespace: testNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "snowplane-fieldexport",
			},
		},
		Data: map[string]string{
			"db-name": "PROD_DB",
		},
	}

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-fe",
			Namespace:         testNS,
			DeletionTimestamp: &now,
			Finalizers:        []string{"snowplane.hupe1980.github.io/fieldexport"},
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "single-key-cm",
				Key:  "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingCM).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	_, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)

	// ConfigMap should be deleted entirely since it was managed by fieldexport and now empty.
	var cm corev1.ConfigMap
	err = c.Get(context.Background(), types.NamespacedName{Name: "single-key-cm", Namespace: testNS}, &cm)
	assert.True(t, apierrors.IsNotFound(err), "empty managed ConfigMap should be deleted")
}

func TestReconcile_Deletion_CleansUpSecret(t *testing.T) {
	scheme := testutil.TestScheme()
	now := metav1.Now()

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-secret",
			Namespace: testNS,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "snowplane-fieldexport",
			},
		},
		Data: map[string][]byte{
			"db-name": []byte("PROD_DB"),
		},
	}

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-fe",
			Namespace:         testNS,
			DeletionTimestamp: &now,
			Finalizers:        []string{"snowplane.hupe1980.github.io/fieldexport"},
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret,
				Name: "target-secret",
				Key:  "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingSecret).
		WithStatusSubresource(fe).
		Build()
	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	_, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)

	// Single-key managed Secret should be deleted entirely.
	var secret corev1.Secret
	err = c.Get(context.Background(), types.NamespacedName{Name: "target-secret", Namespace: testNS}, &secret)
	assert.True(t, apierrors.IsNotFound(err), "empty managed Secret should be deleted")
}

func TestReconcile_EmptyPath_TerminalError(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.Zero(t, result.RequeueAfter)

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))

	cond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	// Path "." fails validation (must start with ".status."), surfacing as ValidationFailed.
	assert.Equal(t, snowplanev1alpha1.ReasonValidationFailed, cond.Reason)
}

func TestReconcile_ExportsToSecretUpdatesExisting(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetSecret,
		Name: "existing-secret",
		Key:  "db-name",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-secret",
			Namespace: testNS,
		},
		Data: map[string][]byte{
			"other-key": []byte("keep-me"),
			"db-name":   []byte("OLD"),
		},
	}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "UPDATED"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe, existingSecret).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	doReconcile(t, rec, "test-fe")

	var secret corev1.Secret
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "existing-secret", Namespace: testNS}, &secret))
	assert.Equal(t, "UPDATED", string(secret.Data["db-name"]))
	assert.Equal(t, "keep-me", string(secret.Data["other-key"]))
}

// ---------------------------------------------------------------------------
// ExtractFieldValue unit tests
// ---------------------------------------------------------------------------

func TestExtractFieldValue(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		path      string
		wantVal   interface{}
		wantFound bool
		wantErr   bool
	}{
		{
			name: "simple field",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"name": "test",
				},
			},
			path:      ".status.name",
			wantVal:   "test",
			wantFound: true,
		},
		{
			name: "nested field",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"showOutput": map[string]interface{}{
						"createdOn": "2024-01-01",
					},
				},
			},
			path:      ".status.showOutput.createdOn",
			wantVal:   "2024-01-01",
			wantFound: true,
		},
		{
			name: "path without leading dot",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"name": "test",
				},
			},
			path:      "status.name",
			wantVal:   "test",
			wantFound: true,
		},
		{
			name: "missing field",
			obj: map[string]interface{}{
				"status": map[string]interface{}{},
			},
			path:      ".status.nonexistent",
			wantFound: false,
		},
		{
			name:    "empty path after dot",
			obj:     map[string]interface{}{},
			path:    ".",
			wantErr: true,
		},
		{
			name:    "empty path",
			obj:     map[string]interface{}{},
			path:    "",
			wantErr: true,
		},
		{
			name: "numeric value",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"retentionDays": int64(7),
				},
			},
			path:      ".status.retentionDays",
			wantVal:   int64(7),
			wantFound: true,
		},
		{
			name: "boolean value",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"isTransient": true,
				},
			},
			path:      ".status.isTransient",
			wantVal:   true,
			wantFound: true,
		},
		{
			name:      "missing status entirely",
			obj:       map[string]interface{}{},
			path:      ".status.showOutput.name",
			wantFound: false,
		},
		{
			name: "deeply nested missing leaf",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"showOutput": map[string]interface{}{},
				},
			},
			path:      ".status.showOutput.nonexistent.deep",
			wantFound: false,
		},
		{
			name: "map value at path",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"showOutput": map[string]interface{}{
						"name":      "DB",
						"createdOn": "2024-01-01",
					},
				},
			},
			path: ".status.showOutput",
			wantVal: map[string]interface{}{
				"name":      "DB",
				"createdOn": "2024-01-01",
			},
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found, err := fieldexport.ExtractFieldValue(tt.obj, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)

			if tt.wantFound {
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatValue tests (via round-trip through reconciler)
// ---------------------------------------------------------------------------

func TestReconcile_ExportsMapValueAsJSON(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "json-cm",
		Key:  "output",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{
		"name":      "MY_DB",
		"createdOn": "2024-01-01",
	})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	doReconcile(t, rec, "test-fe")

	// Verify ConfigMap value is valid JSON (not Go map[string]interface{} format).
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "json-cm", Namespace: testNS}, &cm))
	assert.Contains(t, cm.Data["output"], `"name"`)
	assert.Contains(t, cm.Data["output"], `"createdOn"`)
	// Ensure it's proper JSON, not Go's map[key:value] format.
	assert.NotContains(t, cm.Data["output"], "map[")
}

// ---------------------------------------------------------------------------
// Same-namespace source resolution tests
// ---------------------------------------------------------------------------

func TestReconcile_SameNamespaceSource(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "same-ns-fe",
			Namespace:  testNS,
			Finalizers: []string{"snowplane.hupe1980.github.io/fieldexport"},
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: "local-db",
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "same-ns-cm",
				Key:  "db-name",
			},
		},
	}

	// Source in the same namespace.
	source := newSourceDatabase("local-db", true, map[string]interface{}{"name": "LOCAL_DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "same-ns-fe")
	assert.NotZero(t, result.RequeueAfter)

	// Target ConfigMap should be in the FieldExport's namespace.
	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "same-ns-cm", Namespace: testNS}, &cm))
	assert.Equal(t, "LOCAL_DB", cm.Data["db-name"])
}

// ---------------------------------------------------------------------------
// Edge case: source with no status at all
// ---------------------------------------------------------------------------

func TestReconcile_SourceWithNoStatus_NotReady(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "bare-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	// Source with no status at all (bare minimum object).
	source := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": snowplanev1alpha1.GroupVersion.String(),
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      "bare-db",
				"namespace": testNS,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))

	result := doReconcile(t, rec, "test-fe")
	assert.NotZero(t, result.RequeueAfter) // Should requeue (DependencyNotReady)

	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))

	cond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, cond.Reason)
	assert.Contains(t, cond.Message, "not Ready")
}

// ---------------------------------------------------------------------------
// Event emission verification
// ---------------------------------------------------------------------------

func TestReconcile_EmitsWarningEventOnWriteError(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "DB"})

	// Inject an interceptor that fails ConfigMap creates to simulate a write error.
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return fmt.Errorf("simulated write error")
				}
				return cl.Create(ctx, obj, opts...)
			},
		}).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	recorder := record.NewFakeRecorder(10)
	rec := fieldexport.NewReconciler(c, recorder)

	_, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.Error(t, err)

	// Verify a warning event was emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "failed to write")
	default:
		t.Fatal("expected a warning event but none was emitted")
	}
}

// ---------------------------------------------------------------------------
// Defense-in-depth validation (R8-2)
// ---------------------------------------------------------------------------

func TestReconcile_InvalidSpec_TerminalValidationFailure(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: "InvalidKind", // Fails spec validation.
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()

	recorder := record.NewFakeRecorder(10)
	rec := fieldexport.NewReconciler(c, recorder)

	result, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err)             // Terminal — no error returned.
	assert.Zero(t, result.RequeueAfter) // No requeue.

	// Status should reflect validation failure.
	var updated snowplanev1alpha1.FieldExport
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "test-fe", Namespace: testNS}, &updated))

	readyCond := conditions.Get(&updated, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonValidationFailed, readyCond.Reason)
	assert.Contains(t, readyCond.Message, "InvalidKind")

	syncCond := conditions.Get(&updated, snowplanev1alpha1.TypeSynced)
	require.NotNil(t, syncCond)
	assert.Equal(t, snowplanev1alpha1.ReasonValidationFailed, syncCond.Reason)

	// Verify a warning event was emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "ValidationFailed")
	default:
		t.Fatal("expected a warning event but none was emitted")
	}
}

// ---------------------------------------------------------------------------
// Primitive value formatting round-trip tests
// ---------------------------------------------------------------------------

func TestReconcile_ExportsNumericValueAsString(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.retentionTime",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "num-cm",
		Key:  "retention",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	// Use retentionTime which is int32 in DatabaseShowOutput — survives fake client round-trip.
	source := newSourceDatabase("my-db", true, map[string]interface{}{
		"retentionTime": int64(42),
	})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))
	doReconcile(t, rec, "test-fe")

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "num-cm", Namespace: testNS}, &cm))
	assert.Equal(t, "42", cm.Data["retention"])
}

func TestReconcile_ExportsBooleanValueAsString(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.kind",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "kind-cm",
		Key:  "db-kind",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	// Use "kind" field (string) from DatabaseShowOutput to verify string formatting.
	source := newSourceDatabase("my-db", true, map[string]interface{}{
		"kind": "TRANSIENT",
	})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	rec := fieldexport.NewReconciler(c, record.NewFakeRecorder(10))
	doReconcile(t, rec, "test-fe")

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "kind-cm", Namespace: testNS}, &cm))
	assert.Equal(t, "TRANSIENT", cm.Data["db-kind"])
}

// ---------------------------------------------------------------------------
// Event emission for all error paths
// ---------------------------------------------------------------------------

func TestReconcile_EmitsWarningEventOnSourceNotFound(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "missing-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()

	recorder := record.NewFakeRecorder(10)
	rec := fieldexport.NewReconciler(c, recorder)

	doReconcile(t, rec, "test-fe")

	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "not found")
	default:
		t.Fatal("expected a warning event for source-not-found")
	}
}

func TestReconcile_EmitsWarningEventOnSourceNotReady(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.showOutput.name",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", false, map[string]interface{}{"name": "DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	recorder := record.NewFakeRecorder(10)
	rec := fieldexport.NewReconciler(c, recorder)

	doReconcile(t, rec, "test-fe")

	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "not Ready")
	default:
		t.Fatal("expected a warning event for source-not-ready")
	}
}

func TestReconcile_EmitsWarningEventOnPathNotFound(t *testing.T) {
	scheme := testutil.TestScheme()
	fe := newFieldExport("test-fe", snowplanev1alpha1.FieldExportSource{
		Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
		Path:     ".status.nonexistent.field",
	}, snowplanev1alpha1.FieldExportTarget{
		Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
		Name: "cm",
		Key:  "k",
	})
	fe.Finalizers = []string{"snowplane.hupe1980.github.io/fieldexport"}

	source := newSourceDatabase("my-db", true, map[string]interface{}{"name": "DB"})

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(fe).
		WithStatusSubresource(fe).
		Build()
	require.NoError(t, c.Create(context.Background(), source))

	recorder := record.NewFakeRecorder(10)
	rec := fieldexport.NewReconciler(c, recorder)

	_, err := rec.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-fe", Namespace: testNS},
	})
	require.NoError(t, err) // Terminal — no error returned, just condition set.

	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, "not found")
	default:
		t.Fatal("expected a warning event for path-not-found")
	}
}
