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

func TestMaskingPolicy_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"mp-db", "MP_DB", "mp-schema", "MP_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	maskingPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
		if id.Name() == "MY_MP" && created.Load() {
			return maskingPolicyObservation("MY_MP", "MP_DB", "MP_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.MaskingPolicyObservation{Exists: false}, nil
	})

	maskingPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateMaskingPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestMaskingPolicy("test-mp", "MY_MP", "mp-db", "mp-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-mp", Namespace: testNamespace}

	t.Cleanup(func() {
		maskingPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.MaskingPolicy
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.MaskingPolicy{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.MaskingPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "masking policy should become Ready")

	var obj snowplanev1alpha1.MaskingPolicy
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_MP", obj.Status.ShowOutput.Name)
}

func TestMaskingPolicy_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"mp-orphan-db", "MP_ORPHAN_DB", "mp-orphan-schema", "MP_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	maskingPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
		if id.Name() == "MY_MP_ORPHAN" && created.Load() {
			return maskingPolicyObservation("MY_MP_ORPHAN", "MP_ORPHAN_DB", "MP_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.MaskingPolicyObservation{Exists: false}, nil
	})

	maskingPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateMaskingPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestMaskingPolicy("test-mp-orphan", "MY_MP_ORPHAN", "mp-orphan-db", "mp-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-mp-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.MaskingPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	maskingPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.MaskingPolicy{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
