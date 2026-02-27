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

func TestRowAccessPolicy_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"rap-db", "RAP_DB", "rap-schema", "RAP_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	rowAccessPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
		if id.Name() == "MY_RAP" && created.Load() {
			return rowAccessPolicyObservation("MY_RAP", "RAP_DB", "RAP_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
	})

	rowAccessPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateRowAccessPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestRowAccessPolicy("test-rap", "MY_RAP", "rap-db", "rap-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-rap", Namespace: testNamespace}

	t.Cleanup(func() {
		rowAccessPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.RowAccessPolicy
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.RowAccessPolicy{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.RowAccessPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "row access policy should become Ready")

	var obj snowplanev1alpha1.RowAccessPolicy
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_RAP", obj.Status.ShowOutput.Name)
}

func TestRowAccessPolicy_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"rap-orphan-db", "RAP_ORPHAN_DB", "rap-orphan-schema", "RAP_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	rowAccessPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
		if id.Name() == "MY_RAP_ORPHAN" && created.Load() {
			return rowAccessPolicyObservation("MY_RAP_ORPHAN", "RAP_ORPHAN_DB", "RAP_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
	})

	rowAccessPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateRowAccessPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestRowAccessPolicy("test-rap-orphan", "MY_RAP_ORPHAN", "rap-orphan-db", "rap-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-rap-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.RowAccessPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	rowAccessPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.RowAccessPolicy{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
