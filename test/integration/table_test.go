//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestTable_CreateLifecycle(t *testing.T) {
	resetMocks()
	dbK8s := "tbl-create-db"
	sfDB := "TBL_CREATE_DB"
	schemaK8s := "tbl-create-schema"
	sfSchema := "TBL_CREATE_SCHEMA"
	tblK8s := "tbl-create-test"
	sfTbl := "TBL_CREATE_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var created atomic.Bool
	tableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
		if created.Load() {
			return tableObservation(sfTbl, sfDB, sfSchema, "", "SYSADMIN"), nil
		}
		return &snowflake.TableObservation{Exists: false}, nil
	})
	tableMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateTableOptions) error {
		assert.Equal(t, sfTbl, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.Len(t, opts.Columns, 2)
		created.Store(true)
		return nil
	})
	tbl := newTestTable(tblK8s, sfTbl, dbK8s, schemaK8s)
	require.NoError(t, k8sClient.Create(ctx, tbl))
	key := types.NamespacedName{Name: tblK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "table should become Ready")
	var result snowplanev1alpha1.Table
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	assert.True(t, created.Load(), "Snowflake CREATE should have been called")
	assert.Equal(t, sfTbl, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/table")
	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)
	tableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "table should be cleaned up")
}

func TestTable_UpdateTriggersAlter(t *testing.T) {
	resetMocks()
	dbK8s := "tbl-alter-db"
	sfDB := "TBL_ALTER_DB"
	schemaK8s := "tbl-alter-schema"
	sfSchema := "TBL_ALTER_SCHEMA"
	tblK8s := "tbl-alter-test"
	sfTbl := "TBL_ALTER_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var (
		created    atomic.Bool
		curComment atomic.Value
	)
	curComment.Store("")
	tableMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
		if created.Load() {
			obs := tableObservation(sfTbl, sfDB, sfSchema, curComment.Load().(string), "SYSADMIN")
			return obs, nil
		}
		return &snowflake.TableObservation{Exists: false}, nil
	})
	// Table supports CREATE OR ALTER, so updates go through the Create
	// mock (CREATE OR ALTER) rather than the Alter mock.
	tableMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateTableOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}
		return nil
	})
	tbl := newTestTable(tblK8s, sfTbl, dbK8s, schemaK8s)
	initComment := "initial table comment"
	tbl.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, tbl))
	key := types.NamespacedName{Name: tblK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "table should become Ready initially")
	var current snowplanev1alpha1.Table
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	newComment := "updated table comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))
	// CREATE OR ALTER is the default for Table, so the Create mock
	// handles the update. Verify the status reflects the new comment.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated table comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")
	tableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestTable_WaitForSchemaReady(t *testing.T) {
	resetMocks()
	tblK8s := "tbl-wait-schema"
	sfTbl := "WAIT_SCHEMA_TBL"
	tbl := newTestTable(tblK8s, sfTbl, "nonexistent-db", "nonexistent-schema")
	require.NoError(t, k8sClient.Create(ctx, tbl))
	key := types.NamespacedName{Name: tblK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		refCond := conditions.Get(&obj, snowplanev1alpha1.TypeReferencesResolved)
		return refCond != nil && refCond.Status == metav1.ConditionFalse
	}, defaultTimeout, defaultInterval, "table should have ReferencesResolved=False")
	var result snowplanev1alpha1.Table
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady),
		"table should not be Ready when references cannot be resolved")
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	controllerutil.RemoveFinalizer(&result, "snowplane.hupe1980.github.io/table")
	require.NoError(t, k8sClient.Update(ctx, &result))
	require.NoError(t, k8sClient.Delete(ctx, &result))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestTable_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()
	dbK8s := "tbl-orphan-db"
	sfDB := "TBL_ORPHAN_DB"
	schemaK8s := "tbl-orphan-schema"
	sfSchema := "TBL_ORPHAN_SCHEMA"
	tblK8s := "tbl-orphan-test"
	sfTbl := "TBL_ORPHAN_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var (
		created atomic.Bool
		dropped atomic.Bool
	)
	tableMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
		if created.Load() {
			return tableObservation(sfTbl, sfDB, sfSchema, "", "SYSADMIN"), nil
		}
		return &snowflake.TableObservation{Exists: false}, nil
	})
	tableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateTableOptions) error {
		created.Store(true)
		return nil
	})
	tableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})
	tbl := newTestTable(tblK8s, sfTbl, dbK8s, schemaK8s)
	tbl.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, tbl))
	key := types.NamespacedName{Name: tblK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)
	var current snowplanev1alpha1.Table
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestTable_DriftDetection(t *testing.T) {
	resetMocks()
	dbK8s := "tbl-drift-db"
	sfDB := "TBL_DRIFT_DB"
	schemaK8s := "tbl-drift-schema"
	sfSchema := "TBL_DRIFT_SCHEMA"
	tblK8s := "tbl-drift-test"
	sfTbl := "TBL_DRIFT_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var (
		created    atomic.Bool
		curComment atomic.Value
	)
	curComment.Store("")
	tableMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
		if created.Load() {
			return tableObservation(sfTbl, sfDB, sfSchema, curComment.Load().(string), "SYSADMIN"), nil
		}
		return &snowflake.TableObservation{Exists: false}, nil
	})
	// Table supports CREATE OR ALTER, so drift correction goes through
	// the Create mock rather than the Alter mock.
	tableMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateTableOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}
		return nil
	})
	myComment := "drift table comment"
	tbl := newTestTable(tblK8s, sfTbl, dbK8s, schemaK8s)
	tbl.Spec.Comment = &myComment
	require.NoError(t, k8sClient.Create(ctx, tbl))
	key := types.NamespacedName{Name: tblK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) && obj.Status.LastAppliedSpecHash != ""
	}, defaultTimeout, defaultInterval)
	curComment.Store("externally changed")
	// Table supports CREATE OR ALTER, so drift correction goes through
	// the Create mock. Verify the comment is restored to the desired value.
	require.Eventually(t, func() bool {
		return curComment.Load().(string) == "drift table comment"
	}, defaultTimeout, defaultInterval, "drift should be detected and corrected")
	tableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	var current snowplanev1alpha1.Table
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Table
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
