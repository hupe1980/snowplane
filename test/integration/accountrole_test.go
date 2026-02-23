//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestAccountRole_CreateLifecycle(t *testing.T) {
	resetMocks()

	roleK8s := "role-create-test"
	sfRole := "ROLE_CREATE_TEST"

	var created atomic.Bool

	accountRoleMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
		if created.Load() {
			return accountRoleObservation(sfRole, "", "USERADMIN"), nil
		}

		return &snowflake.AccountRoleObservation{Exists: false}, nil
	})

	accountRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateAccountRoleOptions) error {
		assert.Equal(t, sfRole, opts.Name.Name())
		created.Store(true)

		return nil
	})

	role := newTestAccountRole(roleK8s, sfRole)
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "account role should become Ready")

	var result snowplanev1alpha1.AccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE ROLE should have been called")
	assert.Equal(t, sfRole, result.Status.ShowOutput.Name)
	assert.Equal(t, "USERADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/accountrole")

	// Cleanup.
	accountRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "role should be cleaned up")
}

func TestAccountRole_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	roleK8s := "role-alter-test"
	sfRole := "ROLE_ALTER_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	accountRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
		if created.Load() {
			return accountRoleObservation(sfRole, curComment.Load().(string), "USERADMIN"), nil
		}

		return &snowflake.AccountRoleObservation{Exists: false}, nil
	})

	accountRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateAccountRoleOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	accountRoleMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterAccountRoleOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	role := newTestAccountRole(roleK8s, sfRole)
	initComment := "initial role comment"
	role.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment.
	var current snowplanev1alpha1.AccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated role comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated role comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup.
	accountRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestAccountRole_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	roleK8s := "role-orphan-test"
	sfRole := "ROLE_ORPHAN_TEST"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	accountRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
		if created.Load() {
			return accountRoleObservation(sfRole, "", "USERADMIN"), nil
		}

		return &snowflake.AccountRoleObservation{Exists: false}, nil
	})

	accountRoleMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
		created.Store(true)
		return nil
	})

	accountRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	role := newTestAccountRole(roleK8s, sfRole)
	role.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.AccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestAccountRole_DriftDetection(t *testing.T) {
	resetMocks()

	roleK8s := "role-drift-test"
	sfRole := "ROLE_DRIFT_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("desired comment")

	accountRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
		if created.Load() {
			return accountRoleObservation(sfRole, curComment.Load().(string), "USERADMIN"), nil
		}

		return &snowflake.AccountRoleObservation{Exists: false}, nil
	})

	accountRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateAccountRoleOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	accountRoleMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterAccountRoleOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	role := newTestAccountRole(roleK8s, sfRole)
	comment := "desired comment"
	role.Spec.Comment = &comment
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Simulate external drift.
	curComment.Store("externally changed comment")

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "drift should trigger ALTER to correct the comment")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "desired comment"
	}, defaultTimeout, defaultInterval, "status should reflect corrected comment")

	// Cleanup.
	accountRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.AccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
