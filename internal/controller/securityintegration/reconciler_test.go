package securityintegration

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.SecurityIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSecurityIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSecurityIntegrationOptions) error {
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

func newTestSecurityIntegration(name, namespace string) *snowplanev1alpha1.SecurityIntegration {
	return &snowplanev1alpha1.SecurityIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SecurityIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "MY_SCIM",
			Type: snowplanev1alpha1.SecurityIntegrationTypeSCIM,
			SCIM: &snowplanev1alpha1.SCIMConfig{
				SCIMClient: "AZURE",
				RunAsRole:  "AAD_PROVISIONER",
			},
		},
	}
}

func successfulObservation() *snowflake.SecurityIntegrationObservation {
	return &snowflake.SecurityIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SecurityIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      "MY_SCIM",
			Type:      "SCIM - AZURE",
			Category:  "SECURITY",
		},
		DescribeOutput: map[string]string{
			"SCIM_CLIENT":    "AZURE",
			"RUN_AS_ROLE":    "AAD_PROVISIONER",
			"NETWORK_POLICY": "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SecurityIntegration, Service, *snowflake.SecurityIntegrationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.SecurityIntegration{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	r := NewReconcilerWithServiceFactory(c, factory, rec, nil,
		func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SecurityIntegration")

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
			return newTestSecurityIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SecurityIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	si.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateSecurityIntegrationOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SecurityIntegrationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSecurityIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, si, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysi", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_SCIM", capturedOpts.Name.Name())
	assert.Equal(t, "SCIM", capturedOpts.Type)
	assert.NotNil(t, capturedOpts.SCIMClient)
	assert.Equal(t, "AZURE", *capturedOpts.SCIMClient)

	got := &snowplanev1alpha1.SecurityIntegration{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mysi", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	si.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
			return &snowflake.SecurityIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSecurityIntegrationOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, si, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysi", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	si.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecurityIntegrationObservation, error) {
			return &snowflake.SecurityIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSecurityIntegrationOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, si, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysi", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	si.Finalizers = []string{finalizerName}
	now := metav1.Now()
	si.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_SCIM", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, si, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysi", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.SecurityIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mysi", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	si.Finalizers = []string{finalizerName}
	si.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	si.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, si, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mysi", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	id := snowflake.NewAccountObjectIdentifier("MY_SCIM")

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	opts, err := buildCreateOptions(context.Background(), c, si, id)
	require.NoError(t, err)
	assert.Equal(t, "MY_SCIM", opts.Name.Name())
	assert.Equal(t, "SCIM", opts.Type)
	assert.NotNil(t, opts.SCIMClient)
	assert.Equal(t, "AZURE", *opts.SCIMClient)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.SecurityIntegrationSpec{
			Type: snowplanev1alpha1.SecurityIntegrationTypeSCIM,
			SCIM: &snowplanev1alpha1.SCIMConfig{
				SCIMClient: "AZURE",
				RunAsRole:  "AAD_PROVISIONER",
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Empty(t, fields)
	})

	t.Run("SCIMWithNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.SecurityIntegrationSpec{
			Type: snowplanev1alpha1.SecurityIntegrationTypeSCIM,
			SCIM: &snowplanev1alpha1.SCIMConfig{
				SCIMClient:    "AZURE",
				RunAsRole:     "AAD_PROVISIONER",
				NetworkPolicy: testutil.Ptr("MY_POLICY"),
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "NETWORK_POLICY")
	})

	t.Run("SCIMWithSyncPassword", func(t *testing.T) {
		t.Parallel()
		syncPw := true
		spec := &snowplanev1alpha1.SecurityIntegrationSpec{
			Type: snowplanev1alpha1.SecurityIntegrationTypeSCIM,
			SCIM: &snowplanev1alpha1.SCIMConfig{
				SCIMClient:   "AZURE",
				RunAsRole:    "AAD_PROVISIONER",
				SyncPassword: &syncPw,
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "SYNC_PASSWORD")
	})

	t.Run("ExternalOAuthWithNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.SecurityIntegrationSpec{
			Type: snowplanev1alpha1.SecurityIntegrationTypeExternalOAuth,
			ExternalOAuth: &snowplanev1alpha1.ExternalOAuthConfig{
				Type:                          "CUSTOM",
				Issuer:                        "https://issuer",
				TokenUserMappingClaim:         "upn",
				SnowflakeUserMappingAttribute: "LOGIN_NAME",
				NetworkPolicy:                 testutil.Ptr("MY_POLICY"),
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "NETWORK_POLICY")
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.SecurityIntegrationSpec{
			Type:    snowplanev1alpha1.SecurityIntegrationTypeSCIM,
			Comment: testutil.Ptr("test"),
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "COMMENT")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	si := newTestSecurityIntegration("mysi", "default")
	obs := successfulObservation()

	applyObservation(si, obs)

	assert.Equal(t, "MY_SCIM", si.Status.FullyQualifiedName)
	assert.Equal(t, "MY_SCIM", si.Status.ShowOutput.Name)
	assert.Equal(t, "SCIM - AZURE", si.Status.ShowOutput.Type)
	assert.Contains(t, si.Status.DescribeOutput, "SCIM_CLIENT")
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	si := &snowplanev1alpha1.SecurityIntegration{
		Spec: snowplanev1alpha1.SecurityIntegrationSpec{
			Name: "MY_SCIM",
		},
	}

	obs := &snowflake.SecurityIntegrationObservation{
		ShowOutput: &snowplanev1alpha1.SecurityIntegrationShowOutput{
			Name: "MY_SCIM",
		},
	}

	result := detectDrift(si, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	si := &snowplanev1alpha1.SecurityIntegration{
		Spec: snowplanev1alpha1.SecurityIntegrationSpec{
			Name:    "MY_SCIM",
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.SecurityIntegrationObservation{
		ShowOutput: &snowplanev1alpha1.SecurityIntegrationShowOutput{
			Name:    "MY_SCIM",
			Comment: "drifted",
		},
	}

	result := detectDrift(si, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}
