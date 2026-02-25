package reconciler_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

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
// Mock Snowflake client (no real connection)
// ---------------------------------------------------------------------------

type mockSnowflakeClient struct {
	withRoleFn func(ctx context.Context, role string) (*snowflake.Client, func(context.Context), error)
}

func (m *mockSnowflakeClient) Ping(_ context.Context) error { return nil }
func (m *mockSnowflakeClient) Close() error                 { return nil }
func (m *mockSnowflakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (m *mockSnowflakeClient) QueryRow(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}
func (m *mockSnowflakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockSnowflakeClient) WithRole(ctx context.Context, role string) (*snowflake.Client, func(context.Context), error) {
	if m.withRoleFn != nil {
		return m.withRoleFn(ctx, role)
	}

	return nil, nil, nil
}

// ---------------------------------------------------------------------------
// Mock AlterOptions
// ---------------------------------------------------------------------------

type mockAlterOpts struct {
	hasChanges bool
}

func (m *mockAlterOpts) HasChanges() bool { return m.hasChanges }

// ---------------------------------------------------------------------------
// Mock adapter using Database as the concrete ManagedResource type
// ---------------------------------------------------------------------------

// testID satisfies reconciler.Identifier for test use.
type testID string

func (id testID) FullyQualifiedName() string { return string(id) }
func (id testID) String() string             { return string(id) }

type mockAdapter struct {
	serviceFromClientFn func(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (any, func(context.Context), error)
	observeFn           func(ctx context.Context, svc any, id reconciler.Identifier) (*reconciler.Observation[any], error)
	createFn            func(ctx context.Context, svc any, obj *snowplanev1alpha1.Database, id reconciler.Identifier) error
	alterFn             func(ctx context.Context, svc any, opts reconciler.AlterOptions) error
	dropFn              func(ctx context.Context, svc any, id reconciler.Identifier) error
	validateImmutableFn func(ctx context.Context, obj *snowplanev1alpha1.Database) error
	buildAlterOptsFn    func(ctx context.Context, obj *snowplanev1alpha1.Database, id reconciler.Identifier, obs *reconciler.Observation[any]) (reconciler.AlterOptions, error)
	preReconcileFn      func(ctx context.Context, obj *snowplanev1alpha1.Database) error
	buildIdentifierFn   func(obj *snowplanev1alpha1.Database) (reconciler.Identifier, error)
	detectDriftFn       func(obj *snowplanev1alpha1.Database, obs *reconciler.Observation[any]) *drift.Result

	applyObservationCalled int
	postCreateCalled       int
	postUpdateCalled       int
	postUpdateAltered      bool
	supportsCreateOrAlter  bool
}

func (m *mockAdapter) ResourceName() string  { return "database" }
func (m *mockAdapter) FinalizerName() string { return "snowplane.test/database" }
func (m *mockAdapter) NewObject() *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{}
}

func (m *mockAdapter) ServiceFromClient(ctx context.Context, sfClient clientfactory.SnowflakeClient, useRole string) (any, func(context.Context), error) {
	if m.serviceFromClientFn != nil {
		return m.serviceFromClientFn(ctx, sfClient, useRole)
	}
	return "mock-svc", nil, nil
}

func (m *mockAdapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.Database) error {
	if m.preReconcileFn != nil {
		return m.preReconcileFn(ctx, obj)
	}
	return nil
}

func (m *mockAdapter) BuildIdentifier(obj *snowplanev1alpha1.Database) (reconciler.Identifier, error) {
	if m.buildIdentifierFn != nil {
		return m.buildIdentifierFn(obj)
	}

	return testID("test-id"), nil
}

func (m *mockAdapter) SetupWatches() reconciler.SetupWatchesFunc { return nil }

func (m *mockAdapter) Observe(ctx context.Context, svc any, id reconciler.Identifier) (*reconciler.Observation[any], error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, svc, id)
	}
	return &reconciler.Observation[any]{Exists: false}, nil
}

func (m *mockAdapter) Create(ctx context.Context, svc any, obj *snowplanev1alpha1.Database, id reconciler.Identifier) error {
	if m.createFn != nil {
		return m.createFn(ctx, svc, obj, id)
	}
	return nil
}

func (m *mockAdapter) Alter(ctx context.Context, svc any, opts reconciler.AlterOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, svc, opts)
	}
	return nil
}

func (m *mockAdapter) Drop(ctx context.Context, svc any, id reconciler.Identifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, svc, id)
	}
	return nil
}

func (m *mockAdapter) ValidateImmutableFields(ctx context.Context, obj *snowplanev1alpha1.Database) error {
	if m.validateImmutableFn != nil {
		return m.validateImmutableFn(ctx, obj)
	}
	return nil
}

func (m *mockAdapter) BuildAlterOptions(ctx context.Context, obj *snowplanev1alpha1.Database, id reconciler.Identifier, obs *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
	if m.buildAlterOptsFn != nil {
		return m.buildAlterOptsFn(ctx, obj, id, obs)
	}
	return &mockAlterOpts{hasChanges: false}, nil
}

func (m *mockAdapter) ApplyObservation(_ *snowplanev1alpha1.Database, _ *reconciler.Observation[any]) {
	m.applyObservationCalled++
}

func (m *mockAdapter) ComputeTrackedParameters(_ *snowplanev1alpha1.Database) []string {
	return []string{"NAME", "COMMENT"}
}

func (m *mockAdapter) DetectDrift(_ *snowplanev1alpha1.Database, obs *reconciler.Observation[any]) *drift.Result {
	if m.detectDriftFn != nil {
		return m.detectDriftFn(nil, obs)
	}
	return drift.New().Result()
}

func (m *mockAdapter) PostCreate(_ *snowplanev1alpha1.Database) {
	m.postCreateCalled++
}

func (m *mockAdapter) PostUpdate(_ *snowplanev1alpha1.Database, altered bool, _ reconciler.AlterOptions) {
	m.postUpdateCalled++
	m.postUpdateAltered = altered
}

func (m *mockAdapter) SupportsCreateOrAlter() bool { return m.supportsCreateOrAlter }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Database, any, any] = (*mockAdapter)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestReconciler(adapter *mockAdapter, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Database{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &mockSnowflakeClient{}, nil
	})

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Client:   c,
		Factory:  factory,
		Recorder: record.NewFakeRecorder(100),
		Adapter:  adapter,
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("Database"),
	}
}

func newTestDB() *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "testdb",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "TESTDB",
		},
	}
}

func reconcileReq() ctrl.Request {
	return testutil.ReconcileReq("testdb", "default")
}

// ---------------------------------------------------------------------------
// Tests: CR lifecycle
// ---------------------------------------------------------------------------

func TestReconcile_CRNotFound_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	r := newTestReconciler(&mockAdapter{})
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_Create_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	observeCalls := 0
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			observeCalls++
			if observeCalls <= 2 {
				return &reconciler.Observation[any]{Exists: false}, nil
			}
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	// First reconcile adds finalizer and requeues.
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	// Second reconcile executes create.
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "should requeue after create")
}

func TestReconcile_Create_PostCreateHook(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	callCount := 0
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			callCount++
			if callCount <= 1 {
				return &reconciler.Observation[any]{Exists: false}, nil
			}
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	// First: add finalizer.
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	// Second: create + post-create observe.
	_, err = r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, 1, adapter.postCreateCalled, "PostCreate hook should be called once")
}

// ---------------------------------------------------------------------------
// Tests: Update path
// ---------------------------------------------------------------------------

func TestReconcile_Update_NoChanges(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, 1, adapter.postUpdateCalled)
	assert.False(t, adapter.postUpdateAltered, "no ALTER should have happened")
}

func TestReconcile_Update_WithChanges(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	alterCalled := false
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter should have been called")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.True(t, adapter.postUpdateAltered, "PostUpdate should know alter happened")
}

// ---------------------------------------------------------------------------
// Tests: Delete path
// ---------------------------------------------------------------------------

func TestReconcile_Delete_WithDeletionPolicyDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	now := metav1.Now()
	db.DeletionTimestamp = &now
	dropCalled := false
	adapter := &mockAdapter{
		dropFn: func(_ context.Context, _ any, _ reconciler.Identifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, dropCalled, "Drop should have been called")
}

func TestReconcile_Delete_OrphanPolicy(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	db.DeletionTimestamp = &now
	dropCalled := false
	adapter := &mockAdapter{
		dropFn: func(_ context.Context, _ any, _ reconciler.Identifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.False(t, dropCalled, "Drop should NOT be called with Orphan policy")
}

// ---------------------------------------------------------------------------
// Tests: Immutable field violation (terminal — no requeue)
// ---------------------------------------------------------------------------

func TestReconcile_ImmutableField_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		validateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.Database) error {
			return errors.New("spec.name is immutable")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err, "immutable violation should return nil error (terminal)")
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")
}

// ---------------------------------------------------------------------------
// Tests: PreReconcile failure
// ---------------------------------------------------------------------------

func TestReconcile_PreReconcile_Failure_SetsCondition(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		preReconcileFn: func(_ context.Context, _ *snowplanev1alpha1.Database) error {
			return errors.New("database ref not ready")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter, "should let controller-runtime handle backoff")
}

// ---------------------------------------------------------------------------
// Tests: Validation failure (terminal — no requeue)
// ---------------------------------------------------------------------------

func TestReconcile_ValidationFailure_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Spec.Name = "" // Invalid — ValidateSpec returns error.
	adapter := &mockAdapter{}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err, "validation failure should not return error")
	assert.Equal(t, ctrl.Result{}, result, "validation failure should not requeue")
}

// ---------------------------------------------------------------------------
// Tests: Observe errors
// ---------------------------------------------------------------------------

func TestReconcile_ObserveError_ReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return nil, errors.New("snowflake connection failed")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err, "observe error should be returned")
	assert.Contains(t, err.Error(), "snowflake connection failed")
}

// ---------------------------------------------------------------------------
// Tests: Create errors
// ---------------------------------------------------------------------------

func TestReconcile_CreateError_ReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ any, _ *snowplanev1alpha1.Database, _ reconciler.Identifier) error {
			return errors.New("create failed")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	// First: finalizer.
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	// Second: create fails.
	_, err = r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

// ---------------------------------------------------------------------------
// Tests: BuildIdentifier error
// ---------------------------------------------------------------------------

func TestReconcile_BuildIdentifierError_ReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		buildIdentifierFn: func(_ *snowplanev1alpha1.Database) (reconciler.Identifier, error) {
			return nil, errors.New("invalid object type")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "building identifier")
	assert.Contains(t, err.Error(), "invalid object type")
}

// ---------------------------------------------------------------------------
// Tests: Detect-only drift policy
// ---------------------------------------------------------------------------

func TestReconcile_DriftDetection_DetectOnlyPolicy(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationDriftPolicy: drift.DriftPolicyDetectOnly,
	}
	// Simulate spec-unchanged by pre-setting the hash.
	hash, err := db.ComputeSpecHash()
	require.NoError(t, err)
	db.Status.LastAppliedSpecHash = hash
	alterCalled := false
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

// ---------------------------------------------------------------------------
// Tests: Immutable-only drift skips ALTER
// ---------------------------------------------------------------------------

func TestReconcile_ImmutableOnlyDrift_SkipsAlter(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1

	// Simulate spec-unchanged by pre-setting the hash.
	hash, err := db.ComputeSpecHash()
	require.NoError(t, err)
	db.Status.LastAppliedSpecHash = hash

	alterCalled := false

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
		detectDriftFn: func(_ *snowplanev1alpha1.Database, _ *reconciler.Observation[any]) *drift.Result {
			// Return immutable-only drift — OWNER changed externally.
			return drift.New().
				CompareStringValue("OWNER", "SYSADMIN", "ACCOUNTADMIN", true).
				Result()
		},
	}

	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called for immutable-only drift")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_MixedDrift_StillAlters(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1

	// Simulate spec-unchanged by pre-setting the hash.
	hash, err := db.ComputeSpecHash()
	require.NoError(t, err)
	db.Status.LastAppliedSpecHash = hash

	alterCalled := false

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
		detectDriftFn: func(_ *snowplanev1alpha1.Database, _ *reconciler.Observation[any]) *drift.Result {
			// Return mixed drift: OWNER (immutable) + COMMENT (mutable).
			return drift.New().
				CompareStringValue("OWNER", "SYSADMIN", "ACCOUNTADMIN", true).
				CompareStringValue("COMMENT", "expected", "actual", false).
				Result()
		},
	}

	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err = r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter SHOULD be called when mutable fields also drifted")
}

// ---------------------------------------------------------------------------
// Tests: WithRequeueInterval
// ---------------------------------------------------------------------------

func TestWithRequeueInterval(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := newTestReconciler(adapter)
	r2 := r.WithRequeueInterval(10 * time.Minute)
	assert.NotNil(t, r2, "WithRequeueInterval should return non-nil")
	assert.Equal(t, r, r2, "WithRequeueInterval should return the same instance")
}

func TestDefaultRequeueInterval(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5*time.Minute, reconciler.DefaultRequeueInterval)
}

// ---------------------------------------------------------------------------
// Tests: WithUseRole (now type-safe — compiler prevents invalid types)
// ---------------------------------------------------------------------------

func TestWithUseRole_NoRole(t *testing.T) {
	t.Parallel()
	client := &mockSnowflakeClient{}
	executor, cleanup, err := reconciler.WithUseRole(context.Background(), client, "")
	require.NoError(t, err)
	assert.Nil(t, cleanup)
	// When no use role is specified, the original client is returned.
	assert.Same(t, client, executor)
}

func TestWithUseRole_RoleSwitchFailure_Terminal(t *testing.T) {
	t.Parallel()
	client := &mockSnowflakeClient{
		withRoleFn: func(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
			return nil, nil, snowflake.ErrRoleSwitchFailed
		},
	}
	_, _, err := reconciler.WithUseRole(context.Background(), client, "ENGINEER")
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err), "role switch failure should be wrapped as terminal")
	assert.Contains(t, err.Error(), "GRANT ROLE ENGINEER TO USER", "should suggest fix")
}

func TestWithUseRole_ConnectionError_Recoverable(t *testing.T) {
	t.Parallel()
	client := &mockSnowflakeClient{
		withRoleFn: func(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
			return nil, nil, errors.New("connection reset by peer")
		},
	}
	_, _, err := reconciler.WithUseRole(context.Background(), client, "ENGINEER")
	require.Error(t, err)
	assert.False(t, snowflake.IsTerminalError(err), "connection error should not be terminal")
	assert.Contains(t, err.Error(), "connection reset by peer")
}

// ---------------------------------------------------------------------------
// Tests: ProviderConfig resolution failure
// ---------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	// No ProviderConfig object in the fake client.
	r := newTestReconciler(&mockAdapter{}, db)
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProviderConfig")
}

// ---------------------------------------------------------------------------
// Tests: Delete with provider resolution failure (should still remove finalizer)
// ---------------------------------------------------------------------------

func TestReconcile_Delete_ProviderNotFound_RemovesFinalizer(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	now := metav1.Now()
	db.DeletionTimestamp = &now
	// No ProviderConfig — forces resolution failure during deletion.
	r := newTestReconciler(&mockAdapter{}, db)
	_, err := r.Reconcile(context.Background(), reconcileReq())
	// Reconciler should handle missing provider gracefully during deletion.
	// It logs a warning and removes the finalizer to unblock deletion.
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests: ApplyObservation is called
// ---------------------------------------------------------------------------

func TestReconcile_ApplyObservation_Called(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, adapter.applyObservationCalled, 1, "ApplyObservation should be called")
}

// ---------------------------------------------------------------------------
// Tests: ServiceFromClient error (F12)
// ---------------------------------------------------------------------------

func TestReconcile_ServiceFromClientError_Recoverable(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		serviceFromClientFn: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (any, func(context.Context), error) {
			return nil, nil, errors.New("service creation failed")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service creation failed")

	// Verify condition is recoverable, not terminal.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))

	ready := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonReconcileError, ready.Reason, "plain error should be recoverable")
}

func TestReconcile_ServiceFromClientError_TerminalRoleSwitch(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		serviceFromClientFn: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (any, func(context.Context), error) {
			// Simulate what WithUseRole returns for role switch failures.
			return nil, nil, snowflake.NewTerminalError(errors.New("USE ROLE \"ENGINEER\" failed"))
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))

	// Verify condition is terminal.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))

	ready := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionFalse, ready.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonTerminalError, ready.Reason, "role switch failure should be terminal")
}

// ---------------------------------------------------------------------------
// Tests: CREATE OR ALTER update path (F17)
// ---------------------------------------------------------------------------

func TestReconcile_CreateOrAlter_UpdateUsesCreate(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	// Trigger spec change so alterOpts.HasChanges() is true.
	db.Status.LastAppliedSpecHash = "old-hash"

	createCalled := false
	alterCalled := false
	adapter := &mockAdapter{
		supportsCreateOrAlter: true,
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		createFn: func(_ context.Context, _ any, _ *snowplanev1alpha1.Database, _ reconciler.Identifier) error {
			createCalled = true
			return nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, createCalled, "CREATE OR ALTER should call Create, not Alter")
	assert.False(t, alterCalled, "Alter should NOT be called when CREATE OR ALTER is enabled")
}

func TestReconcile_CreateOrAlter_UnsupportedType_FallsBackToAlter(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	db.Status.LastAppliedSpecHash = "old-hash"

	alterCalled := false
	adapter := &mockAdapter{
		supportsCreateOrAlter: false, // Not supported
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, alterCalled, "should fall back to ALTER when resource doesn't support CREATE OR ALTER")
}

// ---------------------------------------------------------------------------
// Tests: BuildAlterOptions error (F13)
// ---------------------------------------------------------------------------

func TestReconcile_BuildAlterOptionsError_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return nil, errors.New("cannot compute diff")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot compute diff")
}

// ---------------------------------------------------------------------------
// Tests: Alter failure — terminal and non-terminal (F14)
// ---------------------------------------------------------------------------

func TestReconcile_AlterError_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			return snowflake.NewTerminalError(errors.New("invalid SQL"))
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))
}

func TestReconcile_AlterError_Recoverable(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			return errors.New("transient failure")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.False(t, snowflake.IsTerminalError(err))
}

// ---------------------------------------------------------------------------
// Tests: Post-create observe failure (F15)
// ---------------------------------------------------------------------------

func TestReconcile_PostCreateObserve_NotExists(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			// Always returns Exists: false — resource not yet observable after create.
			return &reconciler.Observation[any]{Exists: false}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	// First: finalizer.
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	// Second: create succeeds but post-create observe returns Exists: false.
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter, "should requeue quickly when not yet observable")
}

// ---------------------------------------------------------------------------
// Tests: Ownership drift condition (F16)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Tests: Terminal create error classification (F17)
// ---------------------------------------------------------------------------

func TestReconcile_CreateError_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ any, _ *snowplanev1alpha1.Database, _ reconciler.Identifier) error {
			return snowflake.NewTerminalError(errors.New("invalid identifier"))
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	// First: finalizer.
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	// Second: create fails with terminal error.
	_, err = r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))
}

// ---------------------------------------------------------------------------
// Tests: Delete with Drop error (F18)
// ---------------------------------------------------------------------------

func TestReconcile_Delete_DropError_ReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	now := metav1.Now()
	db.DeletionTimestamp = &now
	adapter := &mockAdapter{
		dropFn: func(_ context.Context, _ any, _ reconciler.Identifier) error {
			return errors.New("permission denied")
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// ---------------------------------------------------------------------------
// Tests: WithRequeueInterval is used in reconcile result (F19)
// ---------------------------------------------------------------------------

func TestWithRequeueInterval_UsedInResult(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	customInterval := 10 * time.Minute
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	r.WithRequeueInterval(customInterval)
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, customInterval, result.RequeueAfter, "custom requeue interval should be used")
}

// ---------------------------------------------------------------------------
// Tests: Adoption — resource already exists, first reconciliation (F20)
// ---------------------------------------------------------------------------

func TestReconcile_Adoption_ExistingResource_NoAnnotation_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err, "Terminal error should not return an error")
	assert.Equal(t, time.Duration(0), result.RequeueAfter, "should not requeue for terminal")

	// Re-read the object from the fake client to check conditions.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))
	tc := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)
	assert.Contains(t, tc.Message, "already exists")
}

func TestReconcile_Adoption_ExistingResource_FailIfExists_Terminal(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAdoptionPolicy: snowplanev1alpha1.AdoptionPolicyFailIfExists,
	}
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))
	tc := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)
}

func TestReconcile_Adoption_Adopt_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAdoptionPolicy: snowplanev1alpha1.AdoptionPolicyAdopt,
	}
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter, "should requeue normally after adoption")

	// Verify ApplyObservation and PostCreate were called.
	assert.GreaterOrEqual(t, adapter.applyObservationCalled, 1)
	assert.Equal(t, 1, adapter.postCreateCalled)

	// Re-read from fake client to check conditions.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))
	assert.True(t, conditions.IsTrue(&fetched, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(&fetched, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, "true", fetched.Annotations[snowplanev1alpha1.AnnotationLateInitialized])
	assert.False(t, conditions.IsTerminal(&fetched))
}

func TestReconcile_Adoption_SecondReconcile_SkipsAdoptionCheck(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1 // Already reconciled.
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter, "should go through normal update path")
	// Should NOT be in terminal state.
	assert.False(t, conditions.IsTerminal(db))
}

func TestReconcile_Adoption_InvalidAnnotation_DefaultsToReject(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationAdoptionPolicy: "adopt-typo", // invalid value
	}
	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err, "Terminal error should not return an error")
	assert.Equal(t, time.Duration(0), result.RequeueAfter, "should not requeue for terminal")

	// Invalid annotation value → treated as fail-if-exists → Terminal.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))
	tc := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)
	assert.Contains(t, tc.Message, "already exists")
}

// --------------------------------------------------------------------------
// Maturity Gating Tests
// --------------------------------------------------------------------------

func TestReconciler_MaturityDefaults(t *testing.T) {
	t.Parallel()
	// A bare GenericReconciler should default to alpha maturity.
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: &mockAdapter{},
	}
	// WithMaturity returns the same reconciler (fluent API).
	same := r.WithMaturity("beta")
	assert.Same(t, r, same, "WithMaturity should return the same reconciler")
	same2 := r.WithAlphaEnabled(true)
	assert.Same(t, r, same2, "WithAlphaEnabled should return the same reconciler")
}

func TestReconciler_SetupWithManager_SkipsAlphaWhenDisabled(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: adapter,
	}
	// Alpha maturity (default) + alpha disabled → should return nil without error.
	// This returns before touching the manager, so nil manager is safe.
	r.WithMaturity("alpha").WithAlphaEnabled(false)
	err := r.SetupWithManager(nil, 1)
	assert.NoError(t, err, "SetupWithManager should skip alpha controllers when disabled")
}

func TestReconciler_SetupWithManager_DoesNotSkipBetaWhenAlphaDisabled(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: adapter,
	}
	// Beta maturity + alpha disabled → should try to register (will fail with nil manager).
	r.WithMaturity("beta").WithAlphaEnabled(false)
	err := r.SetupWithManager(nil, 1)
	// Should fail because it tries to use the nil manager, proving it wasn't skipped.
	assert.Error(t, err, "SetupWithManager should attempt registration for beta controllers")
}

func TestReconciler_SetupWithManager_DoesNotSkipStableWhenAlphaDisabled(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: adapter,
	}
	// Stable maturity + alpha disabled → should try to register.
	r.WithMaturity("stable").WithAlphaEnabled(false)
	err := r.SetupWithManager(nil, 1)
	assert.Error(t, err, "SetupWithManager should attempt registration for stable controllers")
}

func TestReconciler_WithDisabled_FluentAPI(t *testing.T) {
	t.Parallel()
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: &mockAdapter{},
	}
	same := r.WithDisabled(true)
	assert.Same(t, r, same, "WithDisabled should return the same reconciler")
}

func TestReconciler_SetupWithManager_SkipsExplicitlyDisabled(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: adapter,
	}
	// Explicitly disabled → should return nil without error, even with alpha enabled.
	r.WithMaturity("alpha").WithAlphaEnabled(true).WithDisabled(true)
	err := r.SetupWithManager(nil, 1)
	assert.NoError(t, err, "SetupWithManager should skip explicitly disabled controllers")
}

func TestReconciler_SetupWithManager_DisabledTakesPrecedenceOverMaturity(t *testing.T) {
	t.Parallel()
	adapter := &mockAdapter{}
	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Adapter: adapter,
	}
	// Stable maturity + disabled → should still skip.
	r.WithMaturity("stable").WithAlphaEnabled(true).WithDisabled(true)
	err := r.SetupWithManager(nil, 1)
	assert.NoError(t, err, "WithDisabled should take precedence over maturity")
}

// ---------------------------------------------------------------------------
// Tests: ShouldSkipImmutableValidation
// ---------------------------------------------------------------------------

func TestShouldSkipImmutableValidation_ObservedGenerationZero(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Status.ObservedGeneration = 0

	assert.True(t, reconciler.ShouldSkipImmutableValidation(db),
		"should skip when ObservedGeneration is 0 (first reconcile)")
}

func TestShouldSkipImmutableValidation_ForceNewActive(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Status.ObservedGeneration = 1
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "true",
	}

	assert.True(t, reconciler.ShouldSkipImmutableValidation(db),
		"should skip when ForceNew annotation is active")
}

func TestShouldSkipImmutableValidation_NormalCase(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Status.ObservedGeneration = 1

	assert.False(t, reconciler.ShouldSkipImmutableValidation(db),
		"should not skip on normal reconciliation")
}

func TestShouldSkipImmutableValidation_ForceNewFalse(t *testing.T) {
	t.Parallel()

	db := newTestDB()
	db.Status.ObservedGeneration = 1
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "false",
	}

	assert.False(t, reconciler.ShouldSkipImmutableValidation(db),
		"should not skip when ForceNew is explicitly false")
}

// ---------------------------------------------------------------------------
// Tests: isCreateOrAlterUnsupported table-driven (M-2)
// ---------------------------------------------------------------------------

func TestIsCreateOrAlterUnsupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("connection reset"), false},
		{"permission denied", errors.New("insufficient privileges"), false},
		{"unsupported keyword", errors.New("SQL compilation error: UNSUPPORTED feature"), true},
		{"unexpected OR", errors.New("SQL compilation error: unexpected 'OR'"), true},
		{"syntax error", errors.New("SQL compilation error: syntax error"), true},
		{"error code 002032", errors.New("002032 (42601): SQL compilation error"), true},
		{"case insensitive unsupported", errors.New("unsupported statement type"), true},
		{"case insensitive syntax", errors.New("Syntax Error near 'CREATE'"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, reconciler.IsCreateOrAlterUnsupported(tt.err))
		})
	}
}

// ---------------------------------------------------------------------------
// Tests: CREATE OR ALTER fallback to ALTER on syntax error (M-1)
// ---------------------------------------------------------------------------

func TestReconcile_CreateOrAlter_SyntaxError_FallsBackToAlter(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	db.Status.LastAppliedSpecHash = "old-hash"

	createCalled := false
	alterCalled := false
	adapter := &mockAdapter{
		supportsCreateOrAlter: true,
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
		buildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.Database, _ reconciler.Identifier, _ *reconciler.Observation[any]) (reconciler.AlterOptions, error) {
			return &mockAlterOpts{hasChanges: true}, nil
		},
		createFn: func(_ context.Context, _ any, _ *snowplanev1alpha1.Database, _ reconciler.Identifier) error {
			createCalled = true
			return errors.New("SQL compilation error: syntax error line 1 at position 7 unexpected 'OR'")
		},
		alterFn: func(_ context.Context, _ any, _ reconciler.AlterOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, createCalled, "CREATE OR ALTER should be attempted first")
	assert.True(t, alterCalled, "should fall back to ALTER after syntax error")
	assert.True(t, result.RequeueAfter > 0, "should requeue after successful fallback")

	// Verify the fallback warning event was emitted.
	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	var foundFallback bool
	for _, e := range events {
		if strings.Contains(e, snowplanev1alpha1.ReasonCreateOrAlterFallback) {
			foundFallback = true
		}
	}
	assert.True(t, foundFallback, "should emit CreateOrAlterFallback warning event")
}

// ---------------------------------------------------------------------------
// Tests: Post-crash create recovery (M-3)
// ---------------------------------------------------------------------------

func TestReconcile_PostCrashCreate_Recovery(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 0 // Never successfully reconciled.
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationCreationInitiated: "true",
	}

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "should requeue after post-crash recovery")

	// PostCreate hook should have been called.
	assert.Equal(t, 1, adapter.postCreateCalled, "PostCreate should be called during recovery")

	// ApplyObservation should have been called.
	assert.GreaterOrEqual(t, adapter.applyObservationCalled, 1, "ApplyObservation should be called")

	// Re-read the object to check conditions and annotations.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched))

	// Should be Ready after recovery.
	ready := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, ready)
	assert.Equal(t, metav1.ConditionTrue, ready.Status)
	assert.Contains(t, ready.Message, "recovered")

	// Creation-initiated annotation should be cleared.
	assert.Empty(t, fetched.Annotations[snowplanev1alpha1.AnnotationCreationInitiated],
		"creation-initiated annotation should be cleared after recovery")
}

// ---------------------------------------------------------------------------
// Tests: Orphan deletion — condition and event assertions (M-4)
// ---------------------------------------------------------------------------

func TestReconcile_Delete_OrphanPolicy_ConditionsAndEvents(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	db.DeletionTimestamp = &now

	dropCalled := false
	adapter := &mockAdapter{
		dropFn: func(_ context.Context, _ any, _ reconciler.Identifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)
	assert.False(t, dropCalled, "Drop should NOT be called with Orphan policy")

	// Verify orphan event was emitted.
	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	var foundOrphan bool
	for _, e := range events {
		if strings.Contains(e, "orphaned") {
			foundOrphan = true
		}
	}
	assert.True(t, foundOrphan, "should emit orphan event")

	// Object should be fully deleted (finalizer removed + DeletionTimestamp set
	// means the fake client garbage-collects it).
	var fetched snowplanev1alpha1.Database
	err = r.Client.Get(context.Background(), reconcileReq().NamespacedName, &fetched)
	assert.Error(t, err, "object should be deleted after finalizer removal")
}

// ---------------------------------------------------------------------------
// Tests: ForceNew warning event (M-5)
// ---------------------------------------------------------------------------

func TestReconcile_ForceNew_EmitsWarningEvent(t *testing.T) {
	t.Parallel()
	db := newTestDB()
	db.Finalizers = []string{"snowplane.test/database"}
	db.Status.ObservedGeneration = 1
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "true",
	}

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}
	r := newTestReconciler(adapter, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), reconcileReq())
	require.NoError(t, err)

	// Verify ForceNewActive warning event was emitted.
	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	var foundForceNew bool
	for _, e := range events {
		if strings.Contains(e, snowplanev1alpha1.ReasonForceNewActive) {
			foundForceNew = true
		}
	}
	assert.True(t, foundForceNew, "should emit ForceNewActive warning event")
}
