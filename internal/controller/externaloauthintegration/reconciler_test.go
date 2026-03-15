package externaloauthintegration

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.ExternalOAuthIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error {
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

func newTestExternalOAuthIntegration(name, namespace string) *snowplanev1alpha1.ExternalOAuthIntegration {
	return &snowplanev1alpha1.ExternalOAuthIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.ExternalOAuthIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                          "MY_OAUTH",
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://dev-123.okta.com",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
		},
	}
}

func successfulObservation() *snowflake.ExternalOAuthIntegrationObservation {
	return &snowflake.ExternalOAuthIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      "MY_OAUTH",
			Type:      "EXTERNAL_OAUTH - OKTA",
			Category:  "SECURITY",
			Enabled:   true,
		},
		DescribeOutput: map[string]string{
			"EXTERNAL_OAUTH_TYPE":                             "OKTA",
			"EXTERNAL_OAUTH_ISSUER":                           "https://dev-123.okta.com",
			"EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM":         "sub",
			"EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE": "LOGIN_NAME",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalOAuthIntegration, Service, *snowflake.ExternalOAuthIntegrationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.ExternalOAuthIntegration{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("ExternalOAuthIntegration")

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
			return newTestExternalOAuthIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.ExternalOAuthIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestExternalOAuthIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateExternalOAuthIntegrationOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ExternalOAuthIntegrationObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateExternalOAuthIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_OAUTH", capturedOpts.Name.Name())
	assert.Equal(t, "OKTA", capturedOpts.ExternalOAuthType)
	assert.Equal(t, "https://dev-123.okta.com", capturedOpts.Issuer)
	assert.Equal(t, "sub", capturedOpts.TokenUserMappingClaim)
	assert.Equal(t, "LOGIN_NAME", capturedOpts.SnowflakeUserMappingAttribute)

	got := &snowplanev1alpha1.ExternalOAuthIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "MY_OAUTH", got.Status.FullyQualifiedName)
}

// --------------------------------------------------------------------------
// Tests: Update (alter) flow
// --------------------------------------------------------------------------

func TestReconcile_Alter(t *testing.T) {
	t.Parallel()

	obj := newTestExternalOAuthIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	newComment := "updated comment"
	obj.Spec.Comment = &newComment
	obj.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()

	var capturedAlterOpts snowflake.AlterExternalOAuthIntegrationOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterExternalOAuthIntegrationOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_OAUTH", capturedAlterOpts.Name.Name())
	require.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "updated comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestExternalOAuthIntegration("myobj", "default")
	obj.DeletionTimestamp = &now
	obj.Finalizers = []string{finalizerName}

	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_OAUTH", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.ExternalOAuthIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestValidateImmutableFields(t *testing.T) {
	t.Parallel()

	t.Run("NoShowOutput", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obj.Spec.Name = "A"
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obj.Spec.Name = "MY_OAUTH"
		obj.Status.ShowOutput = &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{Name: "MY_OAUTH"}
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameChanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
		obj.Spec.Name = "NEW_NAME"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.ExternalOAuthIntegrationShowOutput{Name: "OLD_NAME"}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
	})
}

// --------------------------------------------------------------------------
// Tests: Build create/alter options
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	obj := newTestExternalOAuthIntegration("x", "default")
	obj.Spec.JWSKeysURL = testutil.Ptr("https://keys.example.com/jwks")
	obj.Spec.AudienceList = []string{"aud1"}
	obj.Spec.AllowedRoles = []string{"ANALYST"}
	obj.Spec.Comment = testutil.Ptr("test")
	obj.Spec.Enabled = testutil.Ptr(true)

	id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
	opts := buildCreateOptions(obj, id)

	assert.Equal(t, "MY_OAUTH", opts.Name.Name())
	assert.Equal(t, "OKTA", opts.ExternalOAuthType)
	assert.Equal(t, "https://dev-123.okta.com", opts.Issuer)
	assert.Equal(t, "sub", opts.TokenUserMappingClaim)
	assert.Equal(t, "LOGIN_NAME", opts.SnowflakeUserMappingAttribute)
	require.NotNil(t, opts.JWSKeysURL)
	assert.Equal(t, "https://keys.example.com/jwks", *opts.JWSKeysURL)
	assert.Equal(t, []string{"aud1"}, opts.AudienceList)
	assert.Equal(t, []string{"ANALYST"}, opts.AllowedRoles)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "test", *opts.Comment)
	require.NotNil(t, opts.Enabled)
	assert.True(t, *opts.Enabled)
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Comment = testutil.Ptr("new comment")
		obj.Status.TrackedParameters = []string{"COMMENT"}

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
		require.NotNil(t, opts.TokenUserMappingClaim)
		assert.Equal(t, "sub", *opts.TokenUserMappingClaim)
	})

	t.Run("ListFieldsCopied", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.AudienceList = []string{"a", "b"}
		obj.Spec.AllowedRoles = []string{"R1"}
		obj.Spec.BlockedRoles = []string{"ACCOUNTADMIN"}

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.AudienceList)
		assert.Equal(t, []string{"a", "b"}, *opts.AudienceList)
		require.NotNil(t, opts.AllowedRoles)
		assert.Equal(t, []string{"R1"}, *opts.AllowedRoles)
		require.NotNil(t, opts.BlockedRoles)
		assert.Equal(t, []string{"ACCOUNTADMIN"}, *opts.BlockedRoles)
	})

	t.Run("EnabledSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Enabled = testutil.Ptr(true)

		obs := successfulObservation()

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.Enabled)
	})

	t.Run("TokenUserMappingClaimSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM"] = "sub" // matches spec

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.TokenUserMappingClaim)
	})

	t.Run("TokenUserMappingClaimSentWhenChanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM"] = "email" // different from spec

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.TokenUserMappingClaim)
		assert.Equal(t, "sub", *opts.TokenUserMappingClaim)
	})

	t.Run("AllFieldsSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Enabled = testutil.Ptr(true)
		obj.Spec.AnyRoleMode = testutil.Ptr("DISABLE")
		obj.Spec.NetworkPolicy = testutil.Ptr("MY_POLICY")
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM"] = "sub"
		obs.DescribeOutput["EXTERNAL_OAUTH_ANY_ROLE_MODE"] = "DISABLE"
		obs.DescribeOutput["NETWORK_POLICY"] = "MY_POLICY"

		id := snowflake.NewAccountObjectIdentifier("MY_OAUTH")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.Enabled)
		assert.Nil(t, opts.TokenUserMappingClaim)
		assert.Nil(t, opts.AnyRoleMode)
		assert.Nil(t, opts.NetworkPolicy)
	})
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Enabled = testutil.Ptr(true)
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("NameDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Name = "NEW_NAME"
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("EnabledDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.Enabled = testutil.Ptr(false)
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("DescribeFieldDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.JWSKeysURL = testutil.Ptr("https://new.example.com/jwks")
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_JWS_KEYS_URL"] = "https://old.example.com/jwks"
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("TokenUserMappingClaimDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.TokenUserMappingClaim = "email"
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM"] = "sub"
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.Contains(t, result.Summary(), "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM")
	})

	t.Run("ScopeDelimiterDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestExternalOAuthIntegration("x", "default")
		obj.Spec.ScopeDelimiter = testutil.Ptr(",")
		obs := successfulObservation()
		obs.DescribeOutput["EXTERNAL_OAUTH_SCOPE_DELIMITER"] = " "
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.Contains(t, result.Summary(), "EXTERNAL_OAUTH_SCOPE_DELIMITER")
	})
}

// --------------------------------------------------------------------------
// Tests: Tracked parameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("RequiredOnly", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.ExternalOAuthIntegrationSpec{
			Name:                          "X",
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://issuer",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
		}
		fields := tracked.ComputeTracked(spec)
		// TokenUserMappingClaim is always tracked because it is a required non-empty string with a snowflake tag
		assert.Contains(t, fields, "EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.ExternalOAuthIntegrationSpec{
			Name:                          "X",
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://issuer",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
			Comment:                       testutil.Ptr("test"),
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "COMMENT")
	})

	t.Run("WithNetworkPolicy", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.ExternalOAuthIntegrationSpec{
			Name:                          "X",
			ExternalOAuthType:             "OKTA",
			Issuer:                        "https://issuer",
			TokenUserMappingClaim:         "sub",
			SnowflakeUserMappingAttribute: "LOGIN_NAME",
			NetworkPolicy:                 testutil.Ptr("MY_POLICY"),
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "NETWORK_POLICY")
	})
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_ObserveFails(t *testing.T) {
	t.Parallel()

	obj := newTestExternalOAuthIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.ExternalOAuthIntegrationObservation, error) {
			return nil, fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	assert.Error(t, err)

	got := &snowplanev1alpha1.ExternalOAuthIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_NotFound(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(&mockService{})
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --------------------------------------------------------------------------
// Tests: Apply observation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.ExternalOAuthIntegration{}
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "MY_OAUTH", obj.Status.FullyQualifiedName)
	assert.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "MY_OAUTH", obj.Status.ShowOutput.Name)
	assert.NotNil(t, obj.Status.DescribeOutput)
}
