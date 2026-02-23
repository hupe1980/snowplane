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

func TestDatabaseRole_CreateLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "dbr-parent-db"
	sfDB := "DBR_PARENT_DB"

	_, cleanupDB := setupReadyDatabase(t, dbK8s, sfDB)
	defer cleanupDB()

	roleK8s := "dbrole-create-test"
	sfRole := "DBROLE_CREATE_TEST"

	var created atomic.Bool

	databaseRoleMockSvc.SetObserve(func(_ context.Context, id snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
		if created.Load() {
			return databaseRoleObservation(sfRole, sfDB, "", "USERADMIN"), nil
		}

		return &snowflake.DatabaseRoleObservation{Exists: false}, nil
	})

	databaseRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
		assert.Equal(t, sfRole, opts.Name.Name())
		created.Store(true)

		return nil
	})

	role := newTestDatabaseRole(roleK8s, sfRole, dbK8s)
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "database role should become Ready")

	var result snowplanev1alpha1.DatabaseRole
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE DATABASE ROLE should have been called")
	assert.Equal(t, sfRole, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.ShowOutput.DatabaseName)
	assert.Equal(t, "USERADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/databaserole")

	// Cleanup.
	databaseRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "database role should be cleaned up")
}

func TestDatabaseRole_WaitForDatabaseReady(t *testing.T) {
	resetMocks()

	roleK8s := "dbrole-wait-for-db"
	sfRole := "DBROLE_WAIT_FOR_DB"

	var created atomic.Bool

	databaseRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
		if created.Load() {
			return databaseRoleObservation(sfRole, "WAIT_DB", "", "USERADMIN"), nil
		}

		return &snowflake.DatabaseRoleObservation{Exists: false}, nil
	})

	databaseRoleMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseRoleOptions) error {
		created.Store(true)
		return nil
	})

	role := newTestDatabaseRole(roleK8s, sfRole, "wait-parent-db")
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	// The role should NOT become Ready while the parent Database does not exist.
	require.Never(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, neverDuration, defaultInterval, "database role should not become Ready without parent Database")

	// Now create the parent Database.
	_, cleanupDB := setupReadyDatabase(t, "wait-parent-db", "WAIT_DB")
	defer cleanupDB()

	// The role should now become Ready.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database role should become Ready after parent Database is ready")

	// Cleanup.
	databaseRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.DatabaseRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabaseRole_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	dbK8s := "dbr-alter-parent"
	sfDB := "DBR_ALTER_PARENT"

	_, cleanupDB := setupReadyDatabase(t, dbK8s, sfDB)
	defer cleanupDB()

	roleK8s := "dbrole-alter-test"
	sfRole := "DBROLE_ALTER_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	databaseRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
		if created.Load() {
			return databaseRoleObservation(sfRole, sfDB, curComment.Load().(string), "USERADMIN"), nil
		}

		return &snowflake.DatabaseRoleObservation{Exists: false}, nil
	})

	databaseRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	databaseRoleMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterDatabaseRoleOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	role := newTestDatabaseRole(roleK8s, sfRole, dbK8s)
	initComment := "initial comment"
	role.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment.
	var current snowplanev1alpha1.DatabaseRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup.
	databaseRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabaseRole_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbK8s := "dbr-orphan-parent"
	sfDB := "DBR_ORPHAN_PARENT"

	_, cleanupDB := setupReadyDatabase(t, dbK8s, sfDB)
	defer cleanupDB()

	roleK8s := "dbrole-orphan-test"
	sfRole := "DBROLE_ORPHAN_TEST"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	databaseRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
		if created.Load() {
			return databaseRoleObservation(sfRole, sfDB, "", "USERADMIN"), nil
		}

		return &snowflake.DatabaseRoleObservation{Exists: false}, nil
	})

	databaseRoleMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseRoleOptions) error {
		created.Store(true)
		return nil
	})

	databaseRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	role := newTestDatabaseRole(roleK8s, sfRole, dbK8s)
	role.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.DatabaseRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestDatabaseRole_DriftDetection(t *testing.T) {
	resetMocks()

	dbK8s := "dbr-drift-parent"
	sfDB := "DBR_DRIFT_PARENT"

	_, cleanupDB := setupReadyDatabase(t, dbK8s, sfDB)
	defer cleanupDB()

	roleK8s := "dbrole-drift-test"
	sfRole := "DBROLE_DRIFT_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("desired comment")

	databaseRoleMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
		if created.Load() {
			return databaseRoleObservation(sfRole, sfDB, curComment.Load().(string), "USERADMIN"), nil
		}

		return &snowflake.DatabaseRoleObservation{Exists: false}, nil
	})

	databaseRoleMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	databaseRoleMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterDatabaseRoleOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	role := newTestDatabaseRole(roleK8s, sfRole, dbK8s)
	comment := "desired comment"
	role.Spec.Comment = &comment
	require.NoError(t, k8sClient.Create(ctx, role))

	key := types.NamespacedName{Name: roleK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Simulate external drift.
	curComment.Store("externally changed")

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "drift should trigger ALTER")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "desired comment"
	}, defaultTimeout, defaultInterval, "status should reflect corrected comment")

	// Cleanup.
	databaseRoleMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.DatabaseRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.DatabaseRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
