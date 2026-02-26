package fieldexport

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/testutil"
)

func TestExtractSourceRef(t *testing.T) {
	fe := &snowplanev1alpha1.FieldExport{
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: "my-db",
				},
			},
		},
	}
	got := extractSourceRef(fe)
	assert.Equal(t, []string{"Database/my-db"}, got)
}

func TestExtractSourceRef_NonFieldExport_ReturnsNil(t *testing.T) {
	cm := &corev1.ConfigMap{}
	got := extractSourceRef(cm)
	assert.Nil(t, got)
}

func TestExtractTargetRef_ConfigMap(t *testing.T) {
	fe := &snowplanev1alpha1.FieldExport{
		Spec: snowplanev1alpha1.FieldExportSpec{
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "my-cm",
			},
		},
	}
	got := extractTargetRef(fe)
	assert.Equal(t, []string{"ConfigMap/my-cm"}, got)
}

func TestExtractTargetRef_Secret(t *testing.T) {
	fe := &snowplanev1alpha1.FieldExport{
		Spec: snowplanev1alpha1.FieldExportSpec{
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret,
				Name: "my-secret",
			},
		},
	}
	got := extractTargetRef(fe)
	assert.Equal(t, []string{"Secret/my-secret"}, got)
}

func TestSourceResourceTypes_Returns14Types(t *testing.T) {
	srcs := sourceResourceTypes()
	assert.Len(t, srcs, 14, "should return one typed object per managed Snowplane resource")
}

func TestMapSourceToFieldExports_MatchingSource(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "cm", Key: "name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexSourceRef, extractSourceRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}

	requests := rec.mapSourceToFieldExports(context.Background(), db)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "fe-1", Namespace: "default"},
	}, requests[0])
}

func TestMapSourceToFieldExports_NoMatch(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "other-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "cm", Key: "name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexSourceRef, extractSourceRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
	}

	requests := rec.mapSourceToFieldExports(context.Background(), db)
	assert.Empty(t, requests)
}

func TestMapSourceToFieldExports_MultipleFieldExports(t *testing.T) {
	scheme := testutil.TestScheme()

	fe1 := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Warehouse", Name: "my-wh"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "cm1", Key: "name",
			},
		},
	}

	fe2 := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-2", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Warehouse", Name: "my-wh"},
				Path:     ".status.showOutput.size",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret, Name: "sec1", Key: "size",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe1, fe2).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexSourceRef, extractSourceRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	wh := &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{Name: "my-wh", Namespace: "default"},
	}

	requests := rec.mapSourceToFieldExports(context.Background(), wh)
	assert.Len(t, requests, 2)
}

func TestMapTargetToFieldExports_MatchingConfigMap(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "my-cm", Key: "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexTargetRef, extractTargetRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"},
	}

	requests := rec.mapTargetToFieldExports(context.Background(), cm)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "fe-1", Namespace: "default"},
	}, requests[0])
}

func TestMapTargetToFieldExports_MatchingSecret(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret, Name: "my-secret", Key: "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexTargetRef, extractTargetRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
	}

	requests := rec.mapTargetToFieldExports(context.Background(), secret)
	require.Len(t, requests, 1)
	assert.Equal(t, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "fe-1", Namespace: "default"},
	}, requests[0])
}

func TestMapTargetToFieldExports_NoMatch(t *testing.T) {
	scheme := testutil.TestScheme()

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-1", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{Kind: "Database", Name: "my-db"},
				Path:     ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "my-cm", Key: "db-name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexTargetRef, extractTargetRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "other-cm", Namespace: "default"},
	}

	requests := rec.mapTargetToFieldExports(context.Background(), cm)
	assert.Empty(t, requests)
}

func TestManagedByFieldExportPredicate_Accept(t *testing.T) {
	pred := managedByFieldExportPredicate()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{managedByLabel: managedByValue},
		},
	}
	// NewPredicateFuncs sets Create/Update/Delete/Generic.
	// Test via Create since it's the most common trigger.
	assert.True(t, pred.Create(event.TypedCreateEvent[client.Object]{Object: cm}))
}

func TestManagedByFieldExportPredicate_Reject_NoLabel(t *testing.T) {
	pred := managedByFieldExportPredicate()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{}}
	assert.False(t, pred.Create(event.TypedCreateEvent[client.Object]{Object: cm}))
}

func TestManagedByFieldExportPredicate_Reject_WrongLabel(t *testing.T) {
	pred := managedByFieldExportPredicate()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{managedByLabel: "helm"},
		},
	}
	assert.False(t, pred.Create(event.TypedCreateEvent[client.Object]{Object: cm}))
}

func TestMapSourceToFieldExports_DifferentNamespace_Skipped(t *testing.T) {
	scheme := testutil.TestScheme()

	// FieldExport in "default" can only reference sources in "default".
	// A Database event from a different namespace should NOT match.
	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-ns-mismatch", Namespace: "default"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database", Name: "my-db",
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "cm", Key: "name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexSourceRef, extractSourceRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "other"},
	}

	requests := rec.mapSourceToFieldExports(context.Background(), db)
	assert.Empty(t, requests, "namespace mismatch: FE in 'default' but event from 'other'")
}

func TestMapSourceToFieldExports_SameNamespace_Match(t *testing.T) {
	scheme := testutil.TestScheme()

	// FieldExport in "app" references source Database — must be in "app" too.
	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{Name: "fe-same-match", Namespace: "app"},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database", Name: "my-db",
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap, Name: "cm", Key: "name",
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fe).
		WithIndex(&snowplanev1alpha1.FieldExport{}, indexSourceRef, extractSourceRef).
		Build()

	rec := &Reconciler{client: c, recorder: record.NewFakeRecorder(10)}

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "app"},
	}

	requests := rec.mapSourceToFieldExports(context.Background(), db)
	require.Len(t, requests, 1, "same-namespace FE should match")
	assert.Equal(t, "fe-same-match", requests[0].Name)
	assert.Equal(t, "app", requests[0].Namespace)
}

// Ensure typed objects used in tests satisfy client.Object.
var _ client.Object = (*snowplanev1alpha1.Database)(nil)
var _ client.Object = (*snowplanev1alpha1.Warehouse)(nil)
