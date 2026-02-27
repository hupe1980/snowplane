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

func TestGrantOwnership_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	grantOwnershipMockSvc.SetObserve(func(_ context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
		if created.Load() {
			return grantOwnershipObservation("DATABASE", "MY_DB", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.GrantOwnershipObservation{Exists: false}, nil
	})

	grantOwnershipMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateGrantOwnershipOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestGrantOwnership("test-go", "DATABASE", "MY_DB", "SYSADMIN")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-go", Namespace: testNamespace}

	t.Cleanup(func() {
		grantOwnershipMockSvc.SetDrop(func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) error { return nil })

		var obj snowplanev1alpha1.GrantOwnership
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.GrantOwnership{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantOwnership
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "grant ownership should become Ready")

	var obj snowplanev1alpha1.GrantOwnership
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_DB", obj.Status.ShowOutput.Name)
	require.Equal(t, "SYSADMIN", obj.Status.ShowOutput.GranteeName)
}

func TestGrantOwnership_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	grantOwnershipMockSvc.SetObserve(func(_ context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
		if created.Load() {
			return grantOwnershipObservation("DATABASE", "MY_DB_ORPHAN", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.GrantOwnershipObservation{Exists: false}, nil
	})

	grantOwnershipMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateGrantOwnershipOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestGrantOwnership("test-go-orphan", "DATABASE", "MY_DB_ORPHAN", "SYSADMIN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-go-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantOwnership
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	grantOwnershipMockSvc.SetDrop(func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.GrantOwnership{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
