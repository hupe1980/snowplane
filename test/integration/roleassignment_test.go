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

// ---------------------------------------------------------------------------
// AccountRoleAssignment
// ---------------------------------------------------------------------------

func TestAccountRoleAssignment_CreateLifecycle(t *testing.T) {
	resetMocks()

	var granted atomic.Bool

	roleAssignmentMockSvc.SetObserve(func(_ context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
		if granted.Load() {
			return roleAssignmentObservation("ANALYST", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.RoleAssignmentObservation{Exists: false}, nil
	})

	roleAssignmentMockSvc.SetGrantRole(func(_ context.Context, _ snowflake.GrantRoleOptions) error {
		granted.Store(true)

		return nil
	})

	cr := newTestAccountRoleAssignment("test-ara", "ANALYST", "SYSADMIN")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ara", Namespace: testNamespace}

	t.Cleanup(func() {
		roleAssignmentMockSvc.SetRevokeRole(func(_ context.Context, _ snowflake.RevokeRoleOptions) error { return nil })

		var obj snowplanev1alpha1.AccountRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.AccountRoleAssignment{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "account role assignment should become Ready")

	var obj snowplanev1alpha1.AccountRoleAssignment
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "ANALYST", obj.Status.ShowOutput.Role)
	require.Equal(t, "SYSADMIN", obj.Status.ShowOutput.GranteeName)
}

func TestAccountRoleAssignment_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var granted atomic.Bool

	roleAssignmentMockSvc.SetObserve(func(_ context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
		if granted.Load() {
			return roleAssignmentObservation("ANALYST_ORPHAN", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.RoleAssignmentObservation{Exists: false}, nil
	})

	roleAssignmentMockSvc.SetGrantRole(func(_ context.Context, _ snowflake.GrantRoleOptions) error {
		granted.Store(true)

		return nil
	})

	cr := newTestAccountRoleAssignment("test-ara-orphan", "ANALYST_ORPHAN", "SYSADMIN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ara-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var revokeCalled atomic.Bool

	roleAssignmentMockSvc.SetRevokeRole(func(_ context.Context, _ snowflake.RevokeRoleOptions) error {
		revokeCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.AccountRoleAssignment{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, revokeCalled.Load())
}

// ---------------------------------------------------------------------------
// DatabaseRoleAssignment
// ---------------------------------------------------------------------------

func TestDatabaseRoleAssignment_CreateLifecycle(t *testing.T) {
	resetMocks()

	var granted atomic.Bool

	roleAssignmentMockSvc.SetObserve(func(_ context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
		if granted.Load() {
			return roleAssignmentObservation("MY_DB.MY_ROLE", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.RoleAssignmentObservation{Exists: false}, nil
	})

	roleAssignmentMockSvc.SetGrantRole(func(_ context.Context, _ snowflake.GrantRoleOptions) error {
		granted.Store(true)

		return nil
	})

	cr := newTestDatabaseRoleAssignment("test-dra", "MY_DB.MY_ROLE", "SYSADMIN")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-dra", Namespace: testNamespace}

	t.Cleanup(func() {
		roleAssignmentMockSvc.SetRevokeRole(func(_ context.Context, _ snowflake.RevokeRoleOptions) error { return nil })

		var obj snowplanev1alpha1.DatabaseRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.DatabaseRoleAssignment{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database role assignment should become Ready")

	var obj snowplanev1alpha1.DatabaseRoleAssignment
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_DB.MY_ROLE", obj.Status.ShowOutput.Role)
	require.Equal(t, "SYSADMIN", obj.Status.ShowOutput.GranteeName)
}

func TestDatabaseRoleAssignment_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var granted atomic.Bool

	roleAssignmentMockSvc.SetObserve(func(_ context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
		if granted.Load() {
			return roleAssignmentObservation("MY_DB.MY_ROLE_ORPHAN", "ROLE", "SYSADMIN"), nil
		}

		return &snowflake.RoleAssignmentObservation{Exists: false}, nil
	})

	roleAssignmentMockSvc.SetGrantRole(func(_ context.Context, _ snowflake.GrantRoleOptions) error {
		granted.Store(true)

		return nil
	})

	cr := newTestDatabaseRoleAssignment("test-dra-orphan", "MY_DB.MY_ROLE_ORPHAN", "SYSADMIN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-dra-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRoleAssignment
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var revokeCalled atomic.Bool

	roleAssignmentMockSvc.SetRevokeRole(func(_ context.Context, _ snowflake.RevokeRoleOptions) error {
		revokeCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.DatabaseRoleAssignment{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, revokeCalled.Load())
}
