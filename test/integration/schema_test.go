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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Schema Integration Tests
// --------------------------------------------------------------------------

func TestSchema_CreateWithDatabaseRef(t *testing.T) {
	resetMocks()

	dbName := "schema-parent-db"
	sfDBName := "SCHEMA_PARENT_DB"
	schemaName := "schema-create-test"
	sfSchemaName := "SCHEMA_CREATE_TEST"

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "parent database should become Ready")

	var schemaCreated atomic.Bool

	schemaMockSvc.SetObserve(func(_ context.Context, id snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			return schemaObservation(sfSchemaName, sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schemaMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateSchemaOptions) error {
		assert.Equal(t, sfSchemaName, opts.Name.Name())
		assert.Equal(t, sfDBName, opts.Name.DatabaseName())
		schemaCreated.Store(true)

		return nil
	})

	schema := newTestSchema(schemaName, sfSchemaName, dbName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey := types.NamespacedName{Name: schemaName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "schema should become Ready")

	var result snowplanev1alpha1.Schema
	require.NoError(t, k8sClient.Get(ctx, schemaKey, &result))

	assert.True(t, schemaCreated.Load(), "Snowflake CREATE should have been called for schema")
	assert.Equal(t, sfSchemaName, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDBName, result.Status.ShowOutput.DatabaseName)
	assert.Equal(t, sfDBName, result.Status.DatabaseName)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/schema")

	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)

	schemaMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema

		return k8sClient.Get(ctx, schemaKey, &obj) != nil
	}, defaultTimeout, defaultInterval)

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var dbCurrent snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, dbKey, &dbCurrent))
	require.NoError(t, k8sClient.Delete(ctx, &dbCurrent))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, dbKey, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestSchema_WaitForDatabaseReady(t *testing.T) {
	resetMocks()

	schemaName := "schema-wait-db"
	sfSchemaName := "WAIT_DB_SCHEMA"
	dbRefName := "nonexistent-db"

	schemaMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schema := newTestSchema(schemaName, sfSchemaName, dbRefName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey := types.NamespacedName{Name: schemaName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		refCond := conditions.Get(&obj, snowplanev1alpha1.TypeReferencesResolved)

		return refCond != nil && refCond.Status == metav1.ConditionFalse
	}, defaultTimeout, defaultInterval, "schema should have ReferencesResolved=False when database does not exist")

	var result snowplanev1alpha1.Schema
	require.NoError(t, k8sClient.Get(ctx, schemaKey, &result))

	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady),
		"schema should not be Ready when database ref cannot be resolved")

	require.NoError(t, k8sClient.Get(ctx, schemaKey, &result))
	controllerutil.RemoveFinalizer(&result, "snowplane.hupe1980.github.io/schema")
	require.NoError(t, k8sClient.Update(ctx, &result))
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema

		return k8sClient.Get(ctx, schemaKey, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestSchema_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	dbName := "schema-alter-parent-db"
	sfDBName := "SCHEMA_ALTER_PARENT_DB"
	schemaName := "schema-alter-test"
	sfSchemaName := "SCHEMA_ALTER_TEST"

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var (
		schemaCreated atomic.Bool
		curComment    atomic.Value
	)

	curComment.Store("")

	schemaMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			obs := schemaObservation(sfSchemaName, sfDBName, curComment.Load().(string), "SYSADMIN")

			return obs, nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	// Schema supports CREATE OR ALTER, so updates go through the Create
	// mock (CREATE OR ALTER) rather than the Alter mock.
	schemaMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateSchemaOptions) error {
		schemaCreated.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	schema := newTestSchema(schemaName, sfSchemaName, dbName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey := types.NamespacedName{Name: schemaName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Schema
	require.NoError(t, k8sClient.Get(ctx, schemaKey, &current))

	newComment := "schema updated comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	// CREATE OR ALTER is the default for Schema, so the Create mock
	// handles the update. Verify the status reflects the new comment.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "schema updated comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	schemaMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Get(ctx, schemaKey, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema

		return k8sClient.Get(ctx, schemaKey, &obj) != nil
	}, defaultTimeout, defaultInterval)

	var dbCurrent snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, dbKey, &dbCurrent))
	require.NoError(t, k8sClient.Delete(ctx, &dbCurrent))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, dbKey, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestSchema_DeleteWithDatabaseGone(t *testing.T) {
	resetMocks()

	dbName := "schema-del-parent-db"
	sfDBName := "SCHEMA_DEL_PARENT_DB"
	schemaName := "schema-del-test"
	sfSchemaName := "SCHEMA_DEL_TEST"

	var dbCreated atomic.Bool

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName && dbCreated.Load() {
			return databaseObservation(sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		if opts.Name.Name() == sfDBName {
			dbCreated.Store(true)
		}

		return nil
	})

	db := newTestDatabase(dbName, sfDBName)
	require.NoError(t, k8sClient.Create(ctx, db))

	dbKey := types.NamespacedName{Name: dbName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, dbKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var schemaCreated atomic.Bool

	schemaMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			return schemaObservation(sfSchemaName, sfDBName, "", "SYSADMIN"), nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schemaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
		schemaCreated.Store(true)

		return nil
	})

	schema := newTestSchema(schemaName, sfSchemaName, dbName)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey := types.NamespacedName{Name: schemaName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			obj.Status.DatabaseName != ""
	}, defaultTimeout, defaultInterval)

	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var dbCurrent snowplanev1alpha1.Database
	require.NoError(t, k8sClient.Get(ctx, dbKey, &dbCurrent))
	require.NoError(t, k8sClient.Delete(ctx, &dbCurrent))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Database

		return k8sClient.Get(ctx, dbKey, &obj) != nil
	}, defaultTimeout, defaultInterval)

	var schemaDropped atomic.Bool

	schemaMockSvc.SetDrop(func(_ context.Context, id snowflake.DatabaseObjectIdentifier) error {
		schemaDropped.Store(true)

		return nil
	})

	var schemaCurrent snowplanev1alpha1.Schema
	require.NoError(t, k8sClient.Get(ctx, schemaKey, &schemaCurrent))
	require.NoError(t, k8sClient.Delete(ctx, &schemaCurrent))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema

		return k8sClient.Get(ctx, schemaKey, &obj) != nil
	}, defaultTimeout, defaultInterval, "schema should be deleted even with parent database gone")

	assert.True(t, schemaDropped.Load(), "Snowflake DROP should be called with cached database name")
}

func TestSchema_ImmutableDatabaseRef(t *testing.T) {
	resetMocks()

	dbName1 := "schema-immut-db1"
	sfDBName1 := "SCHEMA_IMMUT_DB1"
	dbName2 := "schema-immut-db2"
	sfDBName2 := "SCHEMA_IMMUT_DB2"
	schemaName := "schema-immut-ref"
	sfSchemaName := "SCHEMA_IMMUT_REF"

	var (
		db1Created atomic.Bool
		db2Created atomic.Bool
	)

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if id.Name() == sfDBName1 && db1Created.Load() {
			return databaseObservation(sfDBName1, "", "SYSADMIN"), nil
		}

		if id.Name() == sfDBName2 && db2Created.Load() {
			return databaseObservation(sfDBName2, "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		switch opts.Name.Name() {
		case sfDBName1:
			db1Created.Store(true)
		case sfDBName2:
			db2Created.Store(true)
		}

		return nil
	})

	db1 := newTestDatabase(dbName1, sfDBName1)
	require.NoError(t, k8sClient.Create(ctx, db1))

	db2 := newTestDatabase(dbName2, sfDBName2)
	require.NoError(t, k8sClient.Create(ctx, db2))

	db1Key := types.NamespacedName{Name: dbName1, Namespace: testNamespace}
	db2Key := types.NamespacedName{Name: dbName2, Namespace: testNamespace}

	for _, key := range []types.NamespacedName{db1Key, db2Key} {
		k := key
		require.Eventually(t, func() bool {
			var obj snowplanev1alpha1.Database
			if err := k8sClient.Get(ctx, k, &obj); err != nil {
				return false
			}

			return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
		}, defaultTimeout, defaultInterval)
	}

	var schemaCreated atomic.Bool

	schemaMockSvc.SetObserve(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
		if schemaCreated.Load() {
			return schemaObservation(sfSchemaName, sfDBName1, "", "SYSADMIN"), nil
		}

		return &snowflake.SchemaObservation{Exists: false}, nil
	})

	schemaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
		schemaCreated.Store(true)

		return nil
	})

	schema := newTestSchema(schemaName, sfSchemaName, dbName1)
	require.NoError(t, k8sClient.Create(ctx, schema))

	schemaKey := types.NamespacedName{Name: schemaName, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema
		if err := k8sClient.Get(ctx, schemaKey, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// CEL validation now rejects immutable field changes at the API-server level.
	var current snowplanev1alpha1.Schema

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if getErr := k8sClient.Get(ctx, schemaKey, &current); getErr != nil {
			return getErr
		}

		current.Spec.DatabaseRef = &snowplanev1alpha1.ObjectReference{Name: dbName2}

		return k8sClient.Update(ctx, &current)
	})
	require.Error(t, err, "Update should be rejected by CEL validation")
	assert.Contains(t, err.Error(), "spec.databaseRef is immutable")

	// Cleanup.
	schemaMockSvc.SetDrop(func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error { return nil })
	dbMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Get(ctx, schemaKey, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Schema

		return k8sClient.Get(ctx, schemaKey, &obj) != nil
	}, defaultTimeout, defaultInterval)

	for _, key := range []types.NamespacedName{db1Key, db2Key} {
		k := key

		var dbObj snowplanev1alpha1.Database
		require.NoError(t, k8sClient.Get(ctx, k, &dbObj))
		require.NoError(t, k8sClient.Delete(ctx, &dbObj))

		require.Eventually(t, func() bool {
			var obj snowplanev1alpha1.Database

			return k8sClient.Get(ctx, k, &obj) != nil
		}, defaultTimeout, defaultInterval)
	}
}
