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

func TestUser_CreateLifecycle(t *testing.T) {
	resetMocks()

	userK8s := "user-create-test"
	sfUser := "USER_CREATE_TEST"

	var created atomic.Bool

	userMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
		if created.Load() {
			return userObservation(sfUser, "", "USERADMIN"), nil
		}

		return &snowflake.UserObservation{Exists: false}, nil
	})

	userMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateUserOptions) error {
		assert.Equal(t, sfUser, opts.Name.Name())
		created.Store(true)

		return nil
	})

	user := newTestUser(userK8s, sfUser)
	require.NoError(t, k8sClient.Create(ctx, user))

	key := types.NamespacedName{Name: userK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "user should become Ready")

	var result snowplanev1alpha1.User
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE USER should have been called")
	assert.Equal(t, sfUser, result.Status.ShowOutput.Name)
	assert.Equal(t, "USERADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/user")

	// Cleanup.
	userMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "user should be cleaned up")
}

func TestUser_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	userK8s := "user-alter-test"
	sfUser := "USER_ALTER_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	userMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
		if created.Load() {
			obs := userObservation(sfUser, curComment.Load().(string), "USERADMIN")
			return obs, nil
		}

		return &snowflake.UserObservation{Exists: false}, nil
	})

	userMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateUserOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	userMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterUserOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	user := newTestUser(userK8s, sfUser)
	initComment := "initial user comment"
	user.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, user))

	key := types.NamespacedName{Name: userK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "user should become Ready initially")

	// Update the comment.
	var current snowplanev1alpha1.User
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated user comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated user comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup.
	userMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestUser_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	userK8s := "user-orphan-test"
	sfUser := "USER_ORPHAN_TEST"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	userMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
		if created.Load() {
			return userObservation(sfUser, "", "USERADMIN"), nil
		}

		return &snowflake.UserObservation{Exists: false}, nil
	})

	userMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateUserOptions) error {
		created.Store(true)
		return nil
	})

	userMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	user := newTestUser(userK8s, sfUser)
	user.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, user))

	key := types.NamespacedName{Name: userK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.User
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.User
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}
