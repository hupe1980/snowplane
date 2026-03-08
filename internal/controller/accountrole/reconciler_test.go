package accountrole

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.AccountRoleObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error {
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

func newTestRole(name, namespace string) *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "DATA_ANALYST",
		},
	}
}

// successfulObservation returns a standard existing-role observation.
func successfulObservation() *snowflake.AccountRoleObservation {
	return &snowflake.AccountRoleObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.AccountRoleShowOutput{
			CreatedOn:      "2024-01-01",
			Name:           "DATA_ANALYST",
			Comment:        "",
			Owner:          "SECURITYADMIN",
			GrantedToRoles: ptr(int32(0)),
			GrantedRoles:   ptr(int32(0)),
		},
	}
}

// newTestReconciler builds a reconciler with a fake k8s client and injected mock service.
func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.AccountRole{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("AccountRole")

	return r
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

	role := newTestRole("myrole", "default")
	r := newTestReconciler(&mockService{}, role)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

func TestReconcile_ProviderConfigNotReady(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	pc := testutil.NewTestPC("default")
	pc.Status.Conditions = nil

	r := newTestReconciler(&mockService{}, role, pc, testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProviderConfig not ready")
}

// --------------------------------------------------------------------------
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter, "should requeue after adding finalizer")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateRole(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateAccountRoleOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AccountRoleObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAccountRoleOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "DATA_ANALYST", capturedOpts.Name.Name())

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, "SECURITYADMIN", got.Status.ShowOutput.Owner)
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateWithComment(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Spec.Comment = testutil.Ptr("analyst role")

	var capturedOpts snowflake.CreateAccountRoleOptions
	obs := successfulObservation()
	obs.ShowOutput.Comment = "analyst role"

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AccountRoleObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAccountRoleOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	assert.Equal(t, "analyst role", *capturedOpts.Comment)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return &snowflake.AccountRoleObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return &snowflake.AccountRoleObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid SQL"))
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

func TestReconcile_CreatePostObserveError(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AccountRoleObservation{Exists: false}, nil
				}

				return nil, fmt.Errorf("observe timeout")
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post-create observe")
	assert.Zero(t, result.RequeueAfter, "error return should let controller-runtime apply exponential backoff")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1
	role.Generation = 2
	role.Spec.Comment = testutil.Ptr("new comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterAccountRoleOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAccountRoleOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1
	role.Spec.Comment = testutil.Ptr("changed")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "original"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterAccountRoleOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_AlterTerminalError(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1
	role.Spec.Comment = testutil.Ptr("bad")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "original"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterAccountRoleOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("terminal: bad syntax"))
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Observe errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteRole(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "DATA_ANALYST", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.AccountRole{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err), "object should be deleted after finalizer removal")
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	role.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled, "should not drop when orphan policy")
}

func TestReconcile_DeleteAlreadyGone(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return snowflake.ErrObjectNotFound
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_DeleteNoFinalizer(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{"some-other-finalizer"}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	mock := &mockService{}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	mock := &mockService{}
	r := newTestReconciler(mock, role)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.AccountRole{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err), "finalizer should be removed when PC is missing during deletion")
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Spec.Name = "NEW_NAME"
	role.Status.ObservedGeneration = 1
	role.Status.ShowOutput = &snowplanev1alpha1.AccountRoleShowOutput{
		Name: "OLD_NAME",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

func TestValidateImmutableFields_FirstReconcile(t *testing.T) {
	t.Parallel()

	erole := newTestRole("myrole", "default")
	erole.Status.ObservedGeneration = 0

	err := validateImmutableFields(context.Background(), erole)
	assert.NoError(t, err, "should skip on first reconcile")
}

func TestValidateImmutableFields_NoShowOutput(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Status.ObservedGeneration = 1
	role.Status.ShowOutput = nil

	err := validateImmutableFields(context.Background(), role)
	assert.NoError(t, err, "should skip when showOutput is nil")
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Spec.Comment = testutil.Ptr("my role")
	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")

	opts := buildCreateOptions(role, id)
	assert.Equal(t, "DATA_ANALYST", opts.Name.Name())
	assert.Equal(t, "my role", *opts.Comment)
}

func TestBuildCreateOptions_Minimal(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")

	opts := buildCreateOptions(role, id)
	assert.Equal(t, "DATA_ANALYST", opts.Name.Name())
	assert.Nil(t, opts.Comment)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Spec.Comment = testutil.Ptr("new")

	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	opts := buildAlterOptions(role, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new", *opts.Comment)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")
	obs := successfulObservation()

	opts := buildAlterOptions(role, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_UnsetComment(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Status.TrackedParameters = []string{"COMMENT"}

	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	opts := buildAlterOptions(role, id, obs)
	assert.Contains(t, opts.UnsetFields, "COMMENT")
	assert.True(t, opts.HasChanges())
}

func TestBuildAlterOptions_NoUnsetWhenFieldStillSet(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Spec.Comment = testutil.Ptr("still here")
	role.Status.TrackedParameters = []string{"COMMENT"}

	id := snowflake.NewAccountObjectIdentifier("DATA_ANALYST")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "still here"

	opts := buildAlterOptions(role, id, obs)
	assert.Empty(t, opts.UnsetFields)
	assert.False(t, opts.HasChanges())
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.AccountRoleSpec{
		Comment: testutil.Ptr("x"),
	}

	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"COMMENT"}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.AccountRoleSpec{}
	fields := tracked.ComputeTracked(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	obs := successfulObservation()

	applyObservation(role, obs)

	assert.NotEmpty(t, role.Status.FullyQualifiedName)
	assert.Equal(t, "SECURITYADMIN", role.Status.ShowOutput.Owner)
	assert.Equal(t, "2024-01-01", role.Status.ShowOutput.CreatedOn)
	require.NotNil(t, role.Status.ShowOutput)
	assert.Equal(t, "DATA_ANALYST", role.Status.ShowOutput.Name)
}

func TestApplyObservation_PreservesCreatedOn(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")

	obs := successfulObservation()
	obs.ShowOutput.CreatedOn = "2024-01-01"

	applyObservation(role, obs)

	assert.Equal(t, "2024-01-01", role.Status.ShowOutput.CreatedOn)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	role := &snowplanev1alpha1.AccountRole{
		Spec: snowplanev1alpha1.AccountRoleSpec{
			Comment: testutil.Ptr("test"),
		},
	}

	obs := &snowflake.AccountRoleObservation{
		ShowOutput: &snowplanev1alpha1.AccountRoleShowOutput{Comment: "test"},
	}

	result := detectDrift(role, obs)
	assert.False(t, result.HasDrift)
	assert.Empty(t, result.Changes)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	role := &snowplanev1alpha1.AccountRole{
		Spec: snowplanev1alpha1.AccountRoleSpec{
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.AccountRoleObservation{
		ShowOutput: &snowplanev1alpha1.AccountRoleShowOutput{Comment: "drifted"},
	}

	result := detectDrift(role, obs)
	assert.True(t, result.HasDrift)
	assert.Len(t, result.Changes, 1)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Generation = 1
	role.Status.ObservedGeneration = 1
	role.Spec.Comment = testutil.Ptr("desired comment")
	hash, err := snowplanev1alpha1.ComputeSpecHash(role.Spec)
	require.NoError(t, err)
	role.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted comment"

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAccountRoleOptions) error {
			alterCalled = true
			assert.Equal(t, "desired comment", *opts.Comment)
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err = r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter should be called for drift correction")

	recorder := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(recorder)
	assert.True(t, testutil.ContainsEvent(events, "DriftDetected"))
	assert.True(t, testutil.ContainsEvent(events, "DriftCorrected"))
}

func TestReconcile_DriftDetectOnlyPolicy(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Generation = 1
	role.Status.ObservedGeneration = 1
	role.Spec.ManagementPolicies.DriftPolicy = snowplanev1alpha1.DriftPolicyDetectOnly
	role.Spec.Comment = testutil.Ptr("desired")
	hash, err := snowplanev1alpha1.ComputeSpecHash(role.Spec)
	require.NoError(t, err)
	role.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted"

	alterCalled := false

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterAccountRoleOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeDriftDetected))
}

// --------------------------------------------------------------------------
// Tests: Recoverable condition
// --------------------------------------------------------------------------

func TestReconcile_RecoverableConditionOnTransientError(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_RecoverableClearedOnSuccess(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1

	conditions.SetNotReady(role, snowplanev1alpha1.ReasonReconcileError, "previous error")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsRecoverable(got))
}

func TestReconcile_ClearTerminal_OnlyOnSuccess(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1
	conditions.SetNotReady(role, "PreviousError", "old error")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.False(t, conditions.IsTerminal(got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AccountRoleObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
	assert.Contains(t, events[0], "created")
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Deleting")
}

// --------------------------------------------------------------------------
// Tests: TrackedParameters persistence
// --------------------------------------------------------------------------

func TestReconcile_TrackedParametersPersistedOnCreate(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Spec.Comment = testutil.Ptr("hello")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "hello"

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AccountRoleObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateAccountRoleOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.AccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.ElementsMatch(t, []string{"COMMENT"}, got.Status.TrackedParameters)
}

func TestReconcile_UnsetTriggered(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Generation = 2
	role.Status.ObservedGeneration = 1
	role.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	obs.ShowOutput.Comment = "original"

	var capturedAlterOpts snowflake.AlterAccountRoleOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAccountRoleOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	assert.Contains(t, capturedAlterOpts.UnsetFields, "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Ownership (USE ROLE)
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	role := newTestRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Generation = 1
	role.Status.ObservedGeneration = 1
	role.Spec.UseRole = testutil.Ptr("USERADMIN")

	obs := successfulObservation()
	obs.ShowOutput.Owner = "USERADMIN"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
			return obs, nil
		},
	}

	var capturedUseRole string

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.AccountRole{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(role, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	r := NewReconcilerWithServiceFactory(c, testutil.NewTestClientFactory(), record.NewFakeRecorder(100), nil,
		func(_ context.Context, _ SnowflakeClient, useRole string) (Service, func(context.Context), error) {
			capturedUseRole = useRole
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("AccountRole")

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	assert.Equal(t, "USERADMIN", capturedUseRole, "useRole from spec should be passed to ServiceFactory")
}

// --------------------------------------------------------------------------
// Tests: RequeueAfter constant
// --------------------------------------------------------------------------

func TestRequeueInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5*time.Minute, reconciler.DefaultRequeueInterval)
}

func TestWithRequeueInterval(t *testing.T) {
	t.Parallel()

	r := NewReconciler(nil, nil, nil, nil)
	// WithRequeueInterval returns the reconciler for chaining.
	assert.NotNil(t, r.WithRequeueInterval(10*time.Minute))
}
