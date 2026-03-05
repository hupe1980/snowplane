package tagassociation

import (
	"context"
	"fmt"
	"strings"
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
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn  func(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error)
	setTagFn   func(ctx context.Context, opts snowflake.SetTagOptions) error
	unsetTagFn func(ctx context.Context, opts snowflake.UnsetTagOptions) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}

	return &snowflake.TagAssociationObservation{Exists: false}, nil
}

func (m *mockService) SetTag(ctx context.Context, opts snowflake.SetTagOptions) error {
	if m.setTagFn != nil {
		return m.setTagFn(ctx, opts)
	}

	return nil
}

func (m *mockService) UnsetTag(ctx context.Context, opts snowflake.UnsetTagOptions) error {
	if m.unsetTagFn != nil {
		return m.unsetTagFn(ctx, opts)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestTA(name, namespace string) *snowplanev1alpha1.TagAssociation {
	return &snowplanev1alpha1.TagAssociation{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.TagAssociationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			TagName:    testutil.Ptr("\"DB\".\"SCHEMA\".\"MY_TAG\""),
			TagValue:   "production",
			ObjectType: "TABLE",
			ObjectName: "\"DB\".\"SCHEMA\".\"MY_TABLE\"",
		},
	}
}

func successfulObservation() *snowflake.TagAssociationObservation {
	return &snowflake.TagAssociationObservation{
		Exists:   true,
		TagValue: "production",
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.TagAssociation{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: newAdapter(c, rec, func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		}),
		GVK: snowplanev1alpha1.GroupVersion.WithKind("TagAssociation"),
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
			return newTestTA(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.TagAssociation{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow (SetTag)
// --------------------------------------------------------------------------

func TestReconcile_CreateTagAssociation(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.SetTagOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TagAssociationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		setTagFn: func(_ context.Context, opts snowflake.SetTagOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "\"DB\".\"SCHEMA\".\"MY_TAG\"", capturedOpts.TagName)
	assert.Equal(t, "production", capturedOpts.TagValue)
	assert.Equal(t, "TABLE", capturedOpts.ObjectType)
	assert.Equal(t, "\"DB\".\"SCHEMA\".\"MY_TABLE\"", capturedOpts.ObjectName)

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return &snowflake.TagAssociationObservation{Exists: false}, nil
		},
		setTagFn: func(_ context.Context, _ snowflake.SetTagOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return &snowflake.TagAssociationObservation{Exists: false}, nil
		},
		setTagFn: func(_ context.Context, _ snowflake.SetTagOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid SQL"))
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Status.ObservedGeneration = 1
	ta.Generation = 2
	ta.Spec.TagValue = "staging"

	obs := successfulObservation()
	obs.TagValue = "production"

	var capturedOpts snowflake.SetTagOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return obs, nil
		},
		setTagFn: func(_ context.Context, opts snowflake.SetTagOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "staging", capturedOpts.TagValue)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Generation = 1
	ta.Status.ObservedGeneration = 1
	ta.Spec.TagValue = "production"
	hash, err := snowplanev1alpha1.ComputeSpecHash(ta.Spec)
	require.NoError(t, err)
	ta.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.TagValue = "drifted-value"

	var setTagCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return obs, nil
		},
		setTagFn: func(_ context.Context, opts snowflake.SetTagOptions) error {
			setTagCalled = true
			assert.Equal(t, "production", opts.TagValue)
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err = r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.True(t, setTagCalled, "SetTag should be called for drift correction")

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

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Generation = 1
	ta.Status.ObservedGeneration = 1
	ta.Spec.TagValue = "production"
	ta.Spec.ManagementPolicies.DriftPolicy = snowplanev1alpha1.DriftPolicyDetectOnly
	hash, err := snowplanev1alpha1.ComputeSpecHash(ta.Spec)
	require.NoError(t, err)
	ta.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.TagValue = "drifted-value"

	var setTagCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return obs, nil
		},
		setTagFn: func(_ context.Context, _ snowflake.SetTagOptions) error {
			setTagCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.False(t, setTagCalled, "SetTag should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

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
// Tests: Delete flow (UnsetTag)
// --------------------------------------------------------------------------

func TestReconcile_DeleteTagAssociation(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	now := metav1.Now()
	ta.DeletionTimestamp = &now

	var unsetCalled bool

	mock := &mockService{
		unsetTagFn: func(_ context.Context, opts snowflake.UnsetTagOptions) error {
			unsetCalled = true
			assert.Equal(t, "\"DB\".\"SCHEMA\".\"MY_TAG\"", opts.TagName)
			assert.Equal(t, "TABLE", opts.ObjectType)
			assert.Equal(t, "\"DB\".\"SCHEMA\".\"MY_TABLE\"", opts.ObjectName)
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, unsetCalled)

	got := &snowplanev1alpha1.TagAssociation{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	ta.DeletionTimestamp = &now

	var unsetCalled bool

	mock := &mockService{
		unsetTagFn: func(_ context.Context, _ snowflake.UnsetTagOptions) error {
			unsetCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, unsetCalled)
}

func TestReconcile_DeleteAlreadyGone(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	now := metav1.Now()
	ta.DeletionTimestamp = &now

	mock := &mockService{
		unsetTagFn: func(_ context.Context, _ snowflake.UnsetTagOptions) error {
			return snowflake.ErrObjectNotFound
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteUnsetFails(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	now := metav1.Now()
	ta.DeletionTimestamp = &now

	mock := &mockService{
		unsetTagFn: func(_ context.Context, _ snowflake.UnsetTagOptions) error {
			return fmt.Errorf("unset failed")
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unset failed")

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Observe errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestReconciler(mock, ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.TagAssociation{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Deletion with missing ProviderConfig
// --------------------------------------------------------------------------

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	now := metav1.Now()
	ta.DeletionTimestamp = &now

	mock := &mockService{}
	r := newTestReconciler(mock, ta)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.TagAssociation{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myta", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

// --------------------------------------------------------------------------
// Tests: ApplyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	obs := &reconciler.Observation[*snowflake.TagAssociationObservation]{
		Exists: true,
		Detail: successfulObservation(),
	}

	a := newAdapter(nil, nil, nil)
	a.ApplyObservation(ta, obs)

	assert.NotEmpty(t, ta.Status.FullyQualifiedName)
	require.NotNil(t, ta.Status.ObservedValue)
	assert.Equal(t, "production", ta.Status.ObservedValue.TagValue)
}

// --------------------------------------------------------------------------
// Tests: DetectDrift (unit)
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	obs := &reconciler.Observation[*snowflake.TagAssociationObservation]{
		Exists: true,
		Detail: successfulObservation(),
	}

	a := newAdapter(nil, nil, nil)
	result := a.DetectDrift(ta, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_TagValueDrift(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Spec.TagValue = "production"

	obs := &reconciler.Observation[*snowflake.TagAssociationObservation]{
		Exists: true,
		Detail: &snowflake.TagAssociationObservation{
			Exists:   true,
			TagValue: "drifted-value",
		},
	}

	a := newAdapter(nil, nil, nil)
	result := a.DetectDrift(ta, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "TAG_VALUE")
}

// --------------------------------------------------------------------------
// Tests: BuildAlterOptions (unit)
// --------------------------------------------------------------------------

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	obs := &reconciler.Observation[*snowflake.TagAssociationObservation]{
		Exists: true,
		Detail: successfulObservation(),
	}

	a := newAdapter(nil, nil, nil)
	opts, err := a.BuildAlterOptions(context.Background(), ta, nil, obs)
	require.NoError(t, err)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_ValueChanged(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Spec.TagValue = "staging"

	obs := &reconciler.Observation[*snowflake.TagAssociationObservation]{
		Exists: true,
		Detail: successfulObservation(), // TagValue = "production"
	}

	a := newAdapter(nil, nil, nil)
	opts, err := a.BuildAlterOptions(context.Background(), ta, nil, obs)
	require.NoError(t, err)
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: Ownership (USE ROLE)
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	ta := newTestTA("myta", "default")
	ta.Finalizers = []string{finalizerName}
	ta.Generation = 1
	ta.Status.ObservedGeneration = 1
	ta.Spec.UseRole = testutil.Ptr("DATA_ADMIN")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
			return obs, nil
		},
	}

	var capturedUseRole string

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.TagAssociation{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(ta, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	rec := record.NewFakeRecorder(100)

	r := &reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation]{
		Client:   c,
		Factory:  testutil.NewTestClientFactory(),
		Recorder: rec,
		Adapter: newAdapter(c, rec, func(_ context.Context, _ clientfactory.SnowflakeClient, useRole string) (Service, func(context.Context), error) {
			capturedUseRole = useRole
			return mock, nil, nil
		}),
		GVK: snowplanev1alpha1.GroupVersion.WithKind("TagAssociation"),
	}

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myta", "default"))
	require.NoError(t, err)

	assert.Equal(t, "DATA_ADMIN", capturedUseRole, "useRole from spec should be passed to ServiceFactory")
}
