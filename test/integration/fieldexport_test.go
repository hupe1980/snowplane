//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestFieldExport_ExportsToConfigMap(t *testing.T) {
	resetMocks()

	dbName := "fe-db-cm"
	sfDBName := "FE_DB_CM"

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "test comment", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database should become Ready")

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fe-cm-test",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: dbName,
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "fe-exports",
				Key:  "database-name",
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, fe))

	feKey := types.NamespacedName{Name: "fe-cm-test", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FieldExport
		if err := k8sClient.Get(ctx, feKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "fieldexport should become Ready")

	var cm corev1.ConfigMap
	cmKey := types.NamespacedName{Name: "fe-exports", Namespace: testNamespace}
	require.NoError(t, k8sClient.Get(ctx, cmKey, &cm))
	assert.Equal(t, sfDBName, cm.Data["database-name"])

	var result snowplanev1alpha1.FieldExport
	require.NoError(t, k8sClient.Get(ctx, feKey, &result))
	assert.NotEmpty(t, result.Status.LastExportedValueHash)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/fieldexport")

	_ = k8sClient.Delete(ctx, &result)

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, feKey, &snowplanev1alpha1.FieldExport{}) != nil
	}, defaultTimeout, defaultInterval)

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var d snowplanev1alpha1.Database
	if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
		_ = k8sClient.Delete(ctx, &d)

		require.Eventually(t, func() bool {
			return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
		}, defaultTimeout, defaultInterval)
	}
}

func TestFieldExport_SourceNotReady_WaitsForReady(t *testing.T) {
	resetMocks()

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	db := newTestDatabase("fe-db-wait", "FE_DB_WAIT")
	require.NoError(t, k8sClient.Create(ctx, db))

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fe-wait-test",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: "fe-db-wait",
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetConfigMap,
				Name: "fe-wait-cm",
				Key:  "name",
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, fe))

	feKey := types.NamespacedName{Name: "fe-wait-test", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FieldExport
		if err := k8sClient.Get(ctx, feKey, &obj); err != nil {
			return false
		}

		cond := conditions.Get(&obj, snowplanev1alpha1.TypeReady)

		return cond != nil && cond.Status == metav1.ConditionFalse &&
			cond.Reason == snowplanev1alpha1.ReasonDependencyNotReady
	}, defaultTimeout, defaultInterval, "fieldexport should have DependencyNotReady condition")

	_ = k8sClient.Delete(ctx, fe)

	dbKey := types.NamespacedName{Name: "fe-db-wait", Namespace: testNamespace}

	var d snowplanev1alpha1.Database
	if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
		_ = k8sClient.Delete(ctx, &d)
	}
}

func TestFieldExport_ExportsToSecret(t *testing.T) {
	resetMocks()

	dbName := "fe-db-secret"
	sfDBName := "FE_DB_SECRET"

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database should become Ready")

	fe := &snowplanev1alpha1.FieldExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fe-secret-test",
			Namespace: testNamespace,
		},
		Spec: snowplanev1alpha1.FieldExportSpec{
			From: snowplanev1alpha1.FieldExportSource{
				Resource: snowplanev1alpha1.FieldExportResourceRef{
					Kind: "Database",
					Name: dbName,
				},
				Path: ".status.showOutput.name",
			},
			To: snowplanev1alpha1.FieldExportTarget{
				Kind: snowplanev1alpha1.FieldExportTargetSecret,
				Name: "fe-secret-exports",
				Key:  "database-name",
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, fe))

	feKey := types.NamespacedName{Name: "fe-secret-test", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FieldExport
		if err := k8sClient.Get(ctx, feKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "fieldexport should become Ready")

	var secret corev1.Secret
	secretKey := types.NamespacedName{Name: "fe-secret-exports", Namespace: testNamespace}
	require.NoError(t, k8sClient.Get(ctx, secretKey, &secret))
	assert.Equal(t, sfDBName, string(secret.Data["database-name"]))

	_ = k8sClient.Delete(ctx, fe)

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var d snowplanev1alpha1.Database
	if err := k8sClient.Get(ctx, dbKey, &d); err == nil {
		_ = k8sClient.Delete(ctx, &d)

		require.Eventually(t, func() bool {
			return k8sClient.Get(ctx, dbKey, &snowplanev1alpha1.Database{}) != nil
		}, defaultTimeout, defaultInterval)
	}
}
