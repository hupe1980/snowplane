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

func TestStage_CreateInternalLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "stage-create-int-db"
	sfDB := "STAGE_CREATE_INT_DB"
	schemaK8s := "stage-create-int-schema"
	sfSchema := "STAGE_CREATE_INT_SCHEMA"
	stageK8s := "stage-create-int-test"
	sfStage := "STAGE_CREATE_INT_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var created atomic.Bool

	stageMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
		if created.Load() {
			return stageObservation(sfStage, sfDB, sfSchema, "", "SYSADMIN", "INTERNAL"), nil
		}

		return &snowflake.StageObservation{Exists: false}, nil
	})

	stageMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStageOptions) error {
		assert.Equal(t, sfStage, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.Nil(t, opts.URL, "internal stage should have no URL")
		created.Store(true)

		return nil
	})

	stage := newTestStage(stageK8s, sfStage, dbK8s, schemaK8s)
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "internal stage should become Ready")

	var result snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE should have been called")
	assert.Equal(t, sfStage, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.Equal(t, "INTERNAL", result.Status.ShowOutput.Type)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/stage")

	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)

	stageMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "stage should be cleaned up")
}

func TestStage_CreateExternalLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "stage-create-ext-db"
	sfDB := "STAGE_CREATE_EXT_DB"
	schemaK8s := "stage-create-ext-schema"
	sfSchema := "STAGE_CREATE_EXT_SCHEMA"
	stageK8s := "stage-create-ext-test"
	sfStage := "STAGE_CREATE_EXT_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var created atomic.Bool

	stageMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
		if created.Load() {
			obs := stageObservation(sfStage, sfDB, sfSchema, "", "SYSADMIN", "EXTERNAL")
			obs.ShowOutput.URL = "s3://my-bucket/path/"
			obs.ShowOutput.StorageIntegration = "MY_INTEGRATION"

			return obs, nil
		}

		return &snowflake.StageObservation{Exists: false}, nil
	})

	stageMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStageOptions) error {
		assert.Equal(t, sfStage, opts.Name.Name())
		assert.NotNil(t, opts.URL)
		assert.Equal(t, "s3://my-bucket/path/", *opts.URL)
		assert.NotNil(t, opts.StorageIntegration)
		assert.Equal(t, "MY_INTEGRATION", *opts.StorageIntegration)
		created.Store(true)

		return nil
	})

	stage := newTestStage(stageK8s, sfStage, dbK8s, schemaK8s)
	stage.Spec.URL = ptrString("s3://my-bucket/path/")
	stage.Spec.StorageIntegration = ptrString("MY_INTEGRATION")
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "external stage should become Ready")

	var result snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load())
	assert.Equal(t, "EXTERNAL", result.Status.ShowOutput.Type)
	assert.Equal(t, "s3://my-bucket/path/", result.Status.ShowOutput.URL)
	assert.Equal(t, "MY_INTEGRATION", result.Status.ShowOutput.StorageIntegration)

	stageMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestStage_WaitForSchemaReady(t *testing.T) {
	resetMocks()

	stageK8s := "stage-wait-schema"
	sfStage := "WAIT_SCHEMA_STAGE"

	stage := newTestStage(stageK8s, sfStage, "nonexistent-db", "nonexistent-schema")
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		refCond := conditions.Get(&obj, snowplanev1alpha1.TypeReferencesResolved)

		return refCond != nil && refCond.Status == metav1.ConditionFalse
	}, defaultTimeout, defaultInterval, "stage should have ReferencesResolved=False when database does not exist")

	var result snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady),
		"stage should not be Ready when references cannot be resolved")

	require.NoError(t, k8sClient.Get(ctx, key, &result))
	controllerutil.RemoveFinalizer(&result, "snowplane.hupe1980.github.io/stage")
	require.NoError(t, k8sClient.Update(ctx, &result))
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestStage_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	dbK8s := "stage-alter-db"
	sfDB := "STAGE_ALTER_DB"
	schemaK8s := "stage-alter-schema"
	sfSchema := "STAGE_ALTER_SCHEMA"
	stageK8s := "stage-alter-test"
	sfStage := "STAGE_ALTER_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	stageMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
		if created.Load() {
			obs := stageObservation(sfStage, sfDB, sfSchema, curComment.Load().(string), "SYSADMIN", "INTERNAL")

			return obs, nil
		}

		return &snowflake.StageObservation{Exists: false}, nil
	})

	stageMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStageOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	stageMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterStageOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	stage := newTestStage(stageK8s, sfStage, dbK8s, schemaK8s)
	initComment := "initial stage comment"
	stage.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "stage should become Ready initially")

	var current snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated stage comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated stage comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	stageMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestStage_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbK8s := "stage-orphan-db"
	sfDB := "STAGE_ORPHAN_DB"
	schemaK8s := "stage-orphan-schema"
	sfSchema := "STAGE_ORPHAN_SCHEMA"
	stageK8s := "stage-orphan-test"
	sfStage := "STAGE_ORPHAN_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	stageMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
		if created.Load() {
			return stageObservation(sfStage, sfDB, sfSchema, "", "SYSADMIN", "INTERNAL"), nil
		}

		return &snowflake.StageObservation{Exists: false}, nil
	})

	stageMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStageOptions) error {
		created.Store(true)

		return nil
	})

	stageMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)

		return nil
	})

	stage := newTestStage(stageK8s, sfStage, dbK8s, schemaK8s)
	stage.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestStage_DriftDetection(t *testing.T) {
	resetMocks()

	dbK8s := "stage-drift-db"
	sfDB := "STAGE_DRIFT_DB"
	schemaK8s := "stage-drift-schema"
	sfSchema := "STAGE_DRIFT_SCHEMA"
	stageK8s := "stage-drift-test"
	sfStage := "STAGE_DRIFT_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created    atomic.Bool
		drifted    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	stageMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
		if created.Load() {
			return stageObservation(sfStage, sfDB, sfSchema, curComment.Load().(string), "SYSADMIN", "INTERNAL"), nil
		}

		return &snowflake.StageObservation{Exists: false}, nil
	})

	stageMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStageOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	stageMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterStageOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			drifted.Store(true)
		}

		return nil
	})

	myComment := "drift stage comment"
	stage := newTestStage(stageK8s, sfStage, dbK8s, schemaK8s)
	stage.Spec.Comment = &myComment
	require.NoError(t, k8sClient.Create(ctx, stage))

	key := types.NamespacedName{Name: stageK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) && obj.Status.LastAppliedSpecHash != ""
	}, defaultTimeout, defaultInterval)

	curComment.Store("externally changed")

	require.Eventually(t, func() bool {
		return drifted.Load()
	}, defaultTimeout, defaultInterval, "drift should be detected and corrected")

	assert.Equal(t, "drift stage comment", curComment.Load().(string))

	stageMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Stage
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Stage

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
