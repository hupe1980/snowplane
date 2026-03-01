package rowaccesspolicy

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateRowAccessPolicyOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterRowAccessPolicyOptions) error {
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

func newTestRowAccessPolicy(name, namespace string) *snowplanev1alpha1.RowAccessPolicy {
	return &snowplanev1alpha1.RowAccessPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_ROW_POLICY",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
			Signature: []snowplanev1alpha1.RowAccessPolicyArgument{
				{Name: "val", Type: "VARCHAR"},
			},
			Body: "CASE WHEN current_role() IN ('ANALYST') THEN true ELSE false END",
		},
	}
}

func successfulObservation() *snowflake.RowAccessPolicyObservation {
	return &snowflake.RowAccessPolicyObservation{
		Exists: true,
		ShowOutput: &snowflake.RowAccessPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_ROW_POLICY",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Kind:         "ROW_ACCESS_POLICY",
			Owner:        "SYSADMIN",
			Comment:      "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.RowAccessPolicy{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.RowAccessPolicy, Service, *snowflake.RowAccessPolicyObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("RowAccessPolicy"),
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
			return newTestRowAccessPolicy(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.RowAccessPolicy{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateRowAccessPolicyOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateRowAccessPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_ROW_POLICY", capturedOpts.Name.Name())
	assert.Len(t, capturedOpts.Signature, 1)
	assert.Equal(t, "val", capturedOpts.Signature[0].Name)
	assert.Equal(t, "VARCHAR", capturedOpts.Signature[0].Type)
	assert.Contains(t, capturedOpts.Body, "CASE WHEN")

	got := &snowplanev1alpha1.RowAccessPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrap", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateRowAccessPolicyOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.ObservedGeneration = 1
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.ObservedGeneration = 1
	rap.Generation = 2
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	rap.Spec.Comment = testutil.PtrString("updated")

	rap.Spec.ManagementPolicies.CreateOrAlter = testutil.PtrBool(false)

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedOpts snowflake.AlterRowAccessPolicyOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterRowAccessPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated", *capturedOpts.Comment)
	// Body is always sent
	assert.NotNil(t, capturedOpts.Body)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.ObservedGeneration = 1
	rap.Generation = 2
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	rap.Spec.Comment = testutil.PtrString("change")

	rap.Spec.ManagementPolicies.CreateOrAlter = testutil.PtrBool(false)

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterRowAccessPolicyOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	rap.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_ROW_POLICY", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.RowAccessPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myrap", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	rap.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	rap.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	rap.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable name
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.ObservedGeneration = 1
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"
	rap.Spec.Name = "RENAMED_POLICY"
	rap.Status.ShowOutput = &snowplanev1alpha1.RowAccessPolicyShowOutput{
		Name:         "MY_ROW_POLICY",
		DatabaseName: "MY_DB",
		SchemaName:   "MY_SCHEMA",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	// Immutable field violations are terminal — reconciler returns nil error
	// but sets NotReady condition.
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.RowAccessPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrap", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Spec.Comment = testutil.PtrString("test policy")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ROW_POLICY")

	opts := buildCreateOptions(rap, id)
	assert.Equal(t, "MY_ROW_POLICY", opts.Name.Name())
	assert.Len(t, opts.Signature, 1)
	assert.Equal(t, "val", opts.Signature[0].Name)
	assert.Equal(t, "VARCHAR", opts.Signature[0].Type)
	assert.Contains(t, opts.Body, "CASE WHEN")
	assert.Equal(t, "test policy", *opts.Comment)
}

func TestBuildAlterOptions_BodyAlwaysSent(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ROW_POLICY")
	obs := successfulObservation()

	opts := buildAlterOptions(rap, id, obs)
	assert.True(t, opts.HasChanges(), "body is always sent so HasChanges should be true")
	assert.NotNil(t, opts.Body)
	assert.Contains(t, *opts.Body, "CASE WHEN")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.RowAccessPolicySpec{
		Body:    "x",
		Comment: testutil.PtrString("c"),
	}

	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"BODY", "COMMENT"}, fields)
}

func TestComputeTrackedParameters_MinimalBody(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.RowAccessPolicySpec{
		Body: "x",
	}

	fields := tracked.ComputeTracked(spec)
	assert.Equal(t, []string{"BODY"}, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	obs := successfulObservation()

	applyObservation(rap, obs)

	assert.NotEmpty(t, rap.Status.FullyQualifiedName)
	assert.Equal(t, "MY_ROW_POLICY", rap.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", rap.Status.ShowOutput.Owner)
	assert.Equal(t, "ROW_ACCESS_POLICY", rap.Status.ShowOutput.Kind)
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Status.TrackedParameters = []string{"COMMENT", "BODY"}
	rap.Spec.Comment = nil

	unset := tracked.ComputeUnset(&rap.Spec, rap.Status.TrackedParameters)
	assert.Contains(t, unset, "COMMENT")
}

func TestComputeUnsetFields_NoTracked(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	unset := tracked.ComputeUnset(&rap.Spec, rap.Status.TrackedParameters)
	assert.Nil(t, unset)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	rap := &snowplanev1alpha1.RowAccessPolicy{
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			Name:    "MY_ROW_POLICY",
			Comment: testutil.PtrString("test"),
		},
	}

	obs := &snowflake.RowAccessPolicyObservation{
		ShowOutput: &snowflake.RowAccessPolicyShowOutput{
			Name:    "MY_ROW_POLICY",
			Comment: "test",
		},
	}

	result := detectDrift(rap, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	rap := &snowplanev1alpha1.RowAccessPolicy{
		Spec: snowplanev1alpha1.RowAccessPolicySpec{
			Name:    "MY_ROW_POLICY",
			Comment: testutil.PtrString("desired"),
		},
	}

	obs := &snowflake.RowAccessPolicyObservation{
		ShowOutput: &snowflake.RowAccessPolicyShowOutput{
			Name:    "MY_ROW_POLICY",
			Comment: "drifted",
		},
	}

	result := detectDrift(rap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	rap := newTestRowAccessPolicy("myrap", "default")
	rap.Finalizers = []string{finalizerName}
	rap.Status.DatabaseName = "MY_DB"
	rap.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.RowAccessPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.RowAccessPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateRowAccessPolicyOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, rap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrap", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
