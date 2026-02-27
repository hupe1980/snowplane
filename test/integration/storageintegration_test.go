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

func TestStorageIntegration_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	storageIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
		if id.Name() == "MY_STI" && created.Load() {
			return storageIntegrationObservation("MY_STI"), nil
		}

		return &snowflake.StorageIntegrationObservation{Exists: false}, nil
	})

	storageIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStorageIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestStorageIntegration("test-sti", "MY_STI")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-sti", Namespace: testNamespace}

	t.Cleanup(func() {
		storageIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.StorageIntegration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StorageIntegration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StorageIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "storage integration should become Ready")

	var obj snowplanev1alpha1.StorageIntegration
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_STI", obj.Status.ShowOutput.Name)
}

func TestStorageIntegration_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	storageIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
		if id.Name() == "MY_STI_ORPHAN" && created.Load() {
			return storageIntegrationObservation("MY_STI_ORPHAN"), nil
		}

		return &snowflake.StorageIntegrationObservation{Exists: false}, nil
	})

	storageIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStorageIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestStorageIntegration("test-sti-orphan", "MY_STI_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-sti-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StorageIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	storageIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.StorageIntegration{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
