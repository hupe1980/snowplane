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

func TestNetworkPolicy_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	networkPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
		if id.Name() == "MY_NP" && created.Load() {
			return networkPolicyObservation("MY_NP"), nil
		}

		return &snowflake.NetworkPolicyObservation{Exists: false}, nil
	})

	networkPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestNetworkPolicy("test-np", "MY_NP")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-np", Namespace: testNamespace}

	t.Cleanup(func() {
		networkPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.NetworkPolicy
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.NetworkPolicy{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NetworkPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "network policy should become Ready")

	var obj snowplanev1alpha1.NetworkPolicy
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_NP", obj.Status.ShowOutput.Name)
}

func TestNetworkPolicy_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	networkPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
		if id.Name() == "MY_NP_ORPHAN" && created.Load() {
			return networkPolicyObservation("MY_NP_ORPHAN"), nil
		}

		return &snowflake.NetworkPolicyObservation{Exists: false}, nil
	})

	networkPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestNetworkPolicy("test-np-orphan", "MY_NP_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-np-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NetworkPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	networkPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.NetworkPolicy{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
