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

func TestSharedDatabase_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	sharedDatabaseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
		if id.Name() == "MY_SHDB" && created.Load() {
			return sharedDatabaseObservation("MY_SHDB"), nil
		}

		return &snowflake.SharedDatabaseObservation{Exists: false}, nil
	})

	sharedDatabaseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateSharedDatabaseOptions) error {
		assert.Equal(t, "MY_SHDB", opts.Name.Name())
		assert.Equal(t, "provider_account.my_share", opts.FromShare)
		created.Store(true)

		return nil
	})

	cr := newTestSharedDatabase("test-shdb", "MY_SHDB")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-shdb", Namespace: testNamespace}

	t.Cleanup(func() {
		sharedDatabaseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.SharedDatabase
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SharedDatabase{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SharedDatabase
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "shared database should become Ready")

	var obj snowplanev1alpha1.SharedDatabase
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_SHDB", obj.Status.ShowOutput.Name)
	require.Equal(t, "provider_account.my_share", obj.Status.ShowOutput.Origin)
	require.NotEmpty(t, obj.Status.FullyQualifiedName)
}

func TestSharedDatabase_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	var created atomic.Bool
	var alterCalled atomic.Bool

	sharedDatabaseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
		if id.Name() == "MY_SHDB_ALTER" && created.Load() {
			return sharedDatabaseObservation("MY_SHDB_ALTER"), nil
		}

		return &snowflake.SharedDatabaseObservation{Exists: false}, nil
	})

	sharedDatabaseMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSharedDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	sharedDatabaseMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterSharedDatabaseOptions) error {
		alterCalled.Store(true)

		return nil
	})

	cr := newTestSharedDatabase("test-shdb-alter", "MY_SHDB_ALTER")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-shdb-alter", Namespace: testNamespace}

	t.Cleanup(func() {
		sharedDatabaseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.SharedDatabase
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SharedDatabase{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SharedDatabase
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment to trigger alter.
	var obj snowplanev1alpha1.SharedDatabase
	require.NoError(t, k8sClient.Get(ctx, key, &obj))

	newComment := "updated comment"
	obj.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &obj))

	require.Eventually(t, func() bool {
		return alterCalled.Load()
	}, defaultTimeout, defaultInterval, "alter should have been called")
}

func TestSharedDatabase_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	sharedDatabaseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
		if id.Name() == "MY_SHDB_ORPHAN" && created.Load() {
			return sharedDatabaseObservation("MY_SHDB_ORPHAN"), nil
		}

		return &snowflake.SharedDatabaseObservation{Exists: false}, nil
	})

	sharedDatabaseMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSharedDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestSharedDatabase("test-shdb-orphan", "MY_SHDB_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-shdb-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SharedDatabase
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	sharedDatabaseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.SharedDatabase{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
