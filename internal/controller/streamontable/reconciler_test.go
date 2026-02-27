package streamontable

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStreamOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStreamOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.StreamObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateStreamOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterStreamOptions) error {
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

func newTestStreamOnTable(name, namespace string) *snowplanev1alpha1.StreamOnTable {
	return &snowplanev1alpha1.StreamOnTable{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.StreamOnTableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_STREAM",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
			Table:        "MY_TABLE",
		},
	}
}

func successfulObservation() *snowflake.StreamObservation {
	return &snowflake.StreamObservation{
		Exists: true,
		ShowOutput: &snowflake.StreamShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_STREAM",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			TableName:    "MY_TABLE",
			SourceType:   "TABLE",
			Mode:         "DEFAULT",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnTable, Service, *snowflake.StreamObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.StreamOnTable{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnTable, Service, *snowflake.StreamObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("StreamOnTable"),
	}
}

// --------------------------------------------------------------------------
// Tests: CR not found
// --------------------------------------------------------------------------

func TestReconcile_CRNotFound(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(&mockService{})

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("gone", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --------------------------------------------------------------------------
// Tests: ProviderConfig resolution
// --------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	r := newTestReconciler(&mockService{}, s)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

// --------------------------------------------------------------------------
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	r := newTestReconciler(&mockService{}, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.StreamOnTable{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateStreamOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StreamObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateStreamOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_STREAM", capturedOpts.Name.Name())
	assert.Equal(t, snowflake.StreamSourceTable, capturedOpts.SourceType)
	assert.Equal(t, "MY_TABLE", capturedOpts.SourceName)

	got := &snowplanev1alpha1.StreamOnTable{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			return &snowflake.StreamObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateStreamOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			return &snowflake.StreamObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateStreamOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.ObservedGeneration = 1
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.StreamOnTable{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdateComment(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.ObservedGeneration = 1
	s.Generation = 2
	s.Spec.Comment = testutil.PtrString("new comment")
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterStreamOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterStreamOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	s.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_STREAM", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.StreamOnTable{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"
	s.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	s.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Spec.AppendOnly = testutil.PtrBool(true)
	s.Spec.Comment = testutil.PtrString("test")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_STREAM")

	opts := buildCreateOptions(s, id)
	assert.Equal(t, "MY_STREAM", opts.Name.Name())
	assert.Equal(t, snowflake.StreamSourceTable, opts.SourceType)
	assert.Equal(t, "MY_TABLE", opts.SourceName)
	assert.True(t, *opts.AppendOnly)
	assert.Equal(t, "test", *opts.Comment)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Spec.Comment = testutil.PtrString("new")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_STREAM")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	opts := buildAlterOptions(s, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new", *opts.Comment)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_STREAM")
	obs := successfulObservation()

	opts := buildAlterOptions(s, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.StreamOnTableSpec{
		Comment: testutil.PtrString("x"),
	}

	fields := computeTrackedParameters(spec)
	assert.ElementsMatch(t, []string{"COMMENT"}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.StreamOnTableSpec{}
	fields := computeTrackedParameters(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	obs := successfulObservation()

	applyObservation(s, obs)

	assert.NotEmpty(t, s.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", s.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", s.Status.SchemaName)
	assert.Equal(t, "MY_STREAM", s.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", s.Status.ShowOutput.Owner)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.StreamOnTable{
		Spec: snowplanev1alpha1.StreamOnTableSpec{
			Name:    "MY_STREAM",
			Table:   "MY_TABLE",
			Comment: testutil.PtrString("test"),
		},
		Status: snowplanev1alpha1.StreamOnTableStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.StreamObservation{
		ShowOutput: &snowflake.StreamShowOutput{
			Name:         "MY_STREAM",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			TableName:    "MY_TABLE",
			Mode:         "DEFAULT",
			Comment:      "test",
		},
	}

	result := detectDrift(s, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.StreamOnTable{
		Spec: snowplanev1alpha1.StreamOnTableSpec{
			Name:    "MY_STREAM",
			Table:   "MY_TABLE",
			Comment: testutil.PtrString("desired"),
		},
		Status: snowplanev1alpha1.StreamOnTableStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.StreamObservation{
		ShowOutput: &snowflake.StreamShowOutput{
			Name:         "MY_STREAM",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			TableName:    "MY_TABLE",
			Comment:      "drifted",
			Mode:         "DEFAULT",
		},
	}

	result := detectDrift(s, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestDetectDrift_AppendOnlyMode(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.StreamOnTable{
		Spec: snowplanev1alpha1.StreamOnTableSpec{
			Name:       "MY_STREAM",
			Table:      "MY_TABLE",
			AppendOnly: testutil.PtrBool(true),
		},
		Status: snowplanev1alpha1.StreamOnTableStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.StreamObservation{
		ShowOutput: &snowflake.StreamShowOutput{
			Name:         "MY_STREAM",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			TableName:    "MY_TABLE",
			Mode:         "APPEND_ONLY",
		},
	}

	result := detectDrift(s, obs)
	assert.False(t, result.HasDrift)
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	s := newTestStreamOnTable("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.DatabaseName = "MY_DB"
	s.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StreamObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateStreamOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
