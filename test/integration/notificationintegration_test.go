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

func TestNotificationIntegration_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	notificationIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
		if id.Name() == "MY_NI" && created.Load() {
			return notificationIntegrationObservation("MY_NI"), nil
		}

		return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
	})

	notificationIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateNotificationIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestNotificationIntegration("test-ni", "MY_NI")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ni", Namespace: testNamespace}

	t.Cleanup(func() {
		notificationIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.NotificationIntegration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.NotificationIntegration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NotificationIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "notification integration should become Ready")

	var obj snowplanev1alpha1.NotificationIntegration
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_NI", obj.Status.ShowOutput.Name)
}

func TestNotificationIntegration_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	notificationIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
		if id.Name() == "MY_NI_ORPHAN" && created.Load() {
			return notificationIntegrationObservation("MY_NI_ORPHAN"), nil
		}

		return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
	})

	notificationIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateNotificationIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestNotificationIntegration("test-ni-orphan", "MY_NI_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ni-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NotificationIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	notificationIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.NotificationIntegration{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
