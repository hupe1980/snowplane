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

func TestView_CreateLifecycle(t *testing.T) {
	resetMocks()
	dbK8s := "view-create-db"
	sfDB := "VIEW_CREATE_DB"
	schemaK8s := "view-create-schema"
	sfSchema := "VIEW_CREATE_SCHEMA"
	viewK8s := "view-create-test"
	sfView := "VIEW_CREATE_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var created atomic.Bool
	viewMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
		if created.Load() {
			return viewObservation(sfView, sfDB, sfSchema, "", "SYSADMIN", "SELECT * FROM t", false), nil
		}
		return &snowflake.ViewObservation{Exists: false}, nil
	})
	viewMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateViewOptions) error {
		assert.Equal(t, sfView, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.Equal(t, "SELECT * FROM t", opts.Statement)
		created.Store(true)
		return nil
	})
	view := newTestView(viewK8s, sfView, dbK8s, schemaK8s)
	view.Spec.Statement = "SELECT * FROM t"
	require.NoError(t, k8sClient.Create(ctx, view))
	key := types.NamespacedName{Name: viewK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "view should become Ready")
	var result snowplanev1alpha1.View
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	assert.True(t, created.Load())
	assert.Equal(t, sfView, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/view")
	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)
	viewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "view should be cleaned up")
}

func TestView_CreateSecure(t *testing.T) {
	resetMocks()
	dbK8s := "view-secure-db"
	sfDB := "VIEW_SECURE_DB"
	schemaK8s := "view-secure-schema"
	sfSchema := "VIEW_SECURE_SCHEMA"
	viewK8s := "view-secure-test"
	sfView := "VIEW_SECURE_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var created atomic.Bool
	viewMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
		if created.Load() {
			return viewObservation(sfView, sfDB, sfSchema, "", "SYSADMIN", "SELECT 1", true), nil
		}
		return &snowflake.ViewObservation{Exists: false}, nil
	})
	viewMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateViewOptions) error {
		assert.True(t, opts.Secure, "view should be created as secure")
		created.Store(true)
		return nil
	})
	view := newTestView(viewK8s, sfView, dbK8s, schemaK8s)
	view.Spec.Secure = true
	view.Spec.Statement = "SELECT 1"
	require.NoError(t, k8sClient.Create(ctx, view))
	key := types.NamespacedName{Name: viewK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)
	var result snowplanev1alpha1.View
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	assert.True(t, created.Load())
	assert.True(t, result.Status.ShowOutput.IsSecure, "status should reflect secure view")
	viewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestView_WaitForSchemaReady(t *testing.T) {
	resetMocks()
	viewK8s := "view-wait-schema"
	sfView := "WAIT_SCHEMA_VIEW"
	view := newTestView(viewK8s, sfView, "nonexistent-db", "nonexistent-schema")
	require.NoError(t, k8sClient.Create(ctx, view))
	key := types.NamespacedName{Name: viewK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		refCond := conditions.Get(&obj, snowplanev1alpha1.TypeReferencesResolved)
		return refCond != nil && refCond.Status == metav1.ConditionFalse
	}, defaultTimeout, defaultInterval, "view should have ReferencesResolved=False")
	var result snowplanev1alpha1.View
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady))
	require.NoError(t, k8sClient.Get(ctx, key, &result))
	controllerutil.RemoveFinalizer(&result, "snowplane.hupe1980.github.io/view")
	require.NoError(t, k8sClient.Update(ctx, &result))
	require.NoError(t, k8sClient.Delete(ctx, &result))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestView_UpdateTriggersAlter(t *testing.T) {
	resetMocks()
	dbK8s := "view-alter-db"
	sfDB := "VIEW_ALTER_DB"
	schemaK8s := "view-alter-schema"
	sfSchema := "VIEW_ALTER_SCHEMA"
	viewK8s := "view-alter-test"
	sfView := "VIEW_ALTER_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)
	curComment.Store("")
	viewStatement := "SELECT 1" // Must match newTestView's default Statement field
	viewMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
		if created.Load() {
			return viewObservation(sfView, sfDB, sfSchema, curComment.Load().(string), "SYSADMIN", viewStatement, false), nil
		}
		return &snowflake.ViewObservation{Exists: false}, nil
	})
	viewMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateViewOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}
		return nil
	})
	viewMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterViewOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}
		return nil
	})
	view := newTestView(viewK8s, sfView, dbK8s, schemaK8s)
	// Disable CREATE OR ALTER so update goes through the ALTER path.
	f := false
	view.Spec.ManagementPolicies.CreateOrAlter = &f
	initComment := "initial view comment"
	view.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, view))
	key := types.NamespacedName{Name: viewK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)
	var current snowplanev1alpha1.View
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	newComment := "updated view comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))
	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated view comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")
	viewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestView_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()
	dbK8s := "view-orphan-db"
	sfDB := "VIEW_ORPHAN_DB"
	schemaK8s := "view-orphan-schema"
	sfSchema := "VIEW_ORPHAN_SCHEMA"
	viewK8s := "view-orphan-test"
	sfView := "VIEW_ORPHAN_TEST"
	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()
	var (
		created atomic.Bool
		dropped atomic.Bool
	)
	viewMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
		if created.Load() {
			return viewObservation(sfView, sfDB, sfSchema, "", "SYSADMIN", "SELECT 1", false), nil
		}
		return &snowflake.ViewObservation{Exists: false}, nil
	})
	viewMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateViewOptions) error {
		created.Store(true)
		return nil
	})
	viewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})
	view := newTestView(viewK8s, sfView, dbK8s, schemaK8s)
	view.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, view))
	key := types.NamespacedName{Name: viewK8s, Namespace: testNamespace}
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)
	var current snowplanev1alpha1.View
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.View
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}
