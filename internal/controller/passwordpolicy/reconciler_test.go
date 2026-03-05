package passwordpolicy

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

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.PasswordPolicyObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreatePasswordPolicyOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterPasswordPolicyOptions) error {
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

func newTestPasswordPolicy(name, namespace string) *snowplanev1alpha1.PasswordPolicy {
	dbName := "MY_DB"
	schemaName := "PUBLIC"

	return &snowplanev1alpha1.PasswordPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.PasswordPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:              "MY_POLICY",
			DatabaseName:      &dbName,
			SchemaName:        &schemaName,
			PasswordMinLength: testutil.Ptr(int32(10)),
		},
	}
}

func successfulObservation() *snowflake.PasswordPolicyObservation {
	return &snowflake.PasswordPolicyObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.PasswordPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_POLICY",
			DatabaseName: "MY_DB",
			SchemaName:   "PUBLIC",
			Owner:        "SYSADMIN",
			Comment:      "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.PasswordPolicy{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicy, Service, *snowflake.PasswordPolicyObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: newAdapter(c, rec, func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		}),
		GVK: snowplanev1alpha1.GroupVersion.WithKind("PasswordPolicy"),
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
			return newTestPasswordPolicy(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.PasswordPolicy{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	pp.Finalizers = []string{finalizerName}
	pp.Status.DatabaseName = "MY_DB"
	pp.Status.SchemaName = "MY_DB.PUBLIC"

	var capturedOpts snowflake.CreatePasswordPolicyOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.PasswordPolicyObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreatePasswordPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, pp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mypp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_POLICY", capturedOpts.Name.Name())
	assert.NotNil(t, capturedOpts.PasswordMinLength)
	assert.Equal(t, int32(10), *capturedOpts.PasswordMinLength)

	got := &snowplanev1alpha1.PasswordPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mypp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	pp.Finalizers = []string{finalizerName}
	pp.Status.DatabaseName = "MY_DB"
	pp.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
			return &snowflake.PasswordPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreatePasswordPolicyOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, pp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mypp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	pp.Finalizers = []string{finalizerName}
	pp.Status.DatabaseName = "MY_DB"
	pp.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
			return &snowflake.PasswordPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreatePasswordPolicyOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, pp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mypp", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	pp.Finalizers = []string{finalizerName}
	pp.Status.DatabaseName = "MY_DB"
	pp.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	pp.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_POLICY", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, pp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mypp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.PasswordPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mypp", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	pp.Finalizers = []string{finalizerName}
	pp.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	pp.Status.DatabaseName = "MY_DB"
	pp.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	pp.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, pp, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mypp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_POLICY")

	opts := buildCreateOptions(pp, id)
	assert.Equal(t, "MY_POLICY", opts.Name.Name())
	assert.NotNil(t, opts.PasswordMinLength)
	assert.Equal(t, int32(10), *opts.PasswordMinLength)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("MinLengthSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.PasswordPolicySpec{
			PasswordMinLength: testutil.Ptr(int32(10)),
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "PASSWORD_MIN_LENGTH")
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.PasswordPolicySpec{
			Comment: testutil.Ptr("test"),
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "COMMENT")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	pp := newTestPasswordPolicy("mypp", "default")
	obs := successfulObservation()

	applyObservation(pp, obs)

	assert.NotEmpty(t, pp.Status.FullyQualifiedName)
	assert.Equal(t, "MY_POLICY", pp.Status.ShowOutput.Name)
	assert.Equal(t, "MY_DB", pp.Status.ShowOutput.DatabaseName)
	assert.Equal(t, "SYSADMIN", pp.Status.ShowOutput.Owner)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	pp := &snowplanev1alpha1.PasswordPolicy{
		Spec: snowplanev1alpha1.PasswordPolicySpec{
			Name: "MY_POLICY",
		},
	}

	obs := &snowflake.PasswordPolicyObservation{
		ShowOutput: &snowplanev1alpha1.PasswordPolicyShowOutput{
			Name: "MY_POLICY",
		},
	}

	result := detectDrift(pp, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	pp := &snowplanev1alpha1.PasswordPolicy{
		Spec: snowplanev1alpha1.PasswordPolicySpec{
			Name:    "MY_POLICY",
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.PasswordPolicyObservation{
		ShowOutput: &snowplanev1alpha1.PasswordPolicyShowOutput{
			Name:    "MY_POLICY",
			Comment: "drifted",
		},
	}

	result := detectDrift(pp, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}
