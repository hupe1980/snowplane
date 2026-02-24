package fileformat

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFileFormatOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFileFormatOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.FileFormatObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateFileFormatOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterFileFormatOptions) error {
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

func newTestFileFormat(name, namespace string) *snowplanev1alpha1.FileFormat {
	return &snowplanev1alpha1.FileFormat{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.FileFormatSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_FF",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
			Type:         snowplanev1alpha1.FileFormatTypeCSV,
		},
	}
}

func successfulObservation() *snowflake.FileFormatObservation {
	return &snowflake.FileFormatObservation{
		Exists: true,
		ShowOutput: &snowflake.FileFormatShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_FF",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			Type:         "CSV",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.FileFormat{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("FileFormat"),
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
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	r := newTestReconciler(&mockService{}, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.FileFormat{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myff", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateFileFormatOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
				call++
				if call == 1 {
					return &snowflake.FileFormatObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateFileFormatOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_FF", capturedOpts.Name.Name())
	assert.Equal(t, "CSV", capturedOpts.Type)

	got := &snowplanev1alpha1.FileFormat{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myff", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			return &snowflake.FileFormatObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateFileFormatOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Spec.FieldDelimiter = testutil.PtrString("|")
	ff.Spec.Comment = testutil.PtrString("test")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_FF")

	opts := buildCreateOptions(ff, id)
	assert.Equal(t, "MY_FF", opts.Name.Name())
	assert.Equal(t, "CSV", opts.Type)
	assert.Equal(t, "|", *opts.FieldDelimiter)
	assert.Equal(t, "test", *opts.Comment)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FileFormatSpec{}
		assert.Empty(t, computeTrackedParameters(spec))
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FileFormatSpec{
			Comment: testutil.PtrString("x"),
		}
		assert.Contains(t, computeTrackedParameters(spec), "COMMENT")
	})

	t.Run("FieldDelimiterSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FileFormatSpec{
			FieldDelimiter: testutil.PtrString(","),
		}
		assert.Contains(t, computeTrackedParameters(spec), "FIELD_DELIMITER")
	})
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	t.Run("NoTrackedParameters", func(t *testing.T) {
		t.Parallel()
		ff := &snowplanev1alpha1.FileFormat{}
		assert.Nil(t, computeUnsetFields(ff))
	})

	t.Run("CommentRemoved", func(t *testing.T) {
		t.Parallel()
		ff := &snowplanev1alpha1.FileFormat{
			Status: snowplanev1alpha1.FileFormatStatus{
				TrackedParameters: []string{"COMMENT"},
			},
		}
		unset := computeUnsetFields(ff)
		assert.Contains(t, unset, "COMMENT")
	})
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	ff := &snowplanev1alpha1.FileFormat{
		Spec: snowplanev1alpha1.FileFormatSpec{
			Name: "MY_FF",
			Type: snowplanev1alpha1.FileFormatTypeCSV,
		},
		Status: snowplanev1alpha1.FileFormatStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.FileFormatObservation{
		ShowOutput: &snowflake.FileFormatShowOutput{
			Name:         "MY_FF",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Type:         "CSV",
		},
	}

	result := detectDrift(ff, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	ff := &snowplanev1alpha1.FileFormat{
		Spec: snowplanev1alpha1.FileFormatSpec{
			Name:    "MY_FF",
			Type:    snowplanev1alpha1.FileFormatTypeCSV,
			Comment: testutil.PtrString("desired"),
		},
		Status: snowplanev1alpha1.FileFormatStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.FileFormatObservation{
		ShowOutput: &snowflake.FileFormatShowOutput{
			Name:         "MY_FF",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Type:         "CSV",
			Comment:      "drifted",
		},
	}

	result := detectDrift(ff, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: ProviderConfig resolution
// --------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	r := newTestReconciler(&mockService{}, ff)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

// --------------------------------------------------------------------------
// Tests: Create terminal error
// --------------------------------------------------------------------------

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			return &snowflake.FileFormatObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateFileFormatOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.ObservedGeneration = 1
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.FileFormat{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myff", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdateComment(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.ObservedGeneration = 1
	ff.Generation = 2
	ff.Spec.Comment = testutil.PtrString("new comment")
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterFileFormatOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterFileFormatOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	ff.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_FF", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.FileFormat{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myff", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"
	ff.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	ff.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: ApplyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	obs := successfulObservation()

	applyObservation(ff, obs)

	assert.NotEmpty(t, ff.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", ff.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", ff.Status.SchemaName)
	assert.Equal(t, "MY_FF", ff.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", ff.Status.ShowOutput.Owner)
	assert.Equal(t, "CSV", ff.Status.ShowOutput.Type)
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	ff := newTestFileFormat("myff", "default")
	ff.Finalizers = []string{finalizerName}
	ff.Status.DatabaseName = "MY_DB"
	ff.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
				call++
				if call == 1 {
					return &snowflake.FileFormatObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateFileFormatOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, ff, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myff", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
