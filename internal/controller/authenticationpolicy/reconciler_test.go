package authenticationpolicy

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterAuthenticationPolicyOptions) error {
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

func newTestAuthenticationPolicy(name, namespace string) *snowplanev1alpha1.AuthenticationPolicy {
	dbName := "MY_DB"
	schemaName := "PUBLIC"

	return &snowplanev1alpha1.AuthenticationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                  "MY_AUTH_POLICY",
			DatabaseName:          &dbName,
			SchemaName:            &schemaName,
			AuthenticationMethods: []string{"PASSWORD", "SAML"},
			ClientTypes:           []string{"SNOWFLAKE_UI", "DRIVERS"},
			Comment:               testutil.Ptr("test auth policy"),
		},
	}
}

func successfulObservation() *snowflake.AuthenticationPolicyObservation {
	return &snowflake.AuthenticationPolicyObservation{
		Exists: true,
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_AUTH_POLICY",
			DatabaseName: "MY_DB",
			SchemaName:   "PUBLIC",
			Owner:        "SYSADMIN",
			Comment:      "test auth policy",
		},
		DescribeOutput: map[string]string{
			"AUTHENTICATION_METHODS": "[PASSWORD, SAML]",
			"CLIENT_TYPES":           "[SNOWFLAKE_UI, DRIVERS]",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.AuthenticationPolicy{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.AuthenticationPolicy, Service, *snowflake.AuthenticationPolicyObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("AuthenticationPolicy"),
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
			return newTestAuthenticationPolicy(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.AuthenticationPolicy{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"

	var capturedOpts snowflake.CreateAuthenticationPolicyOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_AUTH_POLICY", capturedOpts.Name.Name())
	assert.Equal(t, []string{"PASSWORD", "SAML"}, capturedOpts.AuthenticationMethods)
	assert.Equal(t, []string{"SNOWFLAKE_UI", "DRIVERS"}, capturedOpts.ClientTypes)
	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "test auth policy", *capturedOpts.Comment)

	got := &snowplanev1alpha1.AuthenticationPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myap", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
			return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAuthenticationPolicyOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
			return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateAuthenticationPolicyOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	ap.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_AUTH_POLICY", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.AuthenticationPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myap", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	ap.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")

	opts := buildCreateOptions(ap, id)
	assert.Equal(t, "MY_AUTH_POLICY", opts.Name.Name())
	assert.Equal(t, []string{"PASSWORD", "SAML"}, opts.AuthenticationMethods)
	assert.Equal(t, []string{"SNOWFLAKE_UI", "DRIVERS"}, opts.ClientTypes)
	assert.NotNil(t, opts.Comment)
	assert.Equal(t, "test auth policy", *opts.Comment)
}

func TestBuildCreateOptions_WithSubPolicies(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Spec.MfaPolicy = &snowplanev1alpha1.AuthenticationPolicyMfaPolicy{
		AllowedMethods:                     []string{"TOTP"},
		EnforceMfaOnExternalAuthentication: testutil.Ptr("REQUIRED"),
	}
	ap.Spec.PatPolicy = &snowplanev1alpha1.AuthenticationPolicyPatPolicy{
		DefaultExpiryInDays:                   testutil.Ptr(int32(30)),
		MaxExpiryInDays:                       testutil.Ptr(int32(90)),
		NetworkPolicyEvaluation:               testutil.Ptr("REQUIRED"),
		RequireRoleRestrictionForServiceUsers: testutil.Ptr(true),
	}
	ap.Spec.WorkloadIdentityPolicy = &snowplanev1alpha1.AuthenticationPolicyWorkloadIdentityPolicy{
		AllowedProviders:    []string{"AWS", "AZURE"},
		AllowedAwsAccounts:  []string{"123456789012"},
		AllowedAzureIssuers: []string{"https://sts.windows.net/tenant"},
		AllowedOidcIssuers:  []string{"https://accounts.google.com"},
	}

	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")
	opts := buildCreateOptions(ap, id)

	assert.Equal(t, []string{"TOTP"}, opts.MfaAllowedMethods)
	assert.Equal(t, "REQUIRED", *opts.MfaEnforceMfaOnExternalAuth)
	assert.Equal(t, int32(30), *opts.PatDefaultExpiryInDays)
	assert.Equal(t, int32(90), *opts.PatMaxExpiryInDays)
	assert.Equal(t, "REQUIRED", *opts.PatNetworkPolicyEvaluation)
	assert.True(t, *opts.PatRequireRoleRestriction)
	assert.Equal(t, []string{"AWS", "AZURE"}, opts.WorkloadIdentityAllowedProviders)
	assert.Equal(t, []string{"123456789012"}, opts.WorkloadIdentityAllowedAwsAccounts)
	assert.Equal(t, []string{"https://sts.windows.net/tenant"}, opts.WorkloadIdentityAllowedAzureIssuers)
	assert.Equal(t, []string{"https://accounts.google.com"}, opts.WorkloadIdentityAllowedOidcIssuers)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("AuthenticationMethodsSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.AuthenticationPolicySpec{
			AuthenticationMethods: []string{"PASSWORD"},
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "AUTHENTICATION_METHODS")
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.AuthenticationPolicySpec{
			Comment: testutil.Ptr("test"),
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "COMMENT")
	})

	t.Run("MfaPolicySet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.AuthenticationPolicySpec{
			MfaPolicy: &snowplanev1alpha1.AuthenticationPolicyMfaPolicy{
				AllowedMethods: []string{"TOTP"},
			},
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "MFA_POLICY")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	obs := successfulObservation()

	applyObservation(ap, obs)

	assert.NotEmpty(t, ap.Status.FullyQualifiedName)
	assert.Equal(t, "MY_AUTH_POLICY", ap.Status.ShowOutput.Name)
	assert.Equal(t, "MY_DB", ap.Status.ShowOutput.DatabaseName)
	assert.Equal(t, "SYSADMIN", ap.Status.ShowOutput.Owner)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name: "MY_AUTH_POLICY",
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{
			Name: "MY_AUTH_POLICY",
		},
	}

	result := detectDrift(ap, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:    "MY_AUTH_POLICY",
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{
			Name:    "MY_AUTH_POLICY",
			Comment: "drifted",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestDetectDrift_MfaEnrollmentDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:          "MY_AUTH_POLICY",
			MfaEnrollment: testutil.Ptr("REQUIRED"),
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{
			Name: "MY_AUTH_POLICY",
		},
		DescribeOutput: map[string]string{
			"MFA_ENROLLMENT": "OPTIONAL",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "MFA_ENROLLMENT")
}

func TestDetectDrift_AuthenticationMethodsDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:                  "MY_AUTH_POLICY",
			AuthenticationMethods: []string{"PASSWORD", "SAML"},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"AUTHENTICATION_METHODS": "[PASSWORD, OAUTH]",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "AUTHENTICATION_METHODS")
}

func TestDetectDrift_ClientTypesDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:        "MY_AUTH_POLICY",
			ClientTypes: []string{"SNOWFLAKE_UI", "DRIVERS"},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"CLIENT_TYPES": "[SNOWFLAKE_UI]",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "CLIENT_TYPES")
}

func TestDetectDrift_SecurityIntegrationsDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:                 "MY_AUTH_POLICY",
			SecurityIntegrations: []string{"MY_INT"},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"SECURITY_INTEGRATIONS": "[OTHER_INT]",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "SECURITY_INTEGRATIONS")
}

func TestDetectDrift_PatPolicyDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name: "MY_AUTH_POLICY",
			PatPolicy: &snowplanev1alpha1.AuthenticationPolicyPatPolicy{
				DefaultExpiryInDays:                   testutil.Ptr(int32(30)),
				MaxExpiryInDays:                       testutil.Ptr(int32(90)),
				NetworkPolicyEvaluation:               testutil.Ptr("REQUIRED"),
				RequireRoleRestrictionForServiceUsers: testutil.Ptr(true),
			},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"PAT_DEFAULT_EXPIRY_IN_DAYS":                     "60",
			"PAT_MAX_EXPIRY_IN_DAYS":                         "90",
			"PAT_NETWORK_POLICY_EVALUATION":                  "OPTIONAL",
			"PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS": "false",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "PAT_DEFAULT_EXPIRY_IN_DAYS")
	assert.Contains(t, result.Summary(), "PAT_NETWORK_POLICY_EVALUATION")
	assert.Contains(t, result.Summary(), "PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS")
	// PAT_MAX_EXPIRY_IN_DAYS matches (90 == 90), should NOT be in summary.
	assert.NotContains(t, result.Summary(), "PAT_MAX_EXPIRY_IN_DAYS")
}

func TestDetectDrift_WorkloadIdentityDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name: "MY_AUTH_POLICY",
			WorkloadIdentityPolicy: &snowplanev1alpha1.AuthenticationPolicyWorkloadIdentityPolicy{
				AllowedProviders:   []string{"AWS", "AZURE"},
				AllowedAwsAccounts: []string{"123456789012"},
			},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"WORKLOAD_IDENTITY_ALLOWED_PROVIDERS":    "[AWS, GCP]",
			"WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS": "[123456789012]",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "WORKLOAD_IDENTITY_ALLOWED_PROVIDERS")
	// AWS accounts match, should NOT be in drift.
	assert.NotContains(t, result.Summary(), "WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS")
}

func TestDetectDrift_MfaSubPolicyDrift(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name: "MY_AUTH_POLICY",
			MfaPolicy: &snowplanev1alpha1.AuthenticationPolicyMfaPolicy{
				AllowedMethods:                     []string{"TOTP"},
				EnforceMfaOnExternalAuthentication: testutil.Ptr("REQUIRED"),
			},
		},
	}

	obs := &snowflake.AuthenticationPolicyObservation{
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{Name: "MY_AUTH_POLICY"},
		DescribeOutput: map[string]string{
			"MFA_AUTHENTICATION_METHODS":             "[TOTP, EMAIL]",
			"ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION": "OPTIONAL",
		},
	}

	result := detectDrift(ap, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "MFA_AUTHENTICATION_METHODS")
	assert.Contains(t, result.Summary(), "ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION")
}

func TestDescListEqual(t *testing.T) {
	t.Parallel()

	t.Run("Match", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"AUTH_METHODS": "[PASSWORD, SAML]"}
		assert.True(t, descListEqual("AUTH_METHODS", []string{"PASSWORD", "SAML"}, desc))
	})

	t.Run("Mismatch", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"AUTH_METHODS": "[PASSWORD, SAML]"}
		assert.False(t, descListEqual("AUTH_METHODS", []string{"PASSWORD"}, desc))
	})

	t.Run("MissingKey", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{}
		assert.False(t, descListEqual("AUTH_METHODS", []string{"PASSWORD"}, desc))
	})

	t.Run("NilDesc", func(t *testing.T) {
		t.Parallel()
		assert.False(t, descListEqual("AUTH_METHODS", []string{"PASSWORD"}, nil))
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"AUTH_METHODS": "[password, saml]"}
		assert.True(t, descListEqual("AUTH_METHODS", []string{"PASSWORD", "SAML"}, desc))
	})
}

func TestCompareDescInt32(t *testing.T) {
	t.Parallel()

	t.Run("Match", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"PAT_DEFAULT_EXPIRY_IN_DAYS": "30"}
		result := compareDescInt32(testutil.Ptr(int32(30)), "PAT_DEFAULT_EXPIRY_IN_DAYS", desc)
		assert.Nil(t, result, "matching value should return nil (no change needed)")
	})

	t.Run("Mismatch", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"PAT_DEFAULT_EXPIRY_IN_DAYS": "60"}
		result := compareDescInt32(testutil.Ptr(int32(30)), "PAT_DEFAULT_EXPIRY_IN_DAYS", desc)
		assert.NotNil(t, result)
		assert.Equal(t, int32(30), *result)
	})

	t.Run("NilSpec", func(t *testing.T) {
		t.Parallel()
		desc := map[string]string{"PAT_DEFAULT_EXPIRY_IN_DAYS": "30"}
		result := compareDescInt32(nil, "PAT_DEFAULT_EXPIRY_IN_DAYS", desc)
		assert.Nil(t, result)
	})

	t.Run("NilDesc", func(t *testing.T) {
		t.Parallel()
		result := compareDescInt32(testutil.Ptr(int32(30)), "PAT_DEFAULT_EXPIRY_IN_DAYS", nil)
		assert.NotNil(t, result)
		assert.Equal(t, int32(30), *result)
	})
}

func TestImmutableName(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Spec.Name = "CHANGED_NAME"
	ap.Status.ObservedGeneration = 1
	ap.Status.ShowOutput = &snowplanev1alpha1.AuthenticationPolicyShowOutput{
		Name: "MY_AUTH_POLICY",
	}

	a := &adapter{}
	err := a.ValidateImmutableFields(context.Background(), ap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
}

func TestImmutableDatabase(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Status.ObservedGeneration = 1
	ap.Status.DatabaseName = "NEW_DB"
	ap.Status.ShowOutput = &snowplanev1alpha1.AuthenticationPolicyShowOutput{
		Name:         "MY_AUTH_POLICY",
		DatabaseName: "OLD_DB",
	}

	a := &adapter{}
	err := a.ValidateImmutableFields(context.Background(), ap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
	assert.Contains(t, err.Error(), "databaseRef")
}

func TestImmutableSchema(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Status.ObservedGeneration = 1
	ap.Status.SchemaName = "MY_DB.NEW_SCHEMA"
	ap.Status.ShowOutput = &snowplanev1alpha1.AuthenticationPolicyShowOutput{
		Name:         "MY_AUTH_POLICY",
		DatabaseName: "MY_DB",
		SchemaName:   "OLD_SCHEMA",
	}

	a := &adapter{}
	err := a.ValidateImmutableFields(context.Background(), ap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
	assert.Contains(t, err.Error(), "schemaRef")
}

func TestBuildAlterOptions_NoDiff(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Status.TrackedParameters = []string{"AUTHENTICATION_METHODS", "CLIENT_TYPES", "COMMENT"}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")

	obs := successfulObservation()
	opts := buildAlterOptions(ap, id, obs)

	assert.Empty(t, opts.AuthenticationMethods, "matching lists should not generate alter")
	assert.Empty(t, opts.ClientTypes, "matching lists should not generate alter")
	assert.Nil(t, opts.Comment, "matching comment should not generate alter")
}

func TestBuildAlterOptions_WithChanges(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Spec.AuthenticationMethods = []string{"PASSWORD", "OAUTH"} // Changed from SAML
	ap.Spec.Comment = testutil.Ptr("updated comment")
	ap.Status.TrackedParameters = []string{"AUTHENTICATION_METHODS", "CLIENT_TYPES", "COMMENT"}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")

	obs := successfulObservation()
	opts := buildAlterOptions(ap, id, obs)

	assert.Equal(t, []string{"PASSWORD", "OAUTH"}, opts.AuthenticationMethods)
	assert.NotNil(t, opts.Comment)
	assert.Equal(t, "updated comment", *opts.Comment)
}

func TestBuildAlterOptions_UnsetFields(t *testing.T) {
	t.Parallel()

	// Spec has no comment, but tracked params has it — should UNSET.
	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name: "MY_AUTH_POLICY",
		},
	}
	ap.Status.TrackedParameters = []string{"COMMENT"}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")

	obs := successfulObservation()
	opts := buildAlterOptions(ap, id, obs)

	assert.Contains(t, opts.UnsetFields, "COMMENT")
}

func TestBuildAlterOptions_SubPolicyChanges(t *testing.T) {
	t.Parallel()

	ap := &snowplanev1alpha1.AuthenticationPolicy{
		Spec: snowplanev1alpha1.AuthenticationPolicySpec{
			Name:          "MY_AUTH_POLICY",
			MfaEnrollment: testutil.Ptr("REQUIRED"),
			PatPolicy: &snowplanev1alpha1.AuthenticationPolicyPatPolicy{
				DefaultExpiryInDays: testutil.Ptr(int32(30)),
			},
		},
	}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_AUTH_POLICY")

	obs := &snowflake.AuthenticationPolicyObservation{
		Exists: true,
		ShowOutput: &snowflake.AuthenticationPolicyShowOutput{
			Name: "MY_AUTH_POLICY",
		},
		DescribeOutput: map[string]string{
			"MFA_ENROLLMENT":             "OPTIONAL",
			"PAT_DEFAULT_EXPIRY_IN_DAYS": "60",
		},
	}

	opts := buildAlterOptions(ap, id, obs)

	assert.NotNil(t, opts.MfaEnrollment)
	assert.Equal(t, "REQUIRED", *opts.MfaEnrollment)
	assert.NotNil(t, opts.PatDefaultExpiryInDays)
	assert.Equal(t, int32(30), *opts.PatDefaultExpiryInDays)
}

func TestEventEmission_Create(t *testing.T) {
	t.Parallel()

	ap := newTestAuthenticationPolicy("myap", "default")
	ap.Finalizers = []string{finalizerName}
	ap.Status.DatabaseName = "MY_DB"
	ap.Status.SchemaName = "MY_DB.PUBLIC"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
	}

	r := newTestReconciler(mock, ap, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myap", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	assert.True(t, testutil.ContainsEvent(events, "created"), "expected a 'created' event")
}
