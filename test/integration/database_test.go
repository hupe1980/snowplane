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
	"k8s.io/client-go/util/retry"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Database Integration Tests
// --------------------------------------------------------------------------

func TestDatabase_CreateLifecycle(t *testing.T) {
	resetMocks()

	dbName := "test-create-lifecycle"
	sfName := "CREATE_LIFECYCLE_DB"

	var created atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		assert.Equal(t, sfName, opts.Name.Name())
		created.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "database should become Ready")

	var result snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE should have been called")
	assert.Equal(t, sfName, result.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/database")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		err := k8sClient.Get(ctx, key, &obj)

		return err != nil
	}, defaultTimeout, defaultInterval, "database should be cleaned up")
}

func TestDatabase_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	dbName := "test-update-alter"
	sfName := "UPDATE_ALTER_DB"

	var (
		created    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			obs := databaseObservation(sfName, curComment.Load().(string), "SYSADMIN")

			return obs, nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	// Database supports CREATE OR ALTER, so updates go through the Create
	// mock (CREATE OR ALTER) rather than the Alter mock.
	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	initComment := "initial comment"
	db.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database should become Ready initially")

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	// CREATE OR ALTER is the default for Database, so the Create mock
	// handles the update. Verify the status reflects the new comment.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbName := "test-orphan-delete"
	sfName := "ORPHAN_DELETE_DB"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropped.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	db.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestDatabase_FinalizerAddedOnCreate(t *testing.T) {
	resetMocks()

	dbName := "test-finalizer"
	sfName := "FINALIZER_DB"

	var created atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		for _, f := range obj.Finalizers {
			if f == "snowplane.hupe1980.github.io/database" {
				return true
			}
		}

		return false
	}, defaultTimeout, defaultInterval, "finalizer should be added")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_DriftDetection(t *testing.T) {
	resetMocks()

	dbName := "test-drift"
	sfName := "DRIFT_DB"

	var (
		created    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			obs := databaseObservation(sfName, curComment.Load().(string), "SYSADMIN")

			return obs, nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	// Database supports CREATE OR ALTER, so drift correction goes through
	// the Create mock rather than the Alter mock.
	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	myComment := "original drift comment"
	db := newTestDatabase(dbName, sfName)
	db.Spec.Comment = &myComment
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) && obj.Status.LastAppliedSpecHash != ""
	}, defaultTimeout, defaultInterval)

	curComment.Store("externally changed")

	// Database supports CREATE OR ALTER, so drift correction goes through
	// the Create mock (CREATE OR ALTER) rather than the Alter mock.
	// Verify the comment is restored to the desired spec value.
	require.Eventually(t, func() bool {
		return curComment.Load().(string) == "original drift comment"
	}, defaultTimeout, defaultInterval, "drift should be detected and corrected")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_ImmutableFieldRejection(t *testing.T) {
	resetMocks()

	dbName := "test-immutable"
	sfName := "IMMUTABLE_DB"

	var (
		created atomic.Bool
		altered atomic.Bool
	)

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	dbMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
		altered.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	db.Spec.Transient = false
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// CEL validation now rejects immutable field changes at the API-server level.
	var current snowplanev1alpha1.Database

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if getErr := k8sClient.Get(ctx, key, &current); getErr != nil {
			return getErr
		}

		current.Spec.Transient = true

		return k8sClient.Update(ctx, &current)
	})
	require.Error(t, err, "Update should be rejected by CEL validation")
	assert.Contains(t, err.Error(), "spec.transient is immutable")

	assert.False(t, altered.Load(), "ALTER should NOT be called for immutable field change")

	// Cleanup.
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_ObservedGenerationUpdated(t *testing.T) {
	resetMocks()

	dbName := "test-obs-gen"
	sfName := "OBS_GEN_DB"

	var created atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ObservedGeneration == obj.Generation && obj.Generation > 0
	}, defaultTimeout, defaultInterval, "ObservedGeneration should match Generation")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_TrackedParametersTracking(t *testing.T) {
	resetMocks()

	dbName := "test-managed-fields"
	sfName := "MANAGED_FIELDS_DB"

	var (
		created    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("test comment")

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			obs := databaseObservation(sfName, curComment.Load().(string), "SYSADMIN")

			return obs, nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	comment := "test comment"
	retention := int32(7)
	db := newTestDatabase(dbName, sfName)
	db.Spec.Comment = &comment
	db.Spec.DataRetentionTimeInDays = &retention
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		if !conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) {
			return false
		}

		managed := make(map[string]bool)
		for _, f := range obj.Status.TrackedParameters {
			managed[f] = true
		}

		return managed["COMMENT"] && managed["DATA_RETENTION_TIME_IN_DAYS"]
	}, defaultTimeout, defaultInterval, "TrackedParameters should track COMMENT and DATA_RETENTION_TIME_IN_DAYS")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_StatusSubresourcePatch(t *testing.T) {
	resetMocks()

	dbName := "test-status-patch"
	sfName := "STATUS_PATCH_DB"

	var created atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	if current.Labels == nil {
		current.Labels = make(map[string]string)
	}

	current.Labels["test"] = "value"
	require.NoError(t, k8sClient.Update(ctx, &current))

	// Wait for at least one reconciliation cycle after the label update.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Labels["test"] == "value" &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var afterLabel snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &afterLabel))

	assert.True(t, conditions.IsTrue(&afterLabel, snowplanev1alpha1.TypeReady),
		"status should still be Ready after label update")
	assert.NotNil(t, afterLabel.Status.ShowOutput, "ShowOutput should persist after label update")

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Delete(ctx, &afterLabel))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

// --------------------------------------------------------------------------
// Adoption Integration Tests
// --------------------------------------------------------------------------

func TestDatabase_Adoption_FailIfExists(t *testing.T) {
	resetMocks()

	dbName := "test-adopt-fail"
	sfName := "ADOPT_FAIL_DB"

	// Observe always returns Exists: true — the Snowflake resource is pre-existing.
	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		return databaseObservation(sfName, "", "SYSADMIN"), nil
	})

	// No adoption annotation → should hit Terminal with ReasonResourceExists.
	db := newTestDatabase(dbName, sfName)
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTerminal(&obj)
	}, defaultTimeout, defaultInterval, "should become Terminal when resource exists without adoption annotation")

	var result snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	tc := conditions.Get(&result, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)
	assert.Contains(t, tc.Message, "already exists")
	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady), "should NOT be Ready")

	// Cleanup: force-delete by removing finalizer.
	var cleanup snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &cleanup))

	cleanup.Finalizers = nil
	require.NoError(t, k8sClient.Update(ctx, &cleanup))
	require.NoError(t, k8sClient.Delete(ctx, &cleanup))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Database{}) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_Adoption_AdoptSuccess(t *testing.T) {
	resetMocks()

	dbName := "test-adopt-success"
	sfName := "ADOPT_SUCCESS_DB"

	// Observe always returns Exists: true — pre-existing Snowflake resource.
	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		return databaseObservation(sfName, "pre-existing", "SYSADMIN"), nil
	})

	// With adoption annotation → should adopt and become Ready.
	db := newTestDatabase(dbName, sfName)
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAdoptionPolicy: snowplanev1alpha1.AdoptionPolicyAdopt,
	}
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced) &&
			obj.Annotations[snowplanev1alpha1.AnnotationLateInitialized] == "true"
	}, defaultTimeout, defaultInterval, "adopted database should become Ready with LateInitialized")

	var result snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	// Verify adoption results.
	assert.Equal(t, sfName, result.Status.ShowOutput.Name, "ShowOutput should be populated from existing resource")
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.Equal(t, "pre-existing", result.Status.ShowOutput.Comment)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.False(t, conditions.IsTerminal(&result), "should NOT be terminal")

	// Verify LateInitialized annotation.
	assert.Equal(t, "true", result.Annotations[snowplanev1alpha1.AnnotationLateInitialized])

	// Cleanup.
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Database{}) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_Adoption_ExplicitFailIfExists(t *testing.T) {
	resetMocks()

	dbName := "test-adopt-explicit-fail"
	sfName := "ADOPT_EXPLICIT_FAIL_DB"

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		return databaseObservation(sfName, "", "SYSADMIN"), nil
	})

	// Explicit `fail-if-exists` annotation → should become Terminal.
	db := newTestDatabase(dbName, sfName)
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAdoptionPolicy: snowplanev1alpha1.AdoptionPolicyFailIfExists,
	}
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTerminal(&obj)
	}, defaultTimeout, defaultInterval, "should become Terminal with explicit fail-if-exists annotation")

	var result snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	tc := conditions.Get(&result, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)

	// Cleanup.
	var cleanup snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &cleanup))

	cleanup.Finalizers = nil
	require.NoError(t, k8sClient.Update(ctx, &cleanup))
	require.NoError(t, k8sClient.Delete(ctx, &cleanup))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Database{}) != nil
	}, defaultTimeout, defaultInterval)
}

// ---------------------------------------------------------------------------
// Annotation Toggle Tests — verifies both CREATE OR ALTER (default) and
// traditional ALTER (opt-out via annotation) paths.
// ---------------------------------------------------------------------------

func TestDatabase_UpdateUsesCreateOrAlterByDefault(t *testing.T) {
	resetMocks()

	dbName := "test-coa-default"
	sfName := "COA_DEFAULT_DB"

	var (
		created     atomic.Bool
		createCalls atomic.Int32
		curComment  atomic.Value
	)

	curComment.Store("")

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, curComment.Load().(string), "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		createCalls.Add(1)
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	// Alter should NOT be called when CREATE OR ALTER is enabled (default).
	dbMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
		t.Error("ALTER should not be called when CREATE OR ALTER is the default")

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	initComment := "coa default initial"
	db.Spec.Comment = &initComment
	// No annotation set — CREATE OR ALTER is the default for Database.
	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database should become Ready")

	// The first Create call was the initial creation.
	assert.Equal(t, int32(1), createCalls.Load(), "exactly one Create call for initial creation")

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "coa default updated"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	// Wait for CREATE OR ALTER to process the update via Create mock.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "coa default updated"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment via CREATE OR ALTER")

	// Create should have been called at least twice (initial + update).
	assert.GreaterOrEqual(t, createCalls.Load(), int32(2), "Create should be called for both initial and update (CREATE OR ALTER)")

	// Cleanup.
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestDatabase_UpdateUsesAlterWhenAnnotationDisabled(t *testing.T) {
	resetMocks()

	dbName := "test-coa-disabled"
	sfName := "COA_DISABLED_DB"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	dbMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if created.Load() {
			return databaseObservation(sfName, curComment.Load().(string), "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	// Alter SHOULD be called when CREATE OR ALTER is disabled via annotation.
	dbMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterDatabaseOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfName)
	initComment := "coa disabled initial"
	db.Spec.Comment = &initComment

	// Explicitly disable CREATE OR ALTER via annotation.
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}

	require.NoError(t, k8sClient.Create(ctx, db))

	key := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "database should become Ready")

	var current snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "coa disabled updated"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	// With CREATE OR ALTER disabled, the traditional ALTER path is used.
	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called with annotation disabled")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "coa disabled updated"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment via ALTER")

	// Cleanup.
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
