package task

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/tracked"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTaskOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTaskOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.TaskObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateTaskOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterTaskOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name)
	}
	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestTask(name, namespace string) *snowplanev1alpha1.Task {
	return &snowplanev1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.TaskSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_TASK",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
			SQLStatement: "SELECT 1",
		},
	}
}

func successfulObservation() *snowflake.TaskObservation {
	return &snowflake.TaskObservation{
		Exists: true,
		ShowOutput: &snowflake.TaskShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_TASK",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			Warehouse:    "",
			Schedule:     "",
			State:        "suspended",
			Definition:   "SELECT 1",
			Condition:    "",
		},
		Parameters: &snowflake.TaskParameters{},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Task{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Task, Service, *snowflake.TaskObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Task"),
	}
}

// --------------------------------------------------------------------------
// Tests: Standard reconcile behavioral suite
// --------------------------------------------------------------------------

func TestReconcile_StandardSuite(t *testing.T) {
	t.Parallel()

	testutil.ReconcileSuiteConfig{
		NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
			r := newTestReconciler(&mockService{}, objs...)
			return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
		},
		NewFixture: func(name, ns string) client.Object {
			return newTestTask(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.Task{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateTaskOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TaskObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateTaskOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_TASK", capturedOpts.Name.Name())
	assert.Equal(t, "SELECT 1", capturedOpts.SQLStatement)

	got := &snowplanev1alpha1.Task{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mytask", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateWithAllFields(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	task.Spec.Schedule = testutil.PtrString("10 MINUTE")
	task.Spec.WarehouseName = testutil.PtrString("COMPUTE_WH")
	task.Status.WarehouseName = "COMPUTE_WH"
	task.Spec.Comment = testutil.PtrString("test task")
	task.Spec.When = testutil.PtrString("SYSTEM$STREAM_HAS_DATA('MYSTREAM')")
	task.Spec.AllowOverlappingExecution = testutil.PtrBool(true)
	task.Spec.UserTaskTimeoutMs = testutil.PtrInt32(60000)
	task.Spec.SuspendTaskAfterNumFailures = testutil.PtrInt32(3)
	task.Spec.ErrorIntegrationName = testutil.PtrString("MY_ERROR_INT")
	task.Status.ErrorIntegrationName = "MY_ERROR_INT"
	task.Spec.SuccessIntegrationName = testutil.PtrString("MY_SUCCESS_INT")
	task.Status.SuccessIntegrationName = "MY_SUCCESS_INT"
	task.Spec.TaskAutoRetryAttempts = testutil.PtrInt32(2)

	var capturedOpts snowflake.CreateTaskOptions
	obs := successfulObservation()
	obs.ShowOutput.Schedule = "10 MINUTE"
	obs.ShowOutput.Warehouse = "COMPUTE_WH"
	obs.ShowOutput.Comment = "test task"
	obs.ShowOutput.Condition = "SYSTEM$STREAM_HAS_DATA('MYSTREAM')"
	obs.ShowOutput.ErrorIntegration = "MY_ERROR_INT"

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TaskObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateTaskOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)

	assert.Equal(t, "10 MINUTE", *capturedOpts.Schedule)
	assert.Equal(t, "COMPUTE_WH", *capturedOpts.Warehouse)
	assert.Equal(t, "test task", *capturedOpts.Comment)
	assert.Equal(t, "SYSTEM$STREAM_HAS_DATA('MYSTREAM')", *capturedOpts.When)
	assert.True(t, *capturedOpts.AllowOverlappingExecution)
	assert.Equal(t, int32(60000), *capturedOpts.UserTaskTimeoutMs)
	assert.Equal(t, int32(3), *capturedOpts.SuspendTaskAfterNumFailures)
	assert.Equal(t, "MY_ERROR_INT", *capturedOpts.ErrorIntegration)
	assert.Equal(t, "MY_SUCCESS_INT", *capturedOpts.SuccessIntegration)
	assert.Equal(t, int32(2), *capturedOpts.TaskAutoRetryAttempts)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			return &snowflake.TaskObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateTaskOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.ObservedGeneration = 1
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	// Task supports CreateOrAlter (defaults to true), so updates use
	// the Create path with CREATE OR ALTER semantics, not Alter.
	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.ObservedGeneration = 1
	task.Generation = 2
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	task.Spec.Comment = testutil.PtrString("new comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var createCalled bool
	var capturedOpts snowflake.CreateTaskOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			return obs, nil
		},
		createFn: func(_ context.Context, opts snowflake.CreateTaskOptions) error {
			createCalled = true
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.True(t, createCalled, "expected CREATE OR ALTER path for task updates")
	assert.Equal(t, "new comment", *capturedOpts.Comment)
}

func TestReconcile_CreateOrAlterFails(t *testing.T) {
	t.Parallel()

	// Task uses CREATE OR ALTER for updates, so failures come from createFn.
	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.ObservedGeneration = 1
	task.Generation = 2
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	task.Spec.Comment = testutil.PtrString("change")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			return obs, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateTaskOptions) error {
			return fmt.Errorf("create or alter failed")
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create or alter failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	task.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_TASK", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.Task{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mytask", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	task.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	task.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	task.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable name
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.ObservedGeneration = 1
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"
	task.Spec.Name = "RENAMED_TASK"
	task.Status.ShowOutput = &snowplanev1alpha1.TaskShowOutput{
		Name:         "MY_TASK",
		DatabaseName: "MY_DB",
		SchemaName:   "MY_SCHEMA",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	// Immutable field violations are terminal — reconciler returns nil error
	// but sets NotReady condition.
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.Task{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mytask", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Spec.Schedule = testutil.PtrString("5 MINUTE")
	task.Spec.WarehouseName = testutil.PtrString("WH")
	task.Status.WarehouseName = "WH"
	task.Spec.Comment = testutil.PtrString("c")
	task.Spec.When = testutil.PtrString("SYSTEM$STREAM_HAS_DATA('S')")
	task.Spec.AllowOverlappingExecution = testutil.PtrBool(false)
	task.Spec.UserTaskTimeoutMs = testutil.PtrInt32(1000)
	task.Spec.SuspendTaskAfterNumFailures = testutil.PtrInt32(5)
	task.Spec.ErrorIntegrationName = testutil.PtrString("EI")
	task.Status.ErrorIntegrationName = "EI"
	task.Spec.SuccessIntegrationName = testutil.PtrString("SI")
	task.Status.SuccessIntegrationName = "SI"
	task.Spec.TaskAutoRetryAttempts = testutil.PtrInt32(1)

	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	opts := buildCreateOptions(task, id)

	assert.Equal(t, "MY_TASK", opts.Name.Name())
	assert.Equal(t, "SELECT 1", opts.SQLStatement)
	assert.Equal(t, "5 MINUTE", *opts.Schedule)
	assert.Equal(t, "WH", *opts.Warehouse)
	assert.Equal(t, "c", *opts.Comment)
	assert.Equal(t, "SYSTEM$STREAM_HAS_DATA('S')", *opts.When)
	assert.False(t, *opts.AllowOverlappingExecution)
	assert.Equal(t, int32(1000), *opts.UserTaskTimeoutMs)
	assert.Equal(t, int32(5), *opts.SuspendTaskAfterNumFailures)
	assert.Equal(t, "EI", *opts.ErrorIntegration)
	assert.Equal(t, "SI", *opts.SuccessIntegration)
	assert.Equal(t, int32(1), *opts.TaskAutoRetryAttempts)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Spec.Comment = testutil.PtrString("updated")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	opts := buildAlterOptions(task, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "updated", *opts.Comment)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	obs := successfulObservation()

	opts := buildAlterOptions(task, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_ScheduleChanged(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Spec.Schedule = testutil.PtrString("10 MINUTE")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	obs := successfulObservation()
	obs.ShowOutput.Schedule = "5 MINUTE"

	opts := buildAlterOptions(task, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "10 MINUTE", *opts.Schedule)
}

func TestBuildAlterOptions_SQLStatementChanged(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Spec.SQLStatement = "SELECT 2"
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	obs := successfulObservation()

	opts := buildAlterOptions(task, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "SELECT 2", *opts.SQLStatement)
}

func TestBuildAlterOptions_SuspendStateChange(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Spec.Suspend = testutil.PtrBool(false)
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK")
	obs := successfulObservation()
	obs.ShowOutput.State = "suspended"

	opts := buildAlterOptions(task, id, obs)
	assert.True(t, opts.HasChanges())
	assert.NotNil(t, opts.Suspend)
	assert.False(t, *opts.Suspend)
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Status.TrackedParameters = []string{"COMMENT", "SCHEDULE", "USER_TASK_TIMEOUT_MS", "ERROR_INTEGRATION"}

	// Comment and Schedule nil → should unset both
	unset := tracked.ComputeUnset(&task.Spec, task.Status.TrackedParameters)
	assert.Contains(t, unset, "COMMENT")
	assert.Contains(t, unset, "SCHEDULE")
	assert.Contains(t, unset, "USER_TASK_TIMEOUT_MS")
	assert.Contains(t, unset, "ERROR_INTEGRATION")
}

func TestComputeUnsetFields_NoTracked(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	unset := tracked.ComputeUnset(&task.Spec, task.Status.TrackedParameters)
	assert.Nil(t, unset)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.TaskSpec{
		Comment:                     testutil.PtrString("c"),
		Schedule:                    testutil.PtrString("s"),
		UserTaskTimeoutMs:           testutil.PtrInt32(1),
		SuspendTaskAfterNumFailures: testutil.PtrInt32(1),
		ErrorIntegrationName:        testutil.PtrString("e"),
		SuccessIntegrationName:      testutil.PtrString("s"),
		AllowOverlappingExecution:   testutil.PtrBool(true),
		TaskAutoRetryAttempts:       testutil.PtrInt32(1),
	}

	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{
		"COMMENT", "SCHEDULE", "USER_TASK_TIMEOUT_MS",
		"SUSPEND_TASK_AFTER_NUM_FAILURES", "ERROR_INTEGRATION",
		"SUCCESS_INTEGRATION", "ALLOW_OVERLAPPING_EXECUTION",
		"TASK_AUTO_RETRY_ATTEMPTS",
	}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.TaskSpec{}
	fields := tracked.ComputeTracked(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	obs := successfulObservation()

	applyObservation(task, obs)

	assert.NotEmpty(t, task.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", task.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", task.Status.SchemaName)
	assert.Equal(t, "MY_TASK", task.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", task.Status.ShowOutput.Owner)
	assert.Equal(t, "suspended", task.Status.ShowOutput.State)
	assert.Equal(t, "SELECT 1", task.Status.ShowOutput.Definition)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	task := &snowplanev1alpha1.Task{
		Spec: snowplanev1alpha1.TaskSpec{
			Name:          "MY_TASK",
			SQLStatement:  "SELECT 1",
			Comment:       testutil.PtrString("test"),
			Schedule:      testutil.PtrString("5 MINUTE"),
			WarehouseName: testutil.PtrString("WH"),
		},
		Status: snowplanev1alpha1.TaskStatus{
			DatabaseName:  "MY_DB",
			SchemaName:    "MY_SCHEMA",
			WarehouseName: "WH",
		},
	}

	obs := &snowflake.TaskObservation{
		ShowOutput: &snowflake.TaskShowOutput{
			Name:         "MY_TASK",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Comment:      "test",
			Schedule:     "5 MINUTE",
			Warehouse:    "WH",
			Definition:   "SELECT 1",
		},
	}

	result := detectDrift(task, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	task := &snowplanev1alpha1.Task{
		Spec: snowplanev1alpha1.TaskSpec{
			Name:         "MY_TASK",
			SQLStatement: "SELECT 1",
			Comment:      testutil.PtrString("desired"),
		},
		Status: snowplanev1alpha1.TaskStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.TaskObservation{
		ShowOutput: &snowflake.TaskShowOutput{
			Name:         "MY_TASK",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Comment:      "drifted",
			Definition:   "SELECT 1",
		},
	}

	result := detectDrift(task, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	task := newTestTask("mytask", "default")
	task.Finalizers = []string{finalizerName}
	task.Status.DatabaseName = "MY_DB"
	task.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TaskObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateTaskOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, task, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytask", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
