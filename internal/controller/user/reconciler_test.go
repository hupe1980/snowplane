package user

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
// mockService
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateUserOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterUserOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.UserObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateUserOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterUserOptions) error {
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

func newTestUser(name, namespace string) *snowplanev1alpha1.User {
	return &snowplanev1alpha1.User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snowplane.hupe1980.github.io/v1alpha1",
			Kind:       "User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
			UID:        "test-uid-1234",
		},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "ALICE",
		},
	}
}

func successfulObservation() *snowflake.UserObservation {
	return &snowflake.UserObservation{
		Exists: true,
		ShowOutput: &snowflake.UserShowOutput{
			CreatedOn:   "2024-01-01",
			Name:        "ALICE",
			LoginName:   "ALICE",
			DisplayName: "ALICE",
			Owner:       "USERADMIN",
			Type:        "PERSON",
		},
		DescribeOutput: &snowflake.UserDescribeOutput{},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.User{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client: c,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("User"),
	}
}

// --------------------------------------------------------------------------
// CR Not Found
// --------------------------------------------------------------------------

func TestReconcile_CRNotFound(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(&mockService{})
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --------------------------------------------------------------------------
// Provider Config
// --------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	r := newTestReconciler(&mockService{}, user)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

func TestReconcile_ProviderConfigNotReady(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	pc := testutil.NewTestPC("default")
	pc.Status.Conditions = nil

	r := newTestReconciler(&mockService{}, user, pc, testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProviderConfig not ready")
}

// --------------------------------------------------------------------------
// Finalizer
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.Contains(t, updated.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Create Flow
// --------------------------------------------------------------------------

func TestReconcile_CreateUser(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.True(t, conditions.IsTrue(updated, "Ready"))
	assert.True(t, conditions.IsTrue(updated, "Synced"))
	assert.Equal(t, "USERADMIN", updated.Status.ShowOutput.Owner)
	assert.Equal(t, `"ALICE"`, updated.Status.FullyQualifiedName)
}

func TestReconcile_CreateWithAllFields(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Spec.LoginName = testutil.PtrString("alice_login")
	user.Spec.DisplayName = testutil.PtrString("Alice")
	user.Spec.Email = testutil.PtrString("alice@example.com")
	user.Spec.Comment = testutil.PtrString("Test user")

	var capturedOpts snowflake.CreateUserOptions
	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return successfulObservation(), nil
		},
		createFn: func(_ context.Context, opts snowflake.CreateUserOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	assert.Equal(t, "alice_login", *capturedOpts.LoginName)
	assert.Equal(t, "Alice", *capturedOpts.DisplayName)
	assert.Equal(t, "alice@example.com", *capturedOpts.Email)
	assert.Equal(t, "Test user", *capturedOpts.Comment)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateUserOptions) error {
			return fmt.Errorf("transient network failure")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.False(t, conditions.IsTrue(updated, "Ready"))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateUserOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("insufficient privileges"))
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.True(t, conditions.IsTerminal(updated))
}

func TestReconcile_CreatePostObserveError(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return nil, fmt.Errorf("post-create observe failed")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
}

// --------------------------------------------------------------------------
// Update Flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Generation = 2
	user.Spec.Email = testutil.PtrString("alice@newdomain.com")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}

	var capturedAlterOpts snowflake.AlterUserOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterUserOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	require.NotNil(t, capturedAlterOpts.Email)
	assert.Equal(t, "alice@newdomain.com", *capturedAlterOpts.Email)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Spec.Email = testutil.PtrString("alice@example.com")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterUserOptions) error {
			return fmt.Errorf("transient")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)
}

func TestReconcile_AlterTerminalError(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Spec.Email = testutil.PtrString("alice@example.com")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterUserOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("permanent"))
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.True(t, conditions.IsTerminal(updated))
}

// --------------------------------------------------------------------------
// Observe Errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return nil, fmt.Errorf("observe failure")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.False(t, conditions.IsTrue(updated, "Ready"))
	assert.True(t, conditions.IsRecoverable(updated))
}

// --------------------------------------------------------------------------
// Delete Flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteUser(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now

	var dropped bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.True(t, dropped)

	updated := &snowplanev1alpha1.User{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now
	user.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	var dropped bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.False(t, dropped, "orphan policy should not drop the user")
}

func TestReconcile_DeleteAlreadyGone(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return snowflake.ErrObjectNotFound
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("failed to drop")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)
}

func TestReconcile_DeleteNoFinalizer(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{"some-other-finalizer"}
	user.DeletionTimestamp = &now

	r := newTestReconciler(&mockService{}, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now

	r := newTestReconciler(&mockService{}, user)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updated := &snowplanev1alpha1.User{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated)
	assert.True(t, apierrors.IsNotFound(err))
}

// --------------------------------------------------------------------------
// Immutable Fields
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Generation = 2
	user.Spec.Name = "BOB"
	user.Status.ShowOutput = &snowplanev1alpha1.UserShowOutput{Name: "ALICE"}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.True(t, conditions.IsTerminal(updated))
}

func TestReconcile_ImmutableType(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Generation = 2
	newType := snowplanev1alpha1.UserTypeService
	user.Spec.Type = &newType
	user.Status.ShowOutput = &snowplanev1alpha1.UserShowOutput{Name: "ALICE", Type: "PERSON"}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")
}

func TestValidateImmutableFields_FirstReconcile(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Status.ObservedGeneration = 0
	user.Spec.Name = "CHANGED"

	err := (&adapter{}).ValidateImmutableFields(context.Background(), user)
	assert.NoError(t, err, "first reconcile should skip immutable checks")
}

func TestValidateImmutableFields_NoShowOutput(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Status.ObservedGeneration = 1
	user.Status.ShowOutput = nil

	err := (&adapter{}).ValidateImmutableFields(context.Background(), user)
	assert.NoError(t, err, "missing ShowOutput should not cause error")
}

// --------------------------------------------------------------------------
// Unit: buildCreateOptions
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.Email = testutil.PtrString("alice@example.com")
	user.Spec.Comment = testutil.PtrString("Test")

	r := newTestReconciler(&mockService{})
	opts, err := buildCreateOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"))
	require.NoError(t, err)

	assert.Equal(t, "ALICE", opts.Name.Name())
	assert.Equal(t, "alice@example.com", *opts.Email)
	assert.Equal(t, "Test", *opts.Comment)
}

func TestBuildCreateOptions_Minimal(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")

	r := newTestReconciler(&mockService{})
	opts, err := buildCreateOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"))
	require.NoError(t, err)

	assert.Equal(t, "ALICE", opts.Name.Name())
	assert.Nil(t, opts.LoginName)
	assert.Nil(t, opts.Email)
	assert.Nil(t, opts.Password)
}

func TestBuildCreateOptions_WithType(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	userType := snowplanev1alpha1.UserTypeService
	user.Spec.Type = &userType

	r := newTestReconciler(&mockService{})
	opts, err := buildCreateOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("SVC_USER"))
	require.NoError(t, err)

	require.NotNil(t, opts.Type)
	assert.Equal(t, "SERVICE", *opts.Type)
}

// --------------------------------------------------------------------------
// Unit: buildAlterOptions
// --------------------------------------------------------------------------

func TestBuildAlterOptions_EmailChanged(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.Email = testutil.PtrString("alice@new.com")

	obs := successfulObservation()
	r := newTestReconciler(&mockService{})
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)

	require.NotNil(t, opts.Email)
	assert.Equal(t, "alice@new.com", *opts.Email)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	obs := successfulObservation()

	r := newTestReconciler(&mockService{})
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_UnsetComment(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.Comment = nil
	user.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	r := newTestReconciler(&mockService{})
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	assert.Contains(t, opts.UnsetFields, "COMMENT")
}

func TestBuildAlterOptions_PasswordChanged(t *testing.T) {
	t.Parallel()

	pwSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-password", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("newpassword123")},
	}

	user := newTestUser("test-user", "default")
	user.Spec.Password = &snowplanev1alpha1.SecretKeyReference{
		Name: "user-password", Namespace: "default", Key: "password",
	}
	user.Status.LastAppliedPasswordHash = hashSecret("oldpassword", string(user.UID))

	obs := successfulObservation()

	r := newTestReconciler(&mockService{}, user, pwSecret)
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	require.NotNil(t, opts.Password, "password should be included when hash differs")
	assert.Equal(t, "newpassword123", *opts.Password)
}

func TestBuildAlterOptions_PasswordUnchanged(t *testing.T) {
	t.Parallel()

	pw := "samepassword"
	pwSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-password", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte(pw)},
	}

	user := newTestUser("test-user", "default")
	user.Spec.Password = &snowplanev1alpha1.SecretKeyReference{
		Name: "user-password", Namespace: "default", Key: "password",
	}
	user.Status.LastAppliedPasswordHash = hashSecret(pw, string(user.UID))

	obs := successfulObservation()

	r := newTestReconciler(&mockService{}, user, pwSecret)
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	assert.Nil(t, opts.Password, "password should be skipped when hash matches")
}

func TestHashSecret(t *testing.T) {
	t.Parallel()

	// Deterministic
	h1 := hashSecret("test", "key1")
	h2 := hashSecret("test", "key1")
	assert.Equal(t, h1, h2)

	// Different inputs produce different hashes
	h3 := hashSecret("different", "key1")
	assert.NotEqual(t, h1, h3)

	// Different keys produce different hashes (HMAC property)
	h4 := hashSecret("test", "key2")
	assert.NotEqual(t, h1, h4)

	// Returns a 64-char hex string (HMAC-SHA256)
	assert.Len(t, h1, 64)
}

// --------------------------------------------------------------------------
// Unit: RSA key change detection (L-12)
// --------------------------------------------------------------------------

func TestBuildAlterOptions_RSAKeyChanged(t *testing.T) {
	t.Parallel()

	rsaKey := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQ"
	rsaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rsa-key", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte(rsaKey)},
	}

	user := newTestUser("test-user", "default")
	user.Spec.RSAPublicKey = &snowplanev1alpha1.SecretKeyReference{
		Name: "rsa-key", Namespace: "default", Key: "key",
	}
	user.Status.LastAppliedRSAPublicKeyHash = hashSecret("oldkey", string(user.UID))

	obs := successfulObservation()

	r := newTestReconciler(&mockService{}, user, rsaSecret)
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	require.NotNil(t, opts.RSAPublicKey, "RSA key should be included when hash differs")
	assert.Equal(t, rsaKey, *opts.RSAPublicKey)
}

func TestBuildAlterOptions_RSAKeyUnchanged(t *testing.T) {
	t.Parallel()

	rsaKey := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQ"
	rsaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rsa-key", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte(rsaKey)},
	}

	user := newTestUser("test-user", "default")
	user.Spec.RSAPublicKey = &snowplanev1alpha1.SecretKeyReference{
		Name: "rsa-key", Namespace: "default", Key: "key",
	}
	user.Status.LastAppliedRSAPublicKeyHash = hashSecret(rsaKey, string(user.UID))

	obs := successfulObservation()

	r := newTestReconciler(&mockService{}, user, rsaSecret)
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	assert.Nil(t, opts.RSAPublicKey, "RSA key should be skipped when hash matches")
}

func TestBuildAlterOptions_RSAKey2Changed(t *testing.T) {
	t.Parallel()

	rsaKey2 := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCBB"
	rsaSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rsa-key2", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte(rsaKey2)},
	}

	user := newTestUser("test-user", "default")
	user.Spec.RSAPublicKey2 = &snowplanev1alpha1.SecretKeyReference{
		Name: "rsa-key2", Namespace: "default", Key: "key",
	}
	user.Status.LastAppliedRSAPublicKey2Hash = hashSecret("oldkey2", string(user.UID))

	obs := successfulObservation()

	r := newTestReconciler(&mockService{}, user, rsaSecret)
	opts, err := buildAlterOptions(context.Background(), r.Client, user, snowflake.NewAccountObjectIdentifier("ALICE"), obs)
	require.NoError(t, err)
	require.NotNil(t, opts.RSAPublicKey2, "RSA key 2 should be included when hash differs")
	assert.Equal(t, rsaKey2, *opts.RSAPublicKey2)
}

// --------------------------------------------------------------------------
// Unit: PostUpdate RSA key hash tracking
// --------------------------------------------------------------------------

func TestPostUpdate_RSAKeyHashTracked(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.RSAPublicKey = &snowplanev1alpha1.SecretKeyReference{
		Name: "rsa-key", Namespace: "default", Key: "key",
	}

	rsaKey := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQ"
	alterOpts := &snowflake.AlterUserOptions{RSAPublicKey: &rsaKey}

	a := &adapter{}
	a.PostUpdate(user, true, alterOpts)
	assert.Equal(t, hashSecret(rsaKey, string(user.UID)), user.Status.LastAppliedRSAPublicKeyHash)
}

func TestPostUpdate_RSAKeyHashClearedOnRemoval(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Status.LastAppliedRSAPublicKeyHash = "oldhash"
	// RSAPublicKey is nil in spec (removed)

	a := &adapter{}
	a.PostUpdate(user, false, &snowflake.AlterUserOptions{})
	assert.Empty(t, user.Status.LastAppliedRSAPublicKeyHash)
}

// --------------------------------------------------------------------------
// Unit: computeTrackedParameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.UserSpec{
		Email:   testutil.PtrString("alice@example.com"),
		Comment: testutil.PtrString("test"),
	}

	fields := computeTrackedParameters(spec)
	assert.Contains(t, fields, "EMAIL")
	assert.Contains(t, fields, "COMMENT")
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.UserSpec{}
	fields := computeTrackedParameters(spec)
	assert.Empty(t, fields)
}

// --------------------------------------------------------------------------
// Unit: applyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	obs := successfulObservation()

	applyObservation(user, obs)
	assert.Equal(t, `"ALICE"`, user.Status.FullyQualifiedName)
	assert.Equal(t, "USERADMIN", user.Status.ShowOutput.Owner)
	assert.Equal(t, "2024-01-01", user.Status.ShowOutput.CreatedOn)
	assert.NotNil(t, user.Status.ShowOutput)
	assert.NotNil(t, user.Status.DescribeOutput)
}

func TestApplyObservation_PreservesCreatedOn(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")

	obs := successfulObservation()
	obs.ShowOutput.CreatedOn = "2024-01-01"

	applyObservation(user, obs)
	assert.Equal(t, "2024-01-01", user.Status.ShowOutput.CreatedOn)
}

// --------------------------------------------------------------------------
// Unit: computeUnsetFields
// --------------------------------------------------------------------------

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.Email = nil
	user.Spec.Comment = nil
	user.Spec.LoginName = testutil.PtrString("alice_login")
	user.Status.TrackedParameters = []string{"EMAIL", "COMMENT", "LOGIN_NAME"}

	unset := computeUnsetFields(user)
	assert.Contains(t, unset, "EMAIL")
	assert.Contains(t, unset, "COMMENT")
	assert.NotContains(t, unset, "LOGIN_NAME")
}

func TestComputeUnsetFields_NoTrackedParameters(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Status.TrackedParameters = nil
	unset := computeUnsetFields(user)
	assert.Nil(t, unset)
}

// --------------------------------------------------------------------------
// Drift Detection
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	obs := successfulObservation()

	result := detectDrift(user, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Spec.Email = testutil.PtrString("alice@expected.com")

	obs := successfulObservation()
	obs.ShowOutput.Email = "alice@actual.com"

	result := detectDrift(user, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "EMAIL")
}

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Generation = 1
	user.Spec.Email = testutil.PtrString("correct@example.com")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}
	hash, err := snowplanev1alpha1.ComputeSpecHash(user.Spec)
	require.NoError(t, err)
	user.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Email = "wrong@example.com"

	var alterCalled bool
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterUserOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err = r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled, "drift should trigger alter")

	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	assert.True(t, testutil.ContainsEvent(events, snowplanev1alpha1.ReasonDriftCorrected))
}

func TestReconcile_DriftDetectOnlyPolicy(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Generation = 1
	user.Annotations = map[string]string{
		"snowplane.hupe1980.github.io/drift-policy": "detect-only",
	}
	user.Spec.Email = testutil.PtrString("correct@example.com")
	hash, err := snowplanev1alpha1.ComputeSpecHash(user.Spec)
	require.NoError(t, err)
	user.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Email = "wrong@example.com"

	var alterCalled bool
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterUserOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err = r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "detect-only policy should not call alter")
}

// --------------------------------------------------------------------------
// Recoverable / Terminal conditions
// --------------------------------------------------------------------------

func TestReconcile_RecoverableConditionOnTransientError(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return nil, fmt.Errorf("transient failure")
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.Error(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.True(t, conditions.IsRecoverable(updated))
}

func TestReconcile_RecoverableClearedOnSuccess(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.False(t, conditions.IsRecoverable(updated))
}

// --------------------------------------------------------------------------
// Event Emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}

	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	assert.True(t, testutil.ContainsEvent(events, snowplanev1alpha1.ReasonCreating))
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.DeletionTimestamp = &now

	r := newTestReconciler(&mockService{}, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	rec := r.Recorder.(*record.FakeRecorder)
	events := testutil.DrainEvents(rec)
	assert.True(t, testutil.ContainsEvent(events, snowplanev1alpha1.ReasonDeleting))
}

// --------------------------------------------------------------------------
// TrackedParameters
// --------------------------------------------------------------------------

func TestReconcile_TrackedParametersPersistedOnCreate(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Spec.Email = testutil.PtrString("alice@example.com")

	call := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			call++
			if call == 1 {
				return &snowflake.UserObservation{Exists: false}, nil
			}
			return successfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)

	updated := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, updated))
	assert.Contains(t, updated.Status.TrackedParameters, "EMAIL")
}

func TestReconcile_UnsetTriggered(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Status.ObservedGeneration = 1
	user.Status.TrackedParameters = []string{"COMMENT"}
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationUseCreateOrAlter: "false",
	}

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedOpts snowflake.AlterUserOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterUserOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Contains(t, capturedOpts.UnsetFields, "COMMENT")
}

// --------------------------------------------------------------------------
// Ownership Drift
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Spec.UseRole = testutil.PtrString("USERADMIN")

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return successfulObservation(), nil
		},
	}

	scheme := testutil.TestScheme()
	kClient := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(user, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		WithStatusSubresource(&snowplanev1alpha1.User{}, &snowplanev1alpha1.ProviderConfig{}).
		Build()

	var capturedRole string

	rec := record.NewFakeRecorder(100)

	r := &reconciler.GenericReconciler[*snowplanev1alpha1.User, Service, *snowflake.UserObservation]{
		Client:   kClient,
		Factory:  clientfactory.NewClientFactory(),
		Recorder: rec,
		Adapter: &adapter{
			client: kClient,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, useRole string) (Service, func(context.Context), error) {
				capturedRole = useRole
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("User"),
	}

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	require.NoError(t, err)
	assert.Equal(t, "USERADMIN", capturedRole)
}

// --------------------------------------------------------------------------
// Constants
// --------------------------------------------------------------------------

func TestRequeueInterval(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5*time.Minute, reconciler.DefaultRequeueInterval)
}

func TestWithRequeueInterval(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(&mockService{})
	r2 := r.WithRequeueInterval(10 * time.Minute)
	assert.NotNil(t, r2, "WithRequeueInterval should return non-nil")
}

// --------------------------------------------------------------------------
// Tests: ForceNew annotation bypasses immutable field checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_ForceNewBypass(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "true",
	}
	user.Status.ObservedGeneration = 1
	user.Spec.Name = "BOB"
	svcType := snowplanev1alpha1.UserTypeService
	user.Spec.Type = &svcType
	user.Status.ShowOutput = &snowplanev1alpha1.UserShowOutput{
		Name: "ALICE",
		Type: "PERSON",
	}

	err := (&adapter{}).ValidateImmutableFields(context.Background(), user)
	assert.NoError(t, err, "force-new should bypass immutable checks")
}

func TestValidateImmutableFields_ForceNewFalse_StillRejects(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "false",
	}
	user.Status.ObservedGeneration = 1
	user.Spec.Name = "BOB"
	user.Status.ShowOutput = &snowplanev1alpha1.UserShowOutput{
		Name: "ALICE",
	}

	err := (&adapter{}).ValidateImmutableFields(context.Background(), user)
	assert.Error(t, err, "force-new=false should still reject immutable changes")
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// --------------------------------------------------------------------------
// Tests: Spec validation defense-in-depth
// --------------------------------------------------------------------------

func TestReconcile_SpecValidation_RejectsInvalidType(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	bad := snowplanev1alpha1.UserType("INVALID")
	user.Spec.Type = &bad

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	assert.NoError(t, err, "should return nil (terminal, no requeue)")

	got := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))

	termCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, termCond)
	assert.Contains(t, termCond.Message, "spec.type must be one of")
}

func TestReconcile_SpecValidation_RejectsPasswordMissingKey(t *testing.T) {
	t.Parallel()

	user := newTestUser("test-user", "default")
	user.Finalizers = []string{finalizerName}
	user.Spec.Password = &snowplanev1alpha1.SecretKeyReference{Name: "my-secret"}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.UserObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, user, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("test-user", "default"))
	assert.NoError(t, err, "should return nil (terminal, no requeue)")

	got := &snowplanev1alpha1.User{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "test-user", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))

	termCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, termCond)
	assert.Contains(t, termCond.Message, "spec.password.key is required")
}
