package warehouse

import (
	"context"
	"fmt"
	"strings"
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
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateWarehouseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterWarehouseOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.WarehouseObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateWarehouseOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterWarehouseOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestWH(name, namespace string) *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.WarehouseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "ETL_WH",
		},
	}
}

func successfulObservation() *snowflake.WarehouseObservation {
	return &snowflake.WarehouseObservation{
		Exists: true,
		ShowOutput: &snowflake.WarehouseShowOutput{
			CreatedOn:       "2024-01-01",
			Name:            "ETL_WH",
			State:           "STARTED",
			Type:            "STANDARD",
			Size:            "XSMALL",
			Comment:         "",
			Owner:           "SYSADMIN",
			AutoSuspend:     600,
			AutoResume:      true,
			MinClusterCount: 1,
			MaxClusterCount: 1,
			ScalingPolicy:   "STANDARD",
			ResourceMonitor: "",
		},
		Parameters: &snowflake.WarehouseParameters{
			MaxConcurrencyLevel:             testutil.PtrInt32(8),
			StatementQueuedTimeoutInSeconds: testutil.PtrInt32(0),
			StatementTimeoutInSeconds:       testutil.PtrInt32(172800),
			EnableQueryAcceleration:         testutil.PtrBool(false),
			QueryAccelerationMaxScaleFactor: testutil.PtrInt32(8),
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Warehouse{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
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

	wh := newTestWH("mywh", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateWarehouse(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateWarehouseOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.WarehouseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "ETL_WH", capturedOpts.Name.Name())

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.Equal(t, "STARTED", got.Status.State)
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateWithAllOptions(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}

	whType := snowplanev1alpha1.WarehouseTypeStandard
	whSize := snowplanev1alpha1.WarehouseSizeLarge
	sp := snowplanev1alpha1.ScalingPolicyEconomy
	rc := snowplanev1alpha1.ResourceConstraintMemory

	wh.Spec.WarehouseType = &whType
	wh.Spec.WarehouseSize = &whSize
	wh.Spec.MinClusterCount = testutil.PtrInt32(1)
	wh.Spec.MaxClusterCount = testutil.PtrInt32(3)
	wh.Spec.ScalingPolicy = &sp
	wh.Spec.AutoSuspend = testutil.PtrInt32(300)
	wh.Spec.AutoResume = testutil.PtrBool(true)
	wh.Spec.InitiallySuspended = true
	wh.Spec.ResourceMonitor = testutil.PtrString("my_monitor")
	wh.Spec.Comment = testutil.PtrString("ETL warehouse")
	wh.Spec.EnableQueryAcceleration = testutil.PtrBool(true)
	wh.Spec.QueryAccelerationMaxScaleFactor = testutil.PtrInt32(10)
	wh.Spec.MaxConcurrencyLevel = testutil.PtrInt32(16)
	wh.Spec.StatementQueuedTimeoutInSeconds = testutil.PtrInt32(60)
	wh.Spec.StatementTimeoutInSeconds = testutil.PtrInt32(3600)
	wh.Spec.ResourceConstraint = &rc

	var capturedOpts snowflake.CreateWarehouseOptions

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.WarehouseObservation{Exists: false}, nil
				}

				return successfulObservation(), nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)

	assert.Equal(t, "STANDARD", *capturedOpts.WarehouseType)
	assert.Equal(t, "LARGE", *capturedOpts.WarehouseSize)
	assert.Equal(t, int32(1), *capturedOpts.MinClusterCount)
	assert.Equal(t, int32(3), *capturedOpts.MaxClusterCount)
	assert.Equal(t, "ECONOMY", *capturedOpts.ScalingPolicy)
	assert.Equal(t, int32(300), *capturedOpts.AutoSuspend)
	assert.Equal(t, true, *capturedOpts.AutoResume)
	assert.True(t, capturedOpts.InitiallySuspended)
	assert.Equal(t, "my_monitor", *capturedOpts.ResourceMonitor)
	assert.Equal(t, "ETL warehouse", *capturedOpts.Comment)
	assert.Equal(t, true, *capturedOpts.EnableQueryAcceleration)
	assert.Equal(t, int32(10), *capturedOpts.QueryAccelerationMaxScaleFactor)
	assert.Equal(t, int32(16), *capturedOpts.MaxConcurrencyLevel)
	assert.Equal(t, int32(60), *capturedOpts.StatementQueuedTimeoutInSeconds)
	assert.Equal(t, int32(3600), *capturedOpts.StatementTimeoutInSeconds)
	assert.Equal(t, "MEMORY", *capturedOpts.ResourceConstraint)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return &snowflake.WarehouseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateWarehouseOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return &snowflake.WarehouseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateWarehouseOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid SQL"))
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	wh.Status.ObservedGeneration = 1
	wh.Generation = 2
	wh.Spec.Comment = testutil.PtrString("new comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterWarehouseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterWarehouseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	wh.Generation = 1
	wh.Status.ObservedGeneration = 1
	wh.Spec.Comment = testutil.PtrString("desired comment")
	hash, err := snowplanev1alpha1.ComputeSpecHash(wh.Spec)
	require.NoError(t, err)
	wh.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted comment"

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterWarehouseOptions) error {
			alterCalled = true
			assert.Equal(t, "desired comment", *opts.Comment)
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err = r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter should be called for drift correction")

	// Should emit DriftDetected warning and DriftCorrected normal events.
	events := testutil.DrainEvents(rec)
	require.GreaterOrEqual(t, len(events), 2)

	var hasDriftDetected, hasDriftCorrected bool
	for _, e := range events {
		if strings.Contains(e, "DriftDetected") {
			hasDriftDetected = true
		}
		if strings.Contains(e, "DriftCorrected") {
			hasDriftCorrected = true
		}
	}
	assert.True(t, hasDriftDetected, "expected DriftDetected event")
	assert.True(t, hasDriftCorrected, "expected DriftCorrected event")
}

func TestReconcile_DriftDetectOnly(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Generation = 1
	wh.Status.ObservedGeneration = 1
	wh.Spec.Comment = testutil.PtrString("desired comment")
	wh.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationDriftPolicy: drift.DriftPolicyDetectOnly,
	}
	hash, err := snowplanev1alpha1.ComputeSpecHash(wh.Spec)
	require.NoError(t, err)
	wh.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted comment"

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterWarehouseOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	// DriftDetected event should still be emitted.
	events := testutil.DrainEvents(rec)
	var hasDriftDetected bool
	for _, e := range events {
		if strings.Contains(e, "DriftDetected") {
			hasDriftDetected = true
		}
	}
	assert.True(t, hasDriftDetected, "expected DriftDetected event even in detect-only mode")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteWarehouse(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	now := metav1.Now()
	wh.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "ETL_WH", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.Warehouse{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	wh.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteAlreadyGone(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	now := metav1.Now()
	wh.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return snowflake.ErrObjectNotFound
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	now := metav1.Now()
	wh.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestReconcile_MutableWarehouseTypeChange(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	wh.Generation = 2
	wh.Status.ObservedGeneration = 1
	whType := snowplanev1alpha1.WarehouseTypeSnowparkOptimized
	wh.Spec.WarehouseType = &whType
	wh.Status.ShowOutput = &snowplanev1alpha1.WarehouseShowOutput{
		Name: "ETL_WH",
		Type: "STANDARD",
	}

	obs := successfulObservation()

	var capturedAlterOpts snowflake.AlterWarehouseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterWarehouseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	// Warehouse type should be included in alter options
	require.NotNil(t, capturedAlterOpts.WarehouseType)
	assert.Equal(t, "SNOWPARK-OPTIMIZED", *capturedAlterOpts.WarehouseType)

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.False(t, conditions.IsTerminal(got), "type change should succeed, not set terminal condition")
}

func TestReconcile_ImmutableNameChange(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Generation = 2
	wh.Status.ObservedGeneration = 1
	wh.Spec.Name = "NEW_WH"
	wh.Status.ShowOutput = &snowplanev1alpha1.WarehouseShowOutput{
		Name: "ETL_WH",
		Type: "STANDARD",
	}

	mock := &mockService{}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")
}

func TestReconcile_ImmutableField_FirstReconcile_Skipped(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Status.ObservedGeneration = 0

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return &snowflake.WarehouseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateWarehouseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Observe errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestReconciler(mock, wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.Warehouse{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: UNSET support
// --------------------------------------------------------------------------

func TestBuildAlterOptions_UnsetDetection(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Status.TrackedParameters = []string{"COMMENT", "AUTO_SUSPEND", "MAX_CONCURRENCY_LEVEL"}

	obs := successfulObservation()
	id := snowflake.NewAccountObjectIdentifier("ETL_WH")

	opts := buildAlterOptions(wh, id, obs)
	assert.ElementsMatch(t, []string{"COMMENT", "AUTO_SUSPEND", "MAX_CONCURRENCY_LEVEL"}, opts.UnsetFields)
}

func TestBuildAlterOptions_NoUnsetWhenFieldStillSet(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.Comment = testutil.PtrString("still here")
	wh.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	id := snowflake.NewAccountObjectIdentifier("ETL_WH")

	opts := buildAlterOptions(wh, id, obs)
	assert.Empty(t, opts.UnsetFields)
}

func TestBuildAlterOptions_WarehouseTypeChange(t *testing.T) {
	t.Parallel()

	whType := snowplanev1alpha1.WarehouseTypeSnowparkOptimized
	wh := newTestWH("mywh", "default")
	wh.Spec.WarehouseType = &whType

	obs := successfulObservation()
	obs.ShowOutput.Type = "STANDARD"
	id := snowflake.NewAccountObjectIdentifier("ETL_WH")

	opts := buildAlterOptions(wh, id, obs)
	require.NotNil(t, opts.WarehouseType)
	assert.Equal(t, "SNOWPARK-OPTIMIZED", *opts.WarehouseType)
}

func TestBuildAlterOptions_WarehouseTypeNoChange(t *testing.T) {
	t.Parallel()

	whType := snowplanev1alpha1.WarehouseTypeStandard
	wh := newTestWH("mywh", "default")
	wh.Spec.WarehouseType = &whType

	obs := successfulObservation()
	obs.ShowOutput.Type = "STANDARD"
	id := snowflake.NewAccountObjectIdentifier("ETL_WH")

	opts := buildAlterOptions(wh, id, obs)
	assert.Nil(t, opts.WarehouseType, "should not include type when it matches observed")
}

// --------------------------------------------------------------------------
// Tests: TrackedParameters
// --------------------------------------------------------------------------

func TestComputeWarehouseTrackedParameters(t *testing.T) {
	t.Parallel()

	size := snowplanev1alpha1.WarehouseSizeLarge
	whType := snowplanev1alpha1.WarehouseTypeStandard
	spec := &snowplanev1alpha1.WarehouseSpec{
		WarehouseType: &whType,
		Comment:       testutil.PtrString("x"),
		WarehouseSize: &size,
		AutoSuspend:   testutil.PtrInt32(300),
	}

	fields := computeTrackedParameters(spec)
	assert.ElementsMatch(t, []string{"WAREHOUSE_TYPE", "COMMENT", "WAREHOUSE_SIZE", "AUTO_SUSPEND"}, fields)
}

func TestComputeWarehouseTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.WarehouseSpec{}
	fields := computeTrackedParameters(spec)
	assert.Empty(t, fields)
}

// --------------------------------------------------------------------------
// Tests: applyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	obs := successfulObservation()

	applyObservation(wh, obs)

	assert.NotEmpty(t, wh.Status.FullyQualifiedName)
	assert.Equal(t, "SYSADMIN", wh.Status.ShowOutput.Owner)
	assert.Equal(t, "STARTED", wh.Status.State)
	assert.Equal(t, "2024-01-01", wh.Status.ShowOutput.CreatedOn)
	require.NotNil(t, wh.Status.ShowOutput)
	assert.Equal(t, "ETL_WH", wh.Status.ShowOutput.Name)
	assert.Equal(t, "STANDARD", wh.Status.ShowOutput.Type)
	assert.Equal(t, "XSMALL", wh.Status.ShowOutput.Size)
}

func TestApplyObservation_PreservesCreatedOn(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")

	obs := successfulObservation()
	obs.ShowOutput.CreatedOn = "2024-01-01"

	applyObservation(wh, obs)

	assert.Equal(t, "2024-01-01", wh.Status.ShowOutput.CreatedOn)
}

// --------------------------------------------------------------------------
// Tests: validateImmutableFields (unit)
// --------------------------------------------------------------------------

func TestValidateImmutableFields_FirstReconcile(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Status.ObservedGeneration = 0

	err := (&adapter{}).ValidateImmutableFields(context.Background(), wh)
	assert.NoError(t, err)
}

func TestValidateImmutableFields_TypeChanged_IsAllowed(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Status.ObservedGeneration = 1
	whType := snowplanev1alpha1.WarehouseTypeSnowparkOptimized
	wh.Spec.WarehouseType = &whType
	wh.Status.ShowOutput = &snowplanev1alpha1.WarehouseShowOutput{Type: "STANDARD"}

	err := (&adapter{}).ValidateImmutableFields(context.Background(), wh)
	assert.NoError(t, err, "warehouseType is now mutable; changing it should not be an error")
}

func TestValidateImmutableFields_TypeUnchanged(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Status.ObservedGeneration = 1
	whType := snowplanev1alpha1.WarehouseTypeStandard
	wh.Spec.WarehouseType = &whType
	wh.Status.ShowOutput = &snowplanev1alpha1.WarehouseShowOutput{Type: "STANDARD"}

	err := (&adapter{}).ValidateImmutableFields(context.Background(), wh)
	assert.NoError(t, err)
}

func TestValidateImmutableFields_NoShowOutput(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Status.ObservedGeneration = 1
	wh.Status.ShowOutput = nil

	err := (&adapter{}).ValidateImmutableFields(context.Background(), wh)
	assert.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Deletion with missing ProviderConfig
// --------------------------------------------------------------------------

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	now := metav1.Now()
	wh.DeletionTimestamp = &now

	mock := &mockService{}
	r := newTestReconciler(mock, wh)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.Warehouse{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mywh", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

// --------------------------------------------------------------------------
// Tests: detectDrift (unit)
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	obs := successfulObservation()

	result := detectDrift(wh, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_CommentDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.Comment = testutil.PtrString("desired")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "actual"

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestDetectDrift_WarehouseTypeDrift(t *testing.T) {
	t.Parallel()

	whType := snowplanev1alpha1.WarehouseTypeSnowparkOptimized
	wh := newTestWH("mywh", "default")
	wh.Spec.WarehouseType = &whType

	obs := successfulObservation()
	obs.ShowOutput.Type = "STANDARD"

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "WAREHOUSE_TYPE")

	// It should be mutable drift, not immutable
	for _, d := range result.Changes {
		if d.Field == "WAREHOUSE_TYPE" {
			assert.False(t, d.Immutable, "WAREHOUSE_TYPE should be reported as mutable drift")
		}
	}
}

func TestDetectDrift_ParameterDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.MaxConcurrencyLevel = testutil.PtrInt32(16)

	obs := successfulObservation()
	obs.Parameters.MaxConcurrencyLevel = testutil.PtrInt32(8)

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "MAX_CONCURRENCY_LEVEL")
}

func TestDetectDrift_AutoSuspendDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.AutoSuspend = testutil.PtrInt32(300)

	obs := successfulObservation()
	obs.ShowOutput.AutoSuspend = 600

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "AUTO_SUSPEND")
}

func TestDetectDrift_MultiClusterDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.MinClusterCount = testutil.PtrInt32(2)
	wh.Spec.MaxClusterCount = testutil.PtrInt32(5)

	obs := successfulObservation()
	obs.ShowOutput.MinClusterCount = 1
	obs.ShowOutput.MaxClusterCount = 1

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "MIN_CLUSTER_COUNT")
	assert.Contains(t, result.Summary(), "MAX_CLUSTER_COUNT")
}

func TestDetectDrift_ScalingPolicyDrift(t *testing.T) {
	t.Parallel()

	sp := snowplanev1alpha1.ScalingPolicyEconomy
	wh := newTestWH("mywh", "default")
	wh.Spec.ScalingPolicy = &sp

	obs := successfulObservation()
	obs.ShowOutput.ScalingPolicy = "STANDARD"

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "SCALING_POLICY")
}

func TestDetectDrift_ResourceMonitorDrift(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Spec.ResourceMonitor = testutil.PtrString("my_monitor")

	obs := successfulObservation()
	obs.ShowOutput.ResourceMonitor = ""

	result := detectDrift(wh, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "RESOURCE_MONITOR")
}

func TestDetectDrift_NilFieldsIgnored(t *testing.T) {
	t.Parallel()

	// No spec fields set — observed values should not cause drift.
	wh := newTestWH("mywh", "default")

	obs := successfulObservation()
	obs.ShowOutput.AutoSuspend = 999
	obs.ShowOutput.MinClusterCount = 4
	obs.ShowOutput.MaxClusterCount = 8
	obs.ShowOutput.ResourceMonitor = "some_monitor"

	result := detectDrift(wh, obs)
	assert.False(t, result.HasDrift, "nil spec fields should not report drift")
}

// --------------------------------------------------------------------------
// Tests: Ownership (USE ROLE)
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	wh := newTestWH("mywh", "default")
	wh.Finalizers = []string{finalizerName}
	wh.Generation = 1
	wh.Status.ObservedGeneration = 1
	wh.Spec.UseRole = testutil.PtrString("DATA_ADMIN")

	obs := successfulObservation()
	obs.ShowOutput.Owner = "DATA_ADMIN"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
			return obs, nil
		},
	}

	var capturedUseRole string

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Warehouse{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(wh, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	rec := record.NewFakeRecorder(100)

	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Warehouse, Service, *snowflake.WarehouseObservation]{
		Client:   c,
		Factory:  clientfactory.NewClientFactory(),
		Recorder: rec,
		Adapter: &adapter{
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, useRole string) (Service, func(context.Context), error) {
				capturedUseRole = useRole
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
	}

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mywh", "default"))
	require.NoError(t, err)

	assert.Equal(t, "DATA_ADMIN", capturedUseRole, "useRole from spec should be passed to ServiceFactory")
}
