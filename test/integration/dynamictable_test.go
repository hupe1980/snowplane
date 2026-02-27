//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestDynamicTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"dt-db", "DT_DB", "dt-schema", "DT_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	dynamicTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
		if id.Name() == "MY_DT" && created.Load() {
			return dynamicTableObservation("MY_DT", "DT_DB", "DT_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.DynamicTableObservation{Exists: false}, nil
	})

	dynamicTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDynamicTableOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestDynamicTable("test-dt", "MY_DT", "dt-db", "dt-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-dt", Namespace: testNamespace}

	t.Cleanup(func() {
		dynamicTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.DynamicTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.DynamicTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DynamicTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "dynamic table should become Ready")

	var obj snowplanev1alpha1.DynamicTable
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_DT", obj.Status.ShowOutput.Name)
}

func TestDynamicTable_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"dt-orphan-db", "DT_ORPHAN_DB", "dt-orphan-schema", "DT_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	dynamicTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
		if id.Name() == "MY_DT_ORPHAN" && created.Load() {
			return dynamicTableObservation("MY_DT_ORPHAN", "DT_ORPHAN_DB", "DT_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.DynamicTableObservation{Exists: false}, nil
	})

	dynamicTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDynamicTableOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestDynamicTable("test-dt-orphan", "MY_DT_ORPHAN", "dt-orphan-db", "dt-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-dt-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DynamicTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	dynamicTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.DynamicTable{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
