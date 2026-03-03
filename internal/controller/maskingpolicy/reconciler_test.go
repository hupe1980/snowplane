package maskingpolicy

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.MaskingPolicyObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateMaskingPolicyOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterMaskingPolicyOptions) error {
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

func newTestMaskingPolicy(name, namespace string) *snowplanev1alpha1.MaskingPolicy {
	return &snowplanev1alpha1.MaskingPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_MASKING_POLICY",
			DatabaseName: testutil.Ptr("MY_DB"),
			SchemaName:   testutil.Ptr("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.MaskingPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
			Body: "CASE WHEN current_role() IN ('ANALYST') THEN val ELSE '***' END",
		},
	}
}

func successfulObservation() *snowflake.MaskingPolicyObservation {
	return &snowflake.MaskingPolicyObservation{
		Exists: true,
		ShowOutput: &snowflake.MaskingPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_MASKING_POLICY",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Kind:         "MASKING",
			Owner:        "SYSADMIN",
			Comment:      "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.MaskingPolicy{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicy, Service, *snowflake.MaskingPolicyObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("MaskingPolicy"),
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
			return newTestMaskingPolicy(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.MaskingPolicy{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateMaskingPolicyOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.MaskingPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateMaskingPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_MASKING_POLICY", capturedOpts.Name.Name())
	assert.Len(t, capturedOpts.Signature, 1)
	assert.Equal(t, "val", capturedOpts.Signature[0].Name)
	assert.Equal(t, "VARCHAR", capturedOpts.Signature[0].Type)
	assert.Contains(t, capturedOpts.Body, "CASE WHEN")

	got := &snowplanev1alpha1.MaskingPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mymp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			return &snowflake.MaskingPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateMaskingPolicyOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.ObservedGeneration = 1
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.ObservedGeneration = 1
	mp.Generation = 2
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	mp.Spec.Comment = testutil.Ptr("updated")

	mp.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedOpts snowflake.AlterMaskingPolicyOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterMaskingPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated", *capturedOpts.Comment)
	// Body is always sent
	assert.NotNil(t, capturedOpts.Body)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.ObservedGeneration = 1
	mp.Generation = 2
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	mp.Spec.Comment = testutil.Ptr("change")

	mp.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterMaskingPolicyOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	mp.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_MASKING_POLICY", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.MaskingPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mymp", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	mp.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	mp.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	mp.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable name
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.ObservedGeneration = 1
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"
	mp.Spec.Name = "RENAMED_POLICY"
	mp.Status.ShowOutput = &snowplanev1alpha1.MaskingPolicyShowOutput{
		Name:         "MY_MASKING_POLICY",
		DatabaseName: "MY_DB",
		SchemaName:   "MY_SCHEMA",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	// Immutable field violations are terminal — reconciler returns nil error
	// but sets NotReady condition.
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.MaskingPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mymp", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Spec.Comment = testutil.Ptr("test policy")
	mp.Spec.ExemptOtherPolicies = testutil.Ptr(true)
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_MASKING_POLICY")

	opts := buildCreateOptions(mp, id)
	assert.Equal(t, "MY_MASKING_POLICY", opts.Name.Name())
	assert.Len(t, opts.Signature, 1)
	assert.Equal(t, "val", opts.Signature[0].Name)
	assert.Equal(t, "VARCHAR", opts.Signature[0].Type)
	assert.Contains(t, opts.Body, "CASE WHEN")
	assert.Equal(t, "test policy", *opts.Comment)
	assert.True(t, *opts.ExemptOtherPolicies)
}

func TestBuildAlterOptions_BodyAlwaysSent(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_MASKING_POLICY")
	obs := successfulObservation()

	opts := buildAlterOptions(mp, id, obs)
	assert.True(t, opts.HasChanges(), "body is always sent so HasChanges should be true")
	assert.NotNil(t, opts.Body)
	assert.Contains(t, *opts.Body, "CASE WHEN")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.MaskingPolicySpec{
		Body:                "x",
		Comment:             testutil.Ptr("c"),
		ExemptOtherPolicies: testutil.Ptr(true),
	}

	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"BODY", "COMMENT", "EXEMPT_OTHER_POLICIES"}, fields)
}

func TestComputeTrackedParameters_MinimalBody(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.MaskingPolicySpec{
		Body: "x",
	}

	fields := tracked.ComputeTracked(spec)
	assert.Equal(t, []string{"BODY"}, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	obs := successfulObservation()

	applyObservation(mp, obs)

	assert.NotEmpty(t, mp.Status.FullyQualifiedName)
	assert.Equal(t, "MY_MASKING_POLICY", mp.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", mp.Status.ShowOutput.Owner)
	assert.Equal(t, "MASKING", mp.Status.ShowOutput.Kind)
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Status.TrackedParameters = []string{"COMMENT", "BODY"}
	mp.Spec.Comment = nil

	unset := tracked.ComputeUnset(&mp.Spec, mp.Status.TrackedParameters)
	assert.Contains(t, unset, "COMMENT")
}

func TestComputeUnsetFields_NoTracked(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	unset := tracked.ComputeUnset(&mp.Spec, mp.Status.TrackedParameters)
	assert.Nil(t, unset)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	mp := &snowplanev1alpha1.MaskingPolicy{
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			Name:    "MY_MASKING_POLICY",
			Comment: testutil.Ptr("test"),
		},
	}

	obs := &snowflake.MaskingPolicyObservation{
		ShowOutput: &snowflake.MaskingPolicyShowOutput{
			Name:    "MY_MASKING_POLICY",
			Comment: "test",
		},
	}

	result := detectDrift(mp, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	mp := &snowplanev1alpha1.MaskingPolicy{
		Spec: snowplanev1alpha1.MaskingPolicySpec{
			Name:    "MY_MASKING_POLICY",
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.MaskingPolicyObservation{
		ShowOutput: &snowflake.MaskingPolicyShowOutput{
			Name:    "MY_MASKING_POLICY",
			Comment: "drifted",
		},
	}

	result := detectDrift(mp, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	mp := newTestMaskingPolicy("mymp", "default")
	mp.Finalizers = []string{finalizerName}
	mp.Status.DatabaseName = "MY_DB"
	mp.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaskingPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.MaskingPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateMaskingPolicyOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, mp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymp", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
