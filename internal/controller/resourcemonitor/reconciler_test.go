package resourcemonitor

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
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/tracked"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.ResourceMonitorObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateResourceMonitorOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterResourceMonitorOptions) error {
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

func newTestResourceMonitor(name, namespace string) *snowplanev1alpha1.ResourceMonitor {
	return &snowplanev1alpha1.ResourceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "MY_MONITOR",
			CreditQuota: testutil.Ptr(int32(100)),
		},
	}
}

func successfulObservation() *snowflake.ResourceMonitorObservation {
	return &snowflake.ResourceMonitorObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ResourceMonitorShowOutput{
			CreatedOn:        "2024-01-01",
			Name:             "MY_MONITOR",
			CreditQuota:      "100",
			UsedCredits:      "25",
			RemainingCredits: "75",
			Level:            "ACCOUNT",
			Frequency:        "",
			StartTime:        "",
			EndTime:          "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.ResourceMonitor, Service, *snowflake.ResourceMonitorObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.ResourceMonitor{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := testutil.NewTestClientFactory()

	r := NewReconcilerWithServiceFactory(c, factory, record.NewFakeRecorder(100), nil,
		func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("ResourceMonitor")

	return r
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
			return newTestResourceMonitor(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.ResourceMonitor{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func TestReconcile_Create(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	obs := successfulObservation()
	var capturedOpts snowflake.CreateResourceMonitorOptions
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ResourceMonitorObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateResourceMonitorOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "MY_MONITOR", capturedOpts.Name.Name())
	assert.Equal(t, int32(100), *capturedOpts.CreditQuota)
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateWithTriggers(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	freq := snowplanev1alpha1.ResourceMonitorFrequencyMonthly
	rm.Spec.Frequency = &freq
	rm.Spec.StartTimestamp = testutil.Ptr("IMMEDIATELY")
	rm.Spec.Triggers = []snowplanev1alpha1.ResourceMonitorTrigger{
		{Threshold: 80, Action: snowplanev1alpha1.ResourceMonitorTriggerActionNotify},
		{Threshold: 100, Action: snowplanev1alpha1.ResourceMonitorTriggerActionSuspend},
	}
	obs := successfulObservation()
	obs.ShowOutput.Frequency = "MONTHLY"
	obs.ShowOutput.NotifyAt = "80"
	obs.ShowOutput.SuspendAt = "100"
	var capturedOpts snowflake.CreateResourceMonitorOptions
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ResourceMonitorObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateResourceMonitorOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, "MONTHLY", *capturedOpts.Frequency)
	assert.Equal(t, "IMMEDIATELY", *capturedOpts.StartTimestamp)
	require.Len(t, capturedOpts.Triggers, 2)
	assert.Equal(t, int32(80), capturedOpts.Triggers[0].Threshold)
	assert.Equal(t, "NOTIFY", capturedOpts.Triggers[0].Action)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return &snowflake.ResourceMonitorObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateResourceMonitorOptions) error {
			return fmt.Errorf("permission denied")
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	rm.Status.ObservedGeneration = 1
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return obs, nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	rm.Status.ObservedGeneration = 1
	rm.Generation = 2
	rm.Spec.CreditQuota = testutil.Ptr(int32(200))
	obs := successfulObservation()
	var capturedAlterOpts snowflake.AlterResourceMonitorOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterResourceMonitorOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.NotNil(t, capturedAlterOpts.CreditQuota)
	assert.Equal(t, int32(200), *capturedAlterOpts.CreditQuota)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	rm.Status.ObservedGeneration = 1
	rm.Spec.CreditQuota = testutil.Ptr(int32(999))
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterResourceMonitorOptions) error {
			return fmt.Errorf("alter failed")
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	now := metav1.Now()
	rm.DeletionTimestamp = &now
	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_MONITOR", name.Name())
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)
	got := &snowplanev1alpha1.ResourceMonitor{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	rm.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	rm.DeletionTimestamp = &now
	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled, "should not drop when orphan policy")
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	now := metav1.Now()
	rm.DeletionTimestamp = &now
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	rm.Spec.Name = "NEW_NAME"
	rm.Status.ObservedGeneration = 1
	rm.Status.ShowOutput = &snowplanev1alpha1.ResourceMonitorShowOutput{
		Name: "OLD_NAME",
	}
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			return obs, nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	got := &snowplanev1alpha1.ResourceMonitor{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrm", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	freq := snowplanev1alpha1.ResourceMonitorFrequencyMonthly
	rm.Spec.Frequency = &freq
	rm.Spec.StartTimestamp = testutil.Ptr("IMMEDIATELY")
	rm.Spec.Triggers = []snowplanev1alpha1.ResourceMonitorTrigger{
		{Threshold: 90, Action: snowplanev1alpha1.ResourceMonitorTriggerActionNotify},
	}
	id := snowflake.NewAccountObjectIdentifier("MY_MONITOR")
	opts := buildCreateOptions(rm, id)
	assert.Equal(t, "MY_MONITOR", opts.Name.Name())
	assert.Equal(t, int32(100), *opts.CreditQuota)
	assert.Equal(t, "MONTHLY", *opts.Frequency)
	assert.Equal(t, "IMMEDIATELY", *opts.StartTimestamp)
	require.Len(t, opts.Triggers, 1)
	assert.Equal(t, int32(90), opts.Triggers[0].Threshold)
	assert.Equal(t, "NOTIFY", opts.Triggers[0].Action)
}

func TestBuildAlterOptions_CreditQuotaChanged(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Spec.CreditQuota = testutil.Ptr(int32(200))
	id := snowflake.NewAccountObjectIdentifier("MY_MONITOR")
	obs := successfulObservation()
	opts := buildAlterOptions(rm, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, int32(200), *opts.CreditQuota)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Spec.CreditQuota = nil
	id := snowflake.NewAccountObjectIdentifier("MY_MONITOR")
	obs := successfulObservation()
	opts := buildAlterOptions(rm, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()
	freq := snowplanev1alpha1.ResourceMonitorFrequencyMonthly
	spec := &snowplanev1alpha1.ResourceMonitorSpec{
		CreditQuota:    testutil.Ptr(int32(100)),
		Frequency:      &freq,
		StartTimestamp: testutil.Ptr("IMMEDIATELY"),
		EndTimestamp:   testutil.Ptr("2025-12-31"),
		NotifyUsers:    []string{"admin"},
		Triggers:       []snowplanev1alpha1.ResourceMonitorTrigger{{Threshold: 80, Action: snowplanev1alpha1.ResourceMonitorTriggerActionNotify}},
	}
	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"CREDIT_QUOTA", "FREQUENCY", "START_TIMESTAMP", "END_TIMESTAMP", "NOTIFY_USERS", "TRIGGERS"}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()
	spec := &snowplanev1alpha1.ResourceMonitorSpec{}
	fields := tracked.ComputeTracked(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	obs := successfulObservation()
	applyObservation(rm, obs)
	assert.NotEmpty(t, rm.Status.FullyQualifiedName)
	assert.Equal(t, "MY_MONITOR", rm.Status.ShowOutput.Name)
	assert.Equal(t, "2024-01-01", rm.Status.ShowOutput.CreatedOn)
	assert.Equal(t, "100", rm.Status.ShowOutput.CreditQuota)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()
	rm := &snowplanev1alpha1.ResourceMonitor{
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			Name:        "MY_MONITOR",
			CreditQuota: testutil.Ptr(int32(100)),
		},
	}
	obs := &snowflake.ResourceMonitorObservation{
		ShowOutput: &snowplanev1alpha1.ResourceMonitorShowOutput{
			Name:        "MY_MONITOR",
			CreditQuota: "100",
		},
	}
	result := detectDrift(rm, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()
	rm := &snowplanev1alpha1.ResourceMonitor{
		Spec: snowplanev1alpha1.ResourceMonitorSpec{
			Name:        "MY_MONITOR",
			CreditQuota: testutil.Ptr(int32(200)),
		},
	}
	obs := &snowflake.ResourceMonitorObservation{
		ShowOutput: &snowplanev1alpha1.ResourceMonitorShowOutput{
			Name:        "MY_MONITOR",
			CreditQuota: "100",
		},
	}
	result := detectDrift(rm, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "CREDIT_QUOTA")
}

func TestSpecTriggersToClient(t *testing.T) {
	t.Parallel()
	triggers := []snowplanev1alpha1.ResourceMonitorTrigger{
		{Threshold: 80, Action: snowplanev1alpha1.ResourceMonitorTriggerActionNotify},
		{Threshold: 100, Action: snowplanev1alpha1.ResourceMonitorTriggerActionSuspend},
	}
	result := specTriggersToClient(triggers)
	require.Len(t, result, 2)
	assert.Equal(t, int32(80), result[0].Threshold)
	assert.Equal(t, "NOTIFY", result[0].Action)
	assert.Equal(t, int32(100), result[1].Threshold)
	assert.Equal(t, "SUSPEND", result[1].Action)
}

func TestSpecTriggersToClient_Empty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, specTriggersToClient(nil))
	assert.Nil(t, specTriggersToClient([]snowplanev1alpha1.ResourceMonitorTrigger{}))
}

func TestNormalizeTriggers(t *testing.T) {
	t.Parallel()
	triggers := []snowplanev1alpha1.ResourceMonitorTrigger{
		{Threshold: 100, Action: snowplanev1alpha1.ResourceMonitorTriggerActionSuspend},
		{Threshold: 80, Action: snowplanev1alpha1.ResourceMonitorTriggerActionNotify},
	}
	result := normalizeTriggers(triggers)
	assert.Equal(t, "100:SUSPEND,80:NOTIFY", result)
}

func TestNormalizeTriggers_Empty(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", normalizeTriggers(nil))
}

func TestBuildObservedTriggers(t *testing.T) {
	t.Parallel()
	show := &snowplanev1alpha1.ResourceMonitorShowOutput{
		NotifyAt:             "80,90",
		SuspendAt:            "100",
		SuspendImmediatelyAt: "110",
	}
	result := buildObservedTriggers(show)
	assert.Contains(t, result, "80:NOTIFY")
	assert.Contains(t, result, "90:NOTIFY")
	assert.Contains(t, result, "100:SUSPEND")
	assert.Contains(t, result, "110:SUSPEND_IMMEDIATE")
}

func TestBuildObservedTriggers_Empty(t *testing.T) {
	t.Parallel()
	show := &snowplanev1alpha1.ResourceMonitorShowOutput{}
	result := buildObservedTriggers(show)
	assert.Equal(t, "", result)
}

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ResourceMonitorObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ResourceMonitorObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateResourceMonitorOptions) error {
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()
	rm := newTestResourceMonitor("myrm", "default")
	rm.Finalizers = []string{finalizerName}
	now := metav1.Now()
	rm.DeletionTimestamp = &now
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return nil
		},
	}
	r := newTestReconciler(mock, rm, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrm", "default"))
	require.NoError(t, err)
	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Deleting")
}
