package apiauthenticationintegrationwithjwtbearer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
}
func (m *mockService) Create(ctx context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}
func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error {
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

func newTestIntegration(name, namespace string) *snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer {
	return &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Generation: 1},
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearerSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                 "MY_JWT_AUTH",
			Enabled:              true,
			OAuthClientID:        "jwt-client",
			OAuthClientSecret:    "jwt-secret",
			OAuthAssertionIssuer: "https://issuer.example.com",
		},
	}
}

func successfulObservation() *snowflake.APIAuthenticationIntegrationObservation {
	return &snowflake.APIAuthenticationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowflake.SecurityIntegrationShowOutput{
			CreatedOn: "2024-01-01", Name: "MY_JWT_AUTH", Type: "API_AUTHENTICATION", Category: "SECURITY", Enabled: true,
		},
		DescribeOutput: map[string]string{
			"AUTH_TYPE": "OAUTH2", "OAUTH_GRANT": "JWT_BEARER", "OAUTH_ASSERTION_ISSUER": "https://issuer.example.com",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer, Service, *snowflake.APIAuthenticationIntegrationObservation]{
		Client:   cb.Build(),
		Factory:  clientfactory.NewClientFactory(),
		Recorder: record.NewFakeRecorder(100),
		Adapter: &adapter{
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("APIAuthenticationIntegrationWithJWTBearer"),
	}
}

func TestReconcile_StandardSuite(t *testing.T) {
	t.Parallel()
	testutil.ReconcileSuiteConfig{
		NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
			r := newTestReconciler(&mockService{}, objs...)
			return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
		},
		NewFixture:     func(name, ns string) client.Object { return newTestIntegration(name, ns) },
		NewBlankObject: func() client.Object { return &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{} },
		FinalizerName:  finalizerName,
	}.Run(t)
}

func TestReconcile_Create(t *testing.T) {
	t.Parallel()
	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateAPIAuthenticationIntegrationOptions
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAPIAuthenticationIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, snowflake.OAuthGrantTypeJWTBearer, capturedOpts.OAuthGrantType)
	assert.NotNil(t, capturedOpts.OAuthAssertionIssuer)
	assert.Equal(t, "https://issuer.example.com", *capturedOpts.OAuthAssertionIssuer)

	got := &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myint", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()
	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
			return fmt.Errorf("access denied")
		},
	}
	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.Error(t, err)
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()
	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}
	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Generation = 2
	obj.Spec.Comment = testutil.PtrString("updated")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedOpts snowflake.AlterAPIAuthenticationIntegrationOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAPIAuthenticationIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated", *capturedOpts.Comment)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Generation = 2
	obj.Spec.Comment = testutil.PtrString("change")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterAPIAuthenticationIntegrationOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	now := metav1.Now()
	obj.DeletionTimestamp = &now

	dropCalled := false
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return successfulObservation(), nil
		},
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
	assert.True(t, dropCalled)
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	obj.DeletionTimestamp = &now

	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	now := metav1.Now()
	obj.DeletionTimestamp = &now

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return successfulObservation(), nil
		},
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable name
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Spec.Name = "RENAMED_AUTH"
	obj.Status.ShowOutput = &snowplanev1alpha1.APIAuthenticationIntegrationShowOutput{
		Name: testutil.PtrString("MY_JWT_AUTH"),
	}

	obs := successfulObservation()
	obs.ShowOutput.Name = "MY_JWT_AUTH"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myint", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()
	obj := newTestIntegration("myint", "default")
	id := snowflake.NewAccountObjectIdentifier("MY_JWT_AUTH")
	opts := buildCreateOptions(obj, id)
	assert.Equal(t, snowflake.OAuthGrantTypeJWTBearer, opts.OAuthGrantType)
	assert.NotNil(t, opts.OAuthAssertionIssuer)
	assert.Equal(t, "https://issuer.example.com", *opts.OAuthAssertionIssuer)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()
	obj := newTestIntegration("myint", "default")
	obs := successfulObservation()
	applyObservation(obj, obs)
	assert.Equal(t, "MY_JWT_AUTH", obj.Status.FullyQualifiedName)
	assert.NotNil(t, obj.Status.DescribeOutput)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()
	obj := &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearerSpec{
			Name: "MY_JWT_AUTH", Enabled: true, OAuthAssertionIssuer: "https://issuer.example.com",
		},
	}
	obs := &snowflake.APIAuthenticationIntegrationObservation{
		ShowOutput:     &snowflake.SecurityIntegrationShowOutput{Name: "MY_JWT_AUTH", Enabled: true},
		DescribeOutput: map[string]string{"OAUTH_ASSERTION_ISSUER": "https://issuer.example.com"},
	}
	result := detectDrift(obj, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearer{
		Spec: snowplanev1alpha1.APIAuthenticationIntegrationWithJWTBearerSpec{
			Name:                 "MY_JWT_AUTH",
			Enabled:              true,
			OAuthAssertionIssuer: "https://issuer.example.com",
			Comment:              testutil.PtrString("desired"),
		},
	}

	obs := &snowflake.APIAuthenticationIntegrationObservation{
		ShowOutput: &snowflake.SecurityIntegrationShowOutput{
			Name:    "MY_JWT_AUTH",
			Enabled: true,
			Comment: "drifted",
		},
		DescribeOutput: map[string]string{
			"OAUTH_ASSERTION_ISSUER": "https://issuer.example.com",
		},
	}

	result := detectDrift(obj, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	obj := newTestIntegration("myint", "default")
	obj.Finalizers = []string{finalizerName}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIAuthenticationIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.APIAuthenticationIntegrationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateAPIAuthenticationIntegrationOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myint", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
