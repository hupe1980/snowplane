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

func TestSecondaryDatabase_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool
	var refreshCalled atomic.Bool

	secondaryDatabaseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
		if id.Name() == "MY_SDB" && created.Load() {
			return secondaryDatabaseObservation("MY_SDB"), nil
		}

		return &snowflake.SecondaryDatabaseObservation{Exists: false}, nil
	})

	secondaryDatabaseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error {
		assert.Equal(t, "MY_SDB", opts.Name.Name())
		assert.Equal(t, "myorg.myaccount.mydb", opts.AsReplicaOf)
		created.Store(true)

		return nil
	})

	secondaryDatabaseMockSvc.SetRefresh(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		refreshCalled.Store(true)

		return nil
	})

	secondaryDatabaseMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterSecondaryDatabaseOptions) error {
		return nil
	})

	cr := newTestSecondaryDatabase("test-sdb", "MY_SDB")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-sdb", Namespace: testNamespace}

	t.Cleanup(func() {
		secondaryDatabaseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.SecondaryDatabase
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecondaryDatabase{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecondaryDatabase
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "secondary database should become Ready")

	var obj snowplanev1alpha1.SecondaryDatabase
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_SDB", obj.Status.ShowOutput.Name)
	require.Equal(t, "myorg.myaccount.mydb", obj.Status.ShowOutput.Origin)
	require.NotEmpty(t, obj.Status.FullyQualifiedName)

	// Verify refresh was called (always-refresh pattern).
	require.True(t, refreshCalled.Load(), "Refresh should be called on every reconcile")
}

func TestSecondaryDatabase_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	secondaryDatabaseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
		if id.Name() == "MY_SDB_ORPHAN" && created.Load() {
			return secondaryDatabaseObservation("MY_SDB_ORPHAN"), nil
		}

		return &snowflake.SecondaryDatabaseObservation{Exists: false}, nil
	})

	secondaryDatabaseMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSecondaryDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	secondaryDatabaseMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterSecondaryDatabaseOptions) error {
		return nil
	})

	secondaryDatabaseMockSvc.SetRefresh(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		return nil
	})

	cr := newTestSecondaryDatabase("test-sdb-orphan", "MY_SDB_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-sdb-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecondaryDatabase
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	secondaryDatabaseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecondaryDatabase{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
