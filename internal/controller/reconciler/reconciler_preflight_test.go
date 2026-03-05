package reconciler_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// ---------------------------------------------------------------------------
// Pre-flight integration tests
//
// These tests exercise the auto pre-flight mechanism at the reconciler level.
// They use *Schema (a ScopedResource — database-only scoped) to verify that
// the reconciler automatically checks database existence when raw
// databaseName is used.
//
// Contrast with the unit tests in refresolver/preflight_test.go which cover
// the lower-level PreFlightCheckDatabaseExists / PreFlightCheckSchemaExists
// functions directly.
// ---------------------------------------------------------------------------

// preFlightMockClient is a mock SnowflakeClient with configurable Query
// behaviour so pre-flight checks can simulate various database states.
type preFlightMockClient struct {
	queryFn func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (m *preFlightMockClient) Ping(_ context.Context) error { return nil }
func (m *preFlightMockClient) Close() error                 { return nil }
func (m *preFlightMockClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (m *preFlightMockClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return snowflake.NewErrorRow(fmt.Errorf("mock: no real connection"))
}
func (m *preFlightMockClient) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, query, args...)
	}

	return nil, fmt.Errorf("mock: no real connection")
}
func (m *preFlightMockClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, func(context.Context) {}, fmt.Errorf("mock: no real connection")
}

var _ clientfactory.SnowflakeClient = (*preFlightMockClient)(nil)

// ---------------------------------------------------------------------------
// Minimal Schema adapter (only the 14 required ResourceAdapter methods)
// ---------------------------------------------------------------------------

type mockSchemaAdapter struct {
	observeFn func(ctx context.Context, svc any, id reconciler.Identifier) (*reconciler.Observation[any], error)
	createFn  func(ctx context.Context, svc any, obj *snowplanev1alpha1.Schema, id reconciler.Identifier) error
}

func (a *mockSchemaAdapter) ResourceName() string  { return "schema" }
func (a *mockSchemaAdapter) FinalizerName() string { return "snowplane.test/schema" }
func (a *mockSchemaAdapter) NewObject() *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{}
}
func (a *mockSchemaAdapter) ServiceFromClient(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (any, func(context.Context), error) {
	return "mock-svc", nil, nil
}
func (a *mockSchemaAdapter) BuildIdentifier(_ *snowplanev1alpha1.Schema) (reconciler.Identifier, error) {
	return testID("TEST_DB.TEST_SCHEMA"), nil
}
func (a *mockSchemaAdapter) Observe(ctx context.Context, svc any, id reconciler.Identifier) (*reconciler.Observation[any], error) {
	if a.observeFn != nil {
		return a.observeFn(ctx, svc, id)
	}

	return &reconciler.Observation[any]{Exists: false}, nil
}
func (a *mockSchemaAdapter) Create(ctx context.Context, svc any, obj *snowplanev1alpha1.Schema, id reconciler.Identifier) error {
	if a.createFn != nil {
		return a.createFn(ctx, svc, obj, id)
	}

	return nil
}
func (a *mockSchemaAdapter) Alter(_ context.Context, _ any, _ reconciler.AlterOptions) error {
	return nil
}
func (a *mockSchemaAdapter) Drop(_ context.Context, _ any, _ reconciler.Identifier) error {
	return nil
}
func (a *mockSchemaAdapter) ValidateImmutableFields(_ context.Context, _ *snowplanev1alpha1.Schema) error {
	return nil
}
func (a *mockSchemaAdapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.Schema, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
	return &mockAlterOpts{hasChanges: false}, nil
}
func (a *mockSchemaAdapter) ApplyObservation(_ *snowplanev1alpha1.Schema, _ *reconciler.Observation[any]) {
}
func (a *mockSchemaAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.Schema) []string {
	return []string{"COMMENT"}
}
func (a *mockSchemaAdapter) DetectDrift(_ *snowplanev1alpha1.Schema, _ *reconciler.Observation[any]) *drift.Result {
	return drift.New().Result()
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Schema, any, any] = (*mockSchemaAdapter)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestSchema(dbName *string) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "testschema",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "TEST_SCHEMA",
			DatabaseName: dbName,
		},
	}
}

func buildSchemaReconciler(
	adapter reconciler.ResourceAdapter[*snowplanev1alpha1.Schema, any, any],
	sfClient clientfactory.SnowflakeClient,
	objs ...runtime.Object,
) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, any, any] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Schema{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return sfClient, nil
	})

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, any, any]{
		Client:   c,
		Factory:  factory,
		Recorder: record.NewFakeRecorder(100),
		Adapter:  adapter,
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("Schema"),
	}
}

func schemaReconcileReq() ctrl.Request {
	return testutil.ReconcileReq("testschema", "default")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestPreFlight_DatabaseNotFound verifies that when a ScopedResource uses a
// raw databaseName pointing to a non-existent database, the reconciler sets
// DependencyNotReady condition and returns an error.
func TestPreFlight_DatabaseNotFound(t *testing.T) {
	t.Parallel()

	dbName := "NONEXISTENT_DB"
	schema := newTestSchema(&dbName)
	schema.Finalizers = []string{"snowplane.test/schema"}

	sfClient := &preFlightMockClient{
		queryFn: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return nil, snowflake.ErrObjectNotFound
		},
	}

	r := buildSchemaReconciler(
		&mockSchemaAdapter{},
		sfClient,
		schema,
		testutil.NewTestPC("default"),
		testutil.NewTestSecret("default"),
	)

	_, err := r.Reconcile(context.Background(), schemaReconcileReq())
	require.Error(t, err, "pre-flight should return error for missing database")
	assert.Contains(t, err.Error(), "database not found")

	// Verify DependencyNotReady condition is set.
	var fetched snowplanev1alpha1.Schema
	require.NoError(t, r.Client.Get(context.Background(), schemaReconcileReq().NamespacedName, &fetched))

	ready := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, ready.Reason)
	assert.Contains(t, ready.Message, "pre-flight check failed")
	assert.Contains(t, ready.Message, "database not found")

	synced := conditions.Get(&fetched, snowplanev1alpha1.TypeSynced)
	require.NotNil(t, synced)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, synced.Reason)
}

// TestPreFlight_ConnectionError_Skipped verifies that non-definitive errors
// (connection failures, timeouts) during pre-flight are gracefully skipped,
// allowing the reconciler to proceed with the normal state machine.
func TestPreFlight_ConnectionError_Skipped(t *testing.T) {
	t.Parallel()

	dbName := "MYDB"
	schema := newTestSchema(&dbName)
	schema.Finalizers = []string{"snowplane.test/schema"}

	createCalled := false
	sfClient := &preFlightMockClient{
		queryFn: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			return nil, errors.New("connection refused")
		},
	}

	observeCalls := 0
	adapter := &mockSchemaAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			observeCalls++
			if observeCalls <= 1 {
				return &reconciler.Observation[any]{Exists: false}, nil
			}

			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		createFn: func(_ context.Context, _ any, _ *snowplanev1alpha1.Schema, _ reconciler.Identifier) error {
			createCalled = true
			return nil
		},
	}

	r := buildSchemaReconciler(
		adapter,
		sfClient,
		schema,
		testutil.NewTestPC("default"),
		testutil.NewTestSecret("default"),
	)

	// Pre-flight skips the non-definitive error; reconcile proceeds to create.
	result, err := r.Reconcile(context.Background(), schemaReconcileReq())
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "should requeue after successful create")
	assert.True(t, createCalled, "reconcile should proceed past pre-flight and invoke create")
}

// TestPreFlight_RefBased_Skipped verifies that when a ScopedResource uses a
// databaseRef (CR reference) instead of raw databaseName, the pre-flight
// database existence check is skipped entirely (ref resolution validates
// existence via CR readiness).
func TestPreFlight_RefBased_Skipped(t *testing.T) {
	t.Parallel()

	schema := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "testschema",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "TEST_SCHEMA",
			DatabaseRef:  &snowplanev1alpha1.ObjectReference{Name: "my-db-cr"},
			DatabaseName: nil, // ref-based — no raw name
		},
	}
	schema.Finalizers = []string{"snowplane.test/schema"}

	queryCalled := false
	sfClient := &preFlightMockClient{
		queryFn: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			queryCalled = true
			return nil, snowflake.ErrObjectNotFound
		},
	}

	observeCalls := 0
	adapter := &mockSchemaAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			observeCalls++
			if observeCalls <= 1 {
				return &reconciler.Observation[any]{Exists: false}, nil
			}

			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}

	r := buildSchemaReconciler(
		adapter,
		sfClient,
		schema,
		testutil.NewTestPC("default"),
		testutil.NewTestSecret("default"),
	)

	// Pre-reconcile might fail for ref resolution (Database CR doesn't exist),
	// but pre-flight check should NOT call Query.
	_, _ = r.Reconcile(context.Background(), schemaReconcileReq())

	assert.False(t, queryCalled, "pre-flight should not query Snowflake when databaseRef is set")
}

// TestPreFlight_NonScopedResource_Bypassed verifies that non-ScopedResource
// types (e.g. Database, AccountRole) skip pre-flight entirely. This is already
// implicitly tested by every Database test in reconciler_test.go, but this test
// makes the contract explicit.
func TestPreFlight_NonScopedResource_Bypassed(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}

	queryCalled := false
	sfClient := &preFlightMockClient{
		queryFn: func(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
			queryCalled = true
			return nil, snowflake.ErrObjectNotFound
		},
	}

	// Use Database (non-ScopedResource) via the existing buildTestReconciler.
	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return sfClient, nil
	})

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Database{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(db, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Client:   c,
		Factory:  factory,
		Recorder: record.NewFakeRecorder(100),
		Adapter:  &mockAdapter{},
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("Database"),
	}

	_, _ = r.Reconcile(context.Background(), reconcileReq())

	assert.False(t, queryCalled, "pre-flight should not run for non-ScopedResource types")
}
