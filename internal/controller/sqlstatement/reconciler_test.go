package sqlstatement

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	sqlstmtclient "github.com/hupe1980/snowplane/internal/clients/snowflake/sqlstatement"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

// --- mock service ---

type mockService struct {
	executeFn func(ctx context.Context, sql string) error
	revertFn  func(ctx context.Context, sql string) error
	observeFn func(ctx context.Context, sql string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error)
}

func (m *mockService) Execute(ctx context.Context, sql string) error {
	if m.executeFn != nil {
		return m.executeFn(ctx, sql)
	}

	return nil
}

func (m *mockService) Revert(ctx context.Context, sql string) error {
	if m.revertFn != nil {
		return m.revertFn(ctx, sql)
	}

	return nil
}

func (m *mockService) Observe(ctx context.Context, sql string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, sql, exps)
	}

	return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
}

// --- helpers ---

func ptr(s string) *string { return &s }

func newTestSQLStatement(name, namespace string) *snowplanev1alpha1.SQLStatement {
	return &snowplanev1alpha1.SQLStatement{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SQLStatementSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{
					Name: "default-pc",
				},
			},
			Execute: "CREATE TABLE IF NOT EXISTS test_table (id INT)",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...client.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation] {
	scheme := testutil.TestScheme()

	runtimeObjs := make([]runtime.Object, len(objs))
	for i, obj := range objs {
		runtimeObjs[i] = obj
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(runtimeObjs...).
		WithStatusSubresource(&snowplanev1alpha1.SQLStatement{}).
		Build()

	sf := func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, func(context.Context) {}, nil
	}

	r := NewReconcilerWithServiceFactory(
		c,
		testutil.NewTestClientFactory(),
		record.NewFakeRecorder(16),
		nil,
		nil,
		sf,
	)

	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SQLStatement")

	return r
}

// --- Standard Suite ---

func TestReconcile_StandardSuite(t *testing.T) {
	t.Parallel()

	testutil.ReconcileSuiteConfig{
		NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
			scheme := testutil.TestScheme()

			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				WithStatusSubresource(&snowplanev1alpha1.SQLStatement{}).
				Build()

			mock := &mockService{}
			sf := func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, func(context.Context) {}, nil
			}

			r := NewReconcilerWithServiceFactory(
				c,
				testutil.NewTestClientFactory(),
				record.NewFakeRecorder(16),
				nil,
				nil,
				sf,
			)

			r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SQLStatement")

			return testutil.ReconcilerSetup{
				Reconciler: r,
				Client:     c,
			}
		},
		NewFixture: func(name, ns string) client.Object {
			return newTestSQLStatement(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SQLStatement{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --- Create Flow ---

func TestReconcile_Create_HappyPath(t *testing.T) {
	t.Parallel()

	var executedSQL string

	mock := &mockService{
		executeFn: func(_ context.Context, sql string) error {
			executedSQL = sql
			return nil
		},
	}

	stmt := newTestSQLStatement("test-create", "default")
	stmt.Finalizers = []string{finalizerName}
	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-create", "default"))
	require.NoError(t, err)

	assert.Equal(t, "CREATE TABLE IF NOT EXISTS test_table (id INT)", executedSQL)

	// Without observe SQL, the post-create verify always reports "not yet
	// observable", but the execute itself succeeded.
	var updated snowplanev1alpha1.SQLStatement
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-create", Namespace: "default"}, &updated))

	assert.NotEmpty(t, updated.Status.ExecuteHash)

	// With observe SQL, the reconciler can verify the create and set Ready.
	assert.Equal(t, 5*time.Second, res.RequeueAfter, "without observe SQL, requeues for post-create check")
}

func TestReconcile_Create_WithObserve(t *testing.T) {
	t.Parallel()

	var observedSQL string

	mock := &mockService{
		observeFn: func() func(ctx context.Context, sql string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			call := 0
			return func(_ context.Context, sql string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
				call++
				if call == 1 {
					observedSQL = sql
					return &sqlstmtclient.Observation{Exists: false}, nil
				}

				return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
			}
		}(),
	}

	stmt := newTestSQLStatement("test-observe", "default")
	stmt.Finalizers = []string{finalizerName}
	stmt.Spec.Observe = ptr("SELECT * FROM test_table")

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-observe", "default"))
	require.NoError(t, err)

	assert.Equal(t, "SELECT * FROM test_table", observedSQL)
}

func TestReconcile_Create_ExecuteFails(t *testing.T) {
	t.Parallel()

	mock := &mockService{
		observeFn: func(_ context.Context, _ string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			return &sqlstmtclient.Observation{Exists: false}, nil
		},
		executeFn: func(_ context.Context, _ string) error {
			return assert.AnError
		},
	}

	stmt := newTestSQLStatement("test-fail", "default")
	stmt.Finalizers = []string{finalizerName}
	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-fail", "default"))
	require.Error(t, err)
}

func TestReconcile_Create_AlreadyExists(t *testing.T) {
	t.Parallel()

	var executeCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
		},
		executeFn: func(_ context.Context, _ string) error {
			executeCalled = true
			return nil
		},
	}

	stmt := newTestSQLStatement("test-exists", "default")
	stmt.Finalizers = []string{finalizerName}
	stmt.Spec.Observe = ptr("SELECT 1 FROM test_table")
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL(stmt.Spec.Execute)
	stmt.Status.ObservedGeneration = 1 // Already reconciled — skip adoption path.
	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-exists", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, res.RequeueAfter)

	// Execute should NOT be called since the resource already exists.
	assert.False(t, executeCalled)
}

// --- Delete Flow ---

func TestReconcile_Delete_WithRevert(t *testing.T) {
	t.Parallel()

	var revertedSQL string

	mock := &mockService{
		revertFn: func(_ context.Context, sql string) error {
			revertedSQL = sql
			return nil
		},
	}

	now := metav1.Now()

	stmt := newTestSQLStatement("test-delete", "default")
	stmt.DeletionTimestamp = &now
	stmt.Finalizers = []string{finalizerName}
	stmt.Spec.Revert = ptr("DROP TABLE test_table")

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-delete", "default"))
	require.NoError(t, err)

	assert.Equal(t, "DROP TABLE test_table", revertedSQL)
}

func TestReconcile_Delete_NoRevert(t *testing.T) {
	t.Parallel()

	var revertCalled bool

	mock := &mockService{
		revertFn: func(_ context.Context, _ string) error {
			revertCalled = true
			return nil
		},
	}

	now := metav1.Now()

	stmt := newTestSQLStatement("test-delete-norevert", "default")
	stmt.DeletionTimestamp = &now
	stmt.Finalizers = []string{finalizerName}
	// No revert SQL.

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-delete-norevert", "default"))
	require.NoError(t, err)
	assert.False(t, revertCalled)
}

func TestReconcile_Delete_RevertFails(t *testing.T) {
	t.Parallel()

	mock := &mockService{
		revertFn: func(_ context.Context, _ string) error {
			return assert.AnError
		},
	}

	now := metav1.Now()

	stmt := newTestSQLStatement("test-delete-fail", "default")
	stmt.DeletionTimestamp = &now
	stmt.Finalizers = []string{finalizerName}
	stmt.Spec.Revert = ptr("DROP TABLE test_table")

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-delete-fail", "default"))
	require.Error(t, err)
}

// --- Observe & Drift ---

func TestReconcile_Observe_WithExpectations_Matched(t *testing.T) {
	t.Parallel()

	mock := &mockService{
		observeFn: func(_ context.Context, _ string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			require.Len(t, exps, 1)
			assert.Equal(t, "STATUS", exps[0].Column)
			assert.Equal(t, "active", exps[0].Value)

			return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
		},
	}

	stmt := newTestSQLStatement("test-expect", "default")
	stmt.Spec.Observe = ptr("SELECT status FROM mytable")
	stmt.Spec.ObserveExpect = []snowplanev1alpha1.SQLStatementExpectation{
		{Column: "STATUS", Value: "active"},
	}
	stmt.Finalizers = []string{finalizerName}
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL(stmt.Spec.Execute)
	stmt.Status.ObservedGeneration = 1 // Simulate already-reconciled resource.

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-expect", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, res.RequeueAfter)

	var updated snowplanev1alpha1.SQLStatement
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-expect", Namespace: "default"}, &updated))

	require.NotNil(t, updated.Status.ObserveResult)
	assert.True(t, updated.Status.ObserveResult.Matched)
	assert.Equal(t, int32(1), updated.Status.ObserveResult.RowCount)
}

func TestReconcile_Observe_NoObserveSQL(t *testing.T) {
	t.Parallel()

	var executeCalled bool

	mock := &mockService{
		observeFn: func() func(ctx context.Context, sql string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			call := 0
			return func(_ context.Context, _ string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
				call++
				if call == 1 {
					return &sqlstmtclient.Observation{Exists: false}, nil
				}

				return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
			}
		}(),
		executeFn: func(_ context.Context, _ string) error {
			executeCalled = true
			return nil
		},
	}

	stmt := newTestSQLStatement("test-no-observe", "default")
	stmt.Finalizers = []string{finalizerName}
	// No Observe SQL — should always enter create path.

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-no-observe", "default"))
	require.NoError(t, err)
	assert.True(t, executeCalled)
}

// --- Immutable Validation ---

func TestValidateImmutableFields_NoChange(t *testing.T) {
	t.Parallel()

	stmt := newTestSQLStatement("test-immutable", "default")
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL(stmt.Spec.Execute)

	err := validateImmutableFields(context.Background(), stmt)
	assert.NoError(t, err)
}

func TestValidateImmutableFields_ExecuteChanged(t *testing.T) {
	t.Parallel()

	stmt := newTestSQLStatement("test-immutable-changed", "default")
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL("ORIGINAL SQL")
	stmt.Status.ObservedGeneration = 1 // Must be >0 so validation is not skipped.

	err := validateImmutableFields(context.Background(), stmt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.execute is immutable")
}

func TestValidateImmutableFields_EmptyHash(t *testing.T) {
	t.Parallel()

	stmt := newTestSQLStatement("test-immutable-empty", "default")
	// Empty hash means first reconcile — skip validation.

	err := validateImmutableFields(context.Background(), stmt)
	assert.NoError(t, err)
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	stmt := newTestSQLStatement("test-immutable-force", "default")
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL("ORIGINAL SQL")
	stmt.Annotations = map[string]string{
		"snowplane.hupe1980.github.io/force-new": "true",
	}

	err := validateImmutableFields(context.Background(), stmt)
	assert.NoError(t, err) // Should skip validation with force-new annotation.
}

// --- Identifier ---

func TestSQLStatementIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	id := sqlStatementIdentifier{name: "my-statement", executeHash: "somehash"}
	assert.Equal(t, "my-statement", id.FullyQualifiedName())
	assert.Equal(t, "my-statement", id.String())
}

// --- AlterOptions ---

func TestNoopAlterOptions_HasChanges(t *testing.T) {
	t.Parallel()

	opts := &noopAlterOptions{}
	assert.False(t, opts.HasChanges(), "noopAlterOptions should never report changes")
}

// --- Identifier ---

func TestSQLStatementIdentifier_CarriesExecuteHash(t *testing.T) {
	t.Parallel()

	id := sqlStatementIdentifier{
		name:        "my-statement",
		executeHash: "abc123",
	}
	assert.Equal(t, "my-statement", id.FullyQualifiedName())
	assert.Equal(t, "abc123", id.executeHash)
}

// --- No-Observe Idempotent Fix ---

func TestReconcile_NoObserve_AlreadyExecuted_SkipsReExecution(t *testing.T) {
	t.Parallel()

	var executeCalled bool

	mock := &mockService{
		executeFn: func(_ context.Context, _ string) error {
			executeCalled = true
			return nil
		},
	}

	stmt := newTestSQLStatement("test-no-reexec", "default")
	stmt.Finalizers = []string{finalizerName}
	stmt.Status.ExecuteHash = sqlstmtclient.HashSQL(stmt.Spec.Execute)
	stmt.Status.ObservedGeneration = 1 // Already reconciled.

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-no-reexec", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, res.RequeueAfter)
	assert.False(t, executeCalled, "execute must NOT be called when already executed and no observe SQL")
}

func TestReconcile_NoObserve_FirstExecution_ExecutesSQL(t *testing.T) {
	t.Parallel()

	var executedSQL string

	mock := &mockService{
		executeFn: func(_ context.Context, sql string) error {
			executedSQL = sql
			return nil
		},
	}

	stmt := newTestSQLStatement("test-first-exec", "default")
	stmt.Finalizers = []string{finalizerName}
	// No executeHash → first execution.

	r := newTestReconciler(mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-first-exec", "default"))
	require.NoError(t, err)
	assert.Equal(t, "CREATE TABLE IF NOT EXISTS test_table (id INT)", executedSQL)
}

// --- specExpectationsToClient ---

func TestSpecExpectationsToClient(t *testing.T) {
	t.Parallel()

	exps := []snowplanev1alpha1.SQLStatementExpectation{
		{Column: "STATUS", Value: "active"},
		{Column: "NAME", Value: "test"},
	}

	clientExps := specExpectationsToClient(exps)
	assert.Len(t, clientExps, 2)
	assert.Equal(t, "STATUS", clientExps[0].Column)
	assert.Equal(t, "active", clientExps[0].Value)
	assert.Equal(t, "NAME", clientExps[1].Column)
	assert.Equal(t, "test", clientExps[1].Value)
}

func TestSpecExpectationsToClient_Empty(t *testing.T) {
	t.Parallel()

	clientExps := specExpectationsToClient(nil)
	assert.Empty(t, clientExps)
}

// --- Events ---

func TestReconcile_Create_EmitsCreatedEvent(t *testing.T) {
	t.Parallel()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, sql string, exps []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
			call := 0
			return func(_ context.Context, _ string, _ []sqlstmtclient.Expectation) (*sqlstmtclient.Observation, error) {
				call++
				if call == 1 {
					return &sqlstmtclient.Observation{Exists: false}, nil
				}

				return &sqlstmtclient.Observation{Exists: true, RowCount: 1, Matched: true}, nil
			}
		}(),
	}

	stmt := newTestSQLStatement("test-event", "default")
	stmt.Finalizers = []string{finalizerName}
	stmt.Spec.Observe = ptr("SELECT 1 FROM test_table")

	scheme := testutil.TestScheme()

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		WithStatusSubresource(&snowplanev1alpha1.SQLStatement{}).
		Build()

	recorder := record.NewFakeRecorder(16)

	sf := func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, func(context.Context) {}, nil
	}

	r := NewReconcilerWithServiceFactory(c, testutil.NewTestClientFactory(), recorder, nil, nil, sf)

	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SQLStatement")

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-event", "default"))
	require.NoError(t, err)

	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "created")
	default:
		t.Fatal("expected created event")
	}
}

// --- Denylist Tests ---

func newTestReconcilerWithDenylist(dl *StatementDenylist, mock *mockService, objs ...client.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SQLStatement, Service, *sqlstmtclient.Observation] {
	scheme := testutil.TestScheme()

	runtimeObjs := make([]runtime.Object, len(objs))
	for i, obj := range objs {
		runtimeObjs[i] = obj
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(runtimeObjs...).
		WithStatusSubresource(&snowplanev1alpha1.SQLStatement{}).
		Build()

	sf := func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, func(context.Context) {}, nil
	}

	r := NewReconcilerWithServiceFactory(
		c,
		testutil.NewTestClientFactory(),
		record.NewFakeRecorder(16),
		nil,
		dl,
		sf,
	)

	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SQLStatement")

	return r
}

func TestReconcile_Create_DenylistBlocked(t *testing.T) {
	t.Parallel()

	var executed bool

	mock := &mockService{
		executeFn: func(_ context.Context, _ string) error {
			executed = true
			return nil
		},
	}

	dl, err := NewStatementDenylist([]string{"DROP DATABASE"})
	require.NoError(t, err)

	stmt := newTestSQLStatement("test-blocked", "default")
	stmt.Spec.Execute = "DROP DATABASE mydb"
	stmt.Spec.DangerousAllowDestructive = true
	stmt.Finalizers = []string{finalizerName}

	r := newTestReconcilerWithDenylist(dl, mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, reconcileErr := r.Reconcile(context.Background(), testutil.ReconcileReq("test-blocked", "default"))
	require.NoError(t, reconcileErr, "terminal error should not be returned as reconcile error")
	assert.Equal(t, ctrl.Result{}, res, "should not requeue on terminal error")
	assert.False(t, executed, "execute must NOT be called when denylist blocks")

	// Verify terminal error condition is set on the object.
	var updated snowplanev1alpha1.SQLStatement
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-blocked", Namespace: "default"}, &updated))

	readyCond := apimeta.FindStatusCondition(updated.Status.Conditions, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonTerminalError, readyCond.Reason)
	assert.Contains(t, readyCond.Message, "statement denied")
}

func TestReconcile_Create_DenylistAllowed(t *testing.T) {
	t.Parallel()

	var executedSQL string

	mock := &mockService{
		executeFn: func(_ context.Context, sql string) error {
			executedSQL = sql
			return nil
		},
	}

	dl, err := NewStatementDenylist([]string{"DROP DATABASE"})
	require.NoError(t, err)

	stmt := newTestSQLStatement("test-allowed", "default")
	stmt.Spec.Execute = "CREATE TABLE IF NOT EXISTS safe_table (id INT)"
	stmt.Finalizers = []string{finalizerName}

	r := newTestReconcilerWithDenylist(dl, mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, reconcileErr := r.Reconcile(context.Background(), testutil.ReconcileReq("test-allowed", "default"))
	require.NoError(t, reconcileErr)
	assert.Equal(t, "CREATE TABLE IF NOT EXISTS safe_table (id INT)", executedSQL, "execute should proceed for allowed SQL")
}

func TestReconcile_Delete_DenylistBlocked(t *testing.T) {
	t.Parallel()

	var reverted bool

	mock := &mockService{
		revertFn: func(_ context.Context, _ string) error {
			reverted = true
			return nil
		},
	}

	dl, err := NewStatementDenylist([]string{"DROP SCHEMA"})
	require.NoError(t, err)

	stmt := newTestSQLStatement("test-del-blocked", "default")
	stmt.Spec.Execute = "CREATE SCHEMA myschema"
	stmt.Spec.Revert = ptr("DROP SCHEMA myschema")
	stmt.Spec.DangerousAllowDestructive = true
	stmt.Finalizers = []string{finalizerName}
	// Mark for deletion.
	now := metav1.Now()
	stmt.DeletionTimestamp = &now

	r := newTestReconcilerWithDenylist(dl, mock, stmt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	res, reconcileErr := r.Reconcile(context.Background(), testutil.ReconcileReq("test-del-blocked", "default"))
	require.NoError(t, reconcileErr, "terminal error should not be returned as reconcile error")
	assert.Equal(t, ctrl.Result{}, res, "should not requeue on terminal error")
	assert.False(t, reverted, "revert must NOT be called when denylist blocks")
}

// --- CRNotFound ---

func TestReconcile_CRNotFound(t *testing.T) {
	t.Parallel()

	mock := &mockService{}
	r := newTestReconciler(mock)

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
}
