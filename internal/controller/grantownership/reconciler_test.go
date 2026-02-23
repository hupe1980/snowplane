package grantownership

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
	observeFn func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error
	dropFn    func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}
	return &snowflake.GrantOwnershipObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Drop(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, id)
	}
	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestGrantOwnership(name, namespace string) *snowplanev1alpha1.GrantOwnership {
	return &snowplanev1alpha1.GrantOwnership{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			ObjectType:  "DATABASE",
			ObjectName:  "MY_DB",
			AccountRole: "DATA_ADMIN",
		},
	}
}

func successfulObservation() *snowflake.GrantOwnershipObservation {
	return &snowflake.GrantOwnershipObservation{
		Exists: true,
		ShowOutput: &snowflake.GrantOwnershipShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   "OWNERSHIP",
			GrantedOn:   "DATABASE",
			Name:        "MY_DB",
			GrantedTo:   "ROLE",
			GranteeName: "DATA_ADMIN",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.GrantOwnership{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("GrantOwnership"),
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

	g := newTestGrantOwnership("mygo", "default")
	r := newTestReconciler(&mockService{}, g)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

// --------------------------------------------------------------------------
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateGrantOwnershipOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
				call++
				if call == 1 {
					return &snowflake.GrantOwnershipObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateGrantOwnershipOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "DATABASE", capturedOpts.ObjectType)
	assert.Equal(t, "MY_DB", capturedOpts.ObjectName)
	assert.Contains(t, capturedOpts.ToRole, "DATA_ADMIN")

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, "DATA_ADMIN", got.Status.RoleName)
}

func TestReconcile_CreateWithCurrentGrantsBehavior(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	behavior := snowplanev1alpha1.CurrentGrantsBehavior("COPY")
	g.Spec.CurrentGrantsBehavior = &behavior

	var capturedOpts snowflake.CreateGrantOwnershipOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
				call++
				if call == 1 {
					return &snowflake.GrantOwnershipObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateGrantOwnershipOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)

	assert.Equal(t, "COPY", capturedOpts.CurrentGrantsBehavior)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			return &snowflake.GrantOwnershipObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateGrantOwnershipOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			return &snowflake.GrantOwnershipObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateGrantOwnershipOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Update flow (all immutable — no alter)
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	g.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Observe errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	now := metav1.Now()
	g.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.GrantOwnership{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	g.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	g.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	now := metav1.Now()
	g.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableGrantee(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}
	g.Spec.AccountRole = "NEW_ROLE"
	g.Status.ObservedGeneration = 1
	g.Status.ShowOutput = &snowplanev1alpha1.GrantOwnershipShowOutput{
		GranteeName: "OLD_ROLE",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.GrantOwnership{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mygo", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestOwnershipAlterOptions_NeverHasChanges(t *testing.T) {
	t.Parallel()

	opts := &ownershipAlterOptions{}
	assert.False(t, opts.HasChanges())
}

func TestResolveGranteeName_AccountRole(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			AccountRole: "ADMIN",
		},
	}
	assert.Equal(t, "ADMIN", resolveGranteeName(g))
}

func TestResolveGranteeName_DatabaseRole(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			DatabaseRole: "DB_READER",
		},
	}
	assert.Equal(t, "DB_READER", resolveGranteeName(g))
}

func TestResolveGranteeName_Empty(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{},
	}
	assert.Equal(t, "", resolveGranteeName(g))
}

func TestBuildToRole(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			AccountRole: "ADMIN",
		},
	}
	result := buildToRole(g)
	assert.Contains(t, result, "ADMIN")
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	obs := successfulObservation()

	applyObservation(g, obs)

	assert.Equal(t, "DATA_ADMIN", g.Status.RoleName)
	require.NotNil(t, g.Status.ShowOutput)
	assert.Equal(t, "OWNERSHIP", g.Status.ShowOutput.Privilege)
	assert.Equal(t, "DATA_ADMIN", g.Status.ShowOutput.GranteeName)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			AccountRole: "DATA_ADMIN",
		},
	}

	obs := &snowflake.GrantOwnershipObservation{
		ShowOutput: &snowflake.GrantOwnershipShowOutput{
			GranteeName: "DATA_ADMIN",
		},
	}

	result := detectDrift(g, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	g := &snowplanev1alpha1.GrantOwnership{
		Spec: snowplanev1alpha1.GrantOwnershipSpec{
			AccountRole: "ADMIN",
		},
	}

	obs := &snowflake.GrantOwnershipObservation{
		ShowOutput: &snowflake.GrantOwnershipShowOutput{
			GranteeName: "OTHER_ROLE",
		},
	}

	result := detectDrift(g, obs)
	assert.True(t, result.HasImmutableViolation)
	assert.Contains(t, result.Summary(), "GRANTEE")
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	g := newTestGrantOwnership("mygo", "default")
	g.Finalizers = []string{finalizerName}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error) {
				call++
				if call == 1 {
					return &snowflake.GrantOwnershipObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateGrantOwnershipOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, g, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mygo", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
