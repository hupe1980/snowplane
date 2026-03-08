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

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestCortexSearchService_CreateLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "cortex-create-db"
	sfDB := "CORTEX_CREATE_DB"
	schemaK8s := "cortex-create-schema"
	sfSchema := "CORTEX_CREATE_SCHEMA"
	cssK8s := "cortex-create-test"
	sfCSS := "CORTEX_CREATE_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var created atomic.Bool

	cortexSearchServiceMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error) {
		if created.Load() {
			return cortexSearchServiceObservation(sfCSS, sfDB, sfSchema, "TEXT_COL", "1 hour", "SELECT TEXT_COL FROM my_table", ""), nil
		}

		return &snowflake.CortexSearchServiceObservation{Exists: false}, nil
	})

	cortexSearchServiceMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateCortexSearchServiceOptions) error {
		assert.Equal(t, sfCSS, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.Equal(t, "TEXT_COL", opts.On)
		assert.Equal(t, "1 hour", opts.TargetLag)
		assert.Equal(t, "SELECT TEXT_COL FROM my_table", opts.Query)
		created.Store(true)

		return nil
	})

	css := newTestCortexSearchService(cssK8s, sfCSS, dbK8s, schemaK8s)
	require.NoError(t, k8sClient.Create(ctx, css))

	key := types.NamespacedName{Name: cssK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "cortex search service should become Ready")

	var result snowplanev1alpha1.CortexSearchService
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load())
	assert.Equal(t, sfCSS, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/cortexsearchservice")

	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)

	// Cleanup
	cortexSearchServiceMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "cortex search service should be cleaned up")
}

func TestCortexSearchService_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbK8s := "cortex-orphan-db"
	sfDB := "CORTEX_ORPHAN_DB"
	schemaK8s := "cortex-orphan-schema"
	sfSchema := "CORTEX_ORPHAN_SCHEMA"
	cssK8s := "cortex-orphan-test"
	sfCSS := "CORTEX_ORPHAN_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	cortexSearchServiceMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error) {
		if created.Load() {
			return cortexSearchServiceObservation(sfCSS, sfDB, sfSchema, "TEXT_COL", "1 hour", "SELECT TEXT_COL FROM my_table", ""), nil
		}

		return &snowflake.CortexSearchServiceObservation{Exists: false}, nil
	})

	cortexSearchServiceMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateCortexSearchServiceOptions) error {
		created.Store(true)
		return nil
	})

	cortexSearchServiceMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	css := newTestCortexSearchService(cssK8s, sfCSS, dbK8s, schemaK8s)
	css.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, css))

	key := types.NamespacedName{Name: cssK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.CortexSearchService
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestCortexSearchService_UpdateComment(t *testing.T) {
	resetMocks()

	dbK8s := "cortex-alter-db"
	sfDB := "CORTEX_ALTER_DB"
	schemaK8s := "cortex-alter-schema"
	sfSchema := "CORTEX_ALTER_SCHEMA"
	cssK8s := "cortex-alter-test"
	sfCSS := "CORTEX_ALTER_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	cortexSearchServiceMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error) {
		if created.Load() {
			obs := cortexSearchServiceObservation(sfCSS, sfDB, sfSchema, "TEXT_COL", "1 hour", "SELECT TEXT_COL FROM my_table", curComment.Load().(string))
			return obs, nil
		}

		return &snowflake.CortexSearchServiceObservation{Exists: false}, nil
	})

	cortexSearchServiceMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateCortexSearchServiceOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	cortexSearchServiceMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterCortexSearchServiceOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	css := newTestCortexSearchService(cssK8s, sfCSS, dbK8s, schemaK8s)
	initComment := "initial cortex comment"
	css.Spec.Comment = &initComment
	f := false
	css.Spec.ManagementPolicies.CreateOrAlter = &f

	require.NoError(t, k8sClient.Create(ctx, css))

	key := types.NamespacedName{Name: cssK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment
	var current snowplanev1alpha1.CortexSearchService
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated cortex comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated cortex comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup
	cortexSearchServiceMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.CortexSearchService
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
