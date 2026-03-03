package secretwithclientcredentials

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

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecretOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecretOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.SecretObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSecretOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSecretOptions) error {
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

func newTestSecret(name, namespace string) *snowplanev1alpha1.SecretWithClientCredentials {
	return &snowplanev1alpha1.SecretWithClientCredentials{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SecretWithClientCredentialsSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:              "MY_SECRET",
			DatabaseName:      testutil.Ptr("MY_DB"),
			SchemaName:        testutil.Ptr("MY_SCHEMA"),
			APIAuthentication: "my_integration",
			OAuthScopes:       []string{"session:role:analyst"},
		},
	}
}

func successfulObservation() *snowflake.SecretObservation {
	return &snowflake.SecretObservation{
		Exists: true,
		ShowOutput: &snowflake.SecretShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_SECRET",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			SecretType:   "OAUTH2",
			OAuthScopes:  "session:role:analyst",
		},
		DescribeOutput: map[string]string{
			"secret_type":      "OAUTH2",
			"integration_name": "my_integration",
			"oauth_scopes":     "session:role:analyst",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithClientCredentials, Service, *snowflake.SecretObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.SecretWithClientCredentials{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.SecretWithClientCredentials, Service, *snowflake.SecretObservation]{
		Client:   c,
		Factory:  clientfactory.NewClientFactory(),
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("SecretWithClientCredentials"),
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
			return newTestSecret(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SecretWithClientCredentials{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateSecretOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SecretObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSecretOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, snowflake.SecretTypeOAuth2, capturedOpts.SecretType)
	assert.Equal(t, "my_integration", capturedOpts.APIAuthentication)
	assert.Equal(t, []string{"session:role:analyst"}, capturedOpts.OAuthScopes)
	assert.Equal(t, "MY_SECRET", capturedOpts.Name.Name())

	got := &snowplanev1alpha1.SecretWithClientCredentials{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mysecret", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return &snowflake.SecretObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSecretOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return &snowflake.SecretObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSecretOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("object already exists"))
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err) // terminal errors are not requeued

	got := &snowplanev1alpha1.SecretWithClientCredentials{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mysecret", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Generation = 2
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"
	obj.Spec.Comment = testutil.Ptr("updated")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedOpts snowflake.AlterSecretOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSecretOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated", *capturedOpts.Comment)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Generation = 2
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"
	obj.Spec.Comment = testutil.Ptr("change")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSecretOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.DeletionTimestamp = &now
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	dropCalled := false
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return successfulObservation(), nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)
	assert.True(t, dropCalled)
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestSecret("mysecret", "default")
	obj.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	obj.Finalizers = []string{finalizerName}
	obj.DeletionTimestamp = &now
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	dropCalled := false
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return successfulObservation(), nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.DeletionTimestamp = &now
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return successfulObservation(), nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable name
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"
	obj.Spec.Name = "RENAMED_SECRET"
	obj.Status.ShowOutput = &snowplanev1alpha1.SecretShowOutput{
		Name:         testutil.Ptr("MY_SECRET"),
		DatabaseName: testutil.Ptr("MY_DB"),
		SchemaName:   testutil.Ptr("MY_SCHEMA"),
	}

	obs := successfulObservation()
	obs.ShowOutput.Name = "MY_SECRET"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.SecretWithClientCredentials{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mysecret", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Unit – buildCreateOptions
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_SECRET")

	opts := buildCreateOptions(obj, id)
	assert.Equal(t, snowflake.SecretTypeOAuth2, opts.SecretType)
	assert.Equal(t, "my_integration", opts.APIAuthentication)
	assert.Equal(t, []string{"session:role:analyst"}, opts.OAuthScopes)
	assert.Equal(t, "MY_SECRET", opts.Name.Name())
}

func TestBuildCreateOptions_WithComment(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Spec.Comment = testutil.Ptr("my comment")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_SECRET")

	opts := buildCreateOptions(obj, id)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "my comment", *opts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Unit – applyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "\"MY_DB\".\"MY_SCHEMA\".\"MY_SECRET\"", obj.Status.FullyQualifiedName)
	require.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "MY_SECRET", *obj.Status.ShowOutput.Name)
	assert.Equal(t, "MY_DB", *obj.Status.ShowOutput.DatabaseName)
	require.NotNil(t, obj.Status.DescribeOutput)
	assert.Equal(t, "OAUTH2", *obj.Status.DescribeOutput.SecretType)
}

// --------------------------------------------------------------------------
// Tests: Unit – detectDrift
// --------------------------------------------------------------------------

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.SecretWithClientCredentials{
		Spec: snowplanev1alpha1.SecretWithClientCredentialsSpec{
			Name: "MY_SECRET",
		},
	}

	obs := &snowflake.SecretObservation{
		ShowOutput: &snowflake.SecretShowOutput{
			Name: "MY_SECRET",
		},
	}

	result := detectDrift(obj, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_NameChanged(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.SecretWithClientCredentials{
		Spec: snowplanev1alpha1.SecretWithClientCredentialsSpec{
			Name: "NEW_SECRET",
		},
	}

	obs := &snowflake.SecretObservation{
		ShowOutput: &snowflake.SecretShowOutput{
			Name: "OLD_SECRET",
		},
	}

	result := detectDrift(obj, obs)
	assert.True(t, result.HasImmutableViolation)
}

func TestDetectDrift_CommentChanged(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.SecretWithClientCredentials{
		Spec: snowplanev1alpha1.SecretWithClientCredentialsSpec{
			Name:    "MY_SECRET",
			Comment: testutil.Ptr("new comment"),
		},
	}

	obs := &snowflake.SecretObservation{
		ShowOutput: &snowflake.SecretShowOutput{
			Name:    "MY_SECRET",
			Comment: "old comment",
		},
	}

	result := detectDrift(obj, obs)
	assert.True(t, result.HasDrift)
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	obj := newTestSecret("mysecret", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.DatabaseName = "MY_DB"
	obj.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SecretObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateSecretOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysecret", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
