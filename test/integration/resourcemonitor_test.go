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

func TestResourceMonitor_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	resourceMonitorMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
		if id.Name() == "MY_RM" && created.Load() {
			return resourceMonitorObservation("MY_RM"), nil
		}

		return &snowflake.ResourceMonitorObservation{Exists: false}, nil
	})

	resourceMonitorMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateResourceMonitorOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestResourceMonitor("test-rm", "MY_RM")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-rm", Namespace: testNamespace}

	t.Cleanup(func() {
		resourceMonitorMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.ResourceMonitor
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ResourceMonitor{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ResourceMonitor
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "resource monitor should become Ready")

	var obj snowplanev1alpha1.ResourceMonitor
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_RM", obj.Status.ShowOutput.Name)
}

func TestResourceMonitor_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	resourceMonitorMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
		if id.Name() == "MY_RM_ORPHAN" && created.Load() {
			return resourceMonitorObservation("MY_RM_ORPHAN"), nil
		}

		return &snowflake.ResourceMonitorObservation{Exists: false}, nil
	})

	resourceMonitorMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateResourceMonitorOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestResourceMonitor("test-rm-orphan", "MY_RM_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-rm-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ResourceMonitor
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	resourceMonitorMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.ResourceMonitor{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
