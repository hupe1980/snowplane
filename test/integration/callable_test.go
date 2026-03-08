//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ---------------------------------------------------------------------------
// Function tests
// ---------------------------------------------------------------------------

func TestFunctionJava_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"fj-db", "FJ_DB", "fj-schema", "FJ_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	functionJavaMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.FunctionObservation, error) {
		if id.Name() == "MY_FN_JAVA" && created.Load() {
			return functionObservation("MY_FN_JAVA", "FJ_DB", "FJ_SCHEMA"), nil
		}
		return &snowflake.FunctionObservation{Exists: false}, nil
	})
	functionJavaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFunctionOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFunctionJava("test-fn-java", "MY_FN_JAVA", "fj-db", "fj-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		functionJavaMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.FunctionJava
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FunctionJava{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FunctionJava
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FunctionJava should become Ready")
}

func TestFunctionJavascript_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"fjs-db", "FJS_DB", "fjs-schema", "FJS_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	functionJavascriptMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.FunctionObservation, error) {
		if id.Name() == "MY_FN_JS" && created.Load() {
			return functionObservation("MY_FN_JS", "FJS_DB", "FJS_SCHEMA"), nil
		}
		return &snowflake.FunctionObservation{Exists: false}, nil
	})
	functionJavascriptMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFunctionOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFunctionJavascript("test-fn-js", "MY_FN_JS", "fjs-db", "fjs-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		functionJavascriptMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.FunctionJavascript
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FunctionJavascript{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FunctionJavascript
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FunctionJavascript should become Ready")
}

func TestFunctionPython_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"fpy-db", "FPY_DB", "fpy-schema", "FPY_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	functionPythonMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.FunctionObservation, error) {
		if id.Name() == "MY_FN_PY" && created.Load() {
			return functionObservation("MY_FN_PY", "FPY_DB", "FPY_SCHEMA"), nil
		}
		return &snowflake.FunctionObservation{Exists: false}, nil
	})
	functionPythonMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFunctionOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFunctionPython("test-fn-py", "MY_FN_PY", "fpy-db", "fpy-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		functionPythonMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.FunctionPython
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FunctionPython{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FunctionPython
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FunctionPython should become Ready")
}

func TestFunctionScala_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"fsc-db", "FSC_DB", "fsc-schema", "FSC_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	functionScalaMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.FunctionObservation, error) {
		if id.Name() == "MY_FN_SCALA" && created.Load() {
			return functionObservation("MY_FN_SCALA", "FSC_DB", "FSC_SCHEMA"), nil
		}
		return &snowflake.FunctionObservation{Exists: false}, nil
	})
	functionScalaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFunctionOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFunctionScala("test-fn-scala", "MY_FN_SCALA", "fsc-db", "fsc-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		functionScalaMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.FunctionScala
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FunctionScala{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FunctionScala
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FunctionScala should become Ready")
}

func TestFunctionSQL_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"fsql-db", "FSQL_DB", "fsql-schema", "FSQL_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	functionSQLMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.FunctionObservation, error) {
		if id.Name() == "MY_FN_SQL" && created.Load() {
			return functionObservation("MY_FN_SQL", "FSQL_DB", "FSQL_SCHEMA"), nil
		}
		return &snowflake.FunctionObservation{Exists: false}, nil
	})
	functionSQLMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFunctionOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestFunctionSQL("test-fn-sql", "MY_FN_SQL", "fsql-db", "fsql-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		functionSQLMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.FunctionSQL
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FunctionSQL{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FunctionSQL
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "FunctionSQL should become Ready")
}


// ---------------------------------------------------------------------------
// Procedure tests
// ---------------------------------------------------------------------------

func TestProcedureJava_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pj-db", "PJ_DB", "pj-schema", "PJ_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	procedureJavaMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.ProcedureObservation, error) {
		if id.Name() == "MY_PROC_JAVA" && created.Load() {
			return procedureObservation("MY_PROC_JAVA", "PJ_DB", "PJ_SCHEMA"), nil
		}
		return &snowflake.ProcedureObservation{Exists: false}, nil
	})
	procedureJavaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateProcedureOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestProcedureJava("test-proc-java", "MY_PROC_JAVA", "pj-db", "pj-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		procedureJavaMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.ProcedureJava
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ProcedureJava{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ProcedureJava
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ProcedureJava should become Ready")
}

func TestProcedureJavascript_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pjs-db", "PJS_DB", "pjs-schema", "PJS_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	procedureJavascriptMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.ProcedureObservation, error) {
		if id.Name() == "MY_PROC_JS" && created.Load() {
			return procedureObservation("MY_PROC_JS", "PJS_DB", "PJS_SCHEMA"), nil
		}
		return &snowflake.ProcedureObservation{Exists: false}, nil
	})
	procedureJavascriptMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateProcedureOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestProcedureJavascript("test-proc-js", "MY_PROC_JS", "pjs-db", "pjs-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		procedureJavascriptMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.ProcedureJavascript
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ProcedureJavascript{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ProcedureJavascript
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ProcedureJavascript should become Ready")
}

func TestProcedurePython_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ppy-db", "PPY_DB", "ppy-schema", "PPY_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	procedurePythonMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.ProcedureObservation, error) {
		if id.Name() == "MY_PROC_PY" && created.Load() {
			return procedureObservation("MY_PROC_PY", "PPY_DB", "PPY_SCHEMA"), nil
		}
		return &snowflake.ProcedureObservation{Exists: false}, nil
	})
	procedurePythonMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateProcedureOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestProcedurePython("test-proc-py", "MY_PROC_PY", "ppy-db", "ppy-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		procedurePythonMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.ProcedurePython
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ProcedurePython{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ProcedurePython
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ProcedurePython should become Ready")
}

func TestProcedureScala_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"psc-db", "PSC_DB", "psc-schema", "PSC_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	procedureScalaMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.ProcedureObservation, error) {
		if id.Name() == "MY_PROC_SCALA" && created.Load() {
			return procedureObservation("MY_PROC_SCALA", "PSC_DB", "PSC_SCHEMA"), nil
		}
		return &snowflake.ProcedureObservation{Exists: false}, nil
	})
	procedureScalaMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateProcedureOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestProcedureScala("test-proc-scala", "MY_PROC_SCALA", "psc-db", "psc-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		procedureScalaMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.ProcedureScala
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ProcedureScala{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ProcedureScala
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ProcedureScala should become Ready")
}

func TestProcedureSQL_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"psql-db", "PSQL_DB", "psql-schema", "PSQL_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	procedureSQLMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier, _ []string) (*snowflake.ProcedureObservation, error) {
		if id.Name() == "MY_PROC_SQL" && created.Load() {
			return procedureObservation("MY_PROC_SQL", "PSQL_DB", "PSQL_SCHEMA"), nil
		}
		return &snowflake.ProcedureObservation{Exists: false}, nil
	})
	procedureSQLMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateProcedureOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestProcedureSQL("test-proc-sql", "MY_PROC_SQL", "psql-db", "psql-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		procedureSQLMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier, _ []string) error { return nil })
		var obj snowplanev1alpha1.ProcedureSQL
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ProcedureSQL{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ProcedureSQL
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ProcedureSQL should become Ready")
}
