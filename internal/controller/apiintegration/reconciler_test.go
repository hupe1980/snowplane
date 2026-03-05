package apiintegration

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
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/tracked"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return nil, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error {
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

func newTestAPIIntegration(name, namespace string) *snowplanev1alpha1.APIIntegration {
	return &snowplanev1alpha1.APIIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.APIIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:               "MY_API_INTEGRATION",
			APIProvider:        "aws_api_gateway",
			Enabled:            testutil.Ptr(true),
			APIAllowedPrefixes: []string{"https://api.example.com/v1/"},
			APIAWSRoleARN:      testutil.Ptr("arn:aws:iam::123456789012:role/my-role"),
			Comment:            testutil.Ptr("test comment"),
		},
	}
}

func successfulObservation() *snowflake.APIIntegrationObservation {
	return &snowflake.APIIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.APIIntegrationShowOutput{
			CreatedOn: "2025-01-01 00:00:00",
			Name:      "MY_API_INTEGRATION",
			Type:      "EXTERNAL_API",
			Category:  "API",
			Enabled:   true,
			Comment:   "test comment",
		},
		DescribeOutput: map[string]string{
			"API_AWS_ROLE_ARN":     "arn:aws:iam::123456789012:role/my-role",
			"API_ALLOWED_PREFIXES": "https://api.example.com/v1/",
			"API_BLOCKED_PREFIXES": "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.APIIntegration, Service, *snowflake.APIIntegrationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.APIIntegration{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("APIIntegration")

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
			return newTestAPIIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.APIIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestAPIIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateAPIIntegrationOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.APIIntegrationObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAPIIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_API_INTEGRATION", capturedOpts.Name.Name())
	assert.Equal(t, "aws_api_gateway", capturedOpts.APIProvider)
	assert.Equal(t, []string{"https://api.example.com/v1/"}, capturedOpts.APIAllowedPrefixes)
	assert.Equal(t, testutil.Ptr("arn:aws:iam::123456789012:role/my-role"), capturedOpts.APIAWSRoleARN)

	got := &snowplanev1alpha1.APIIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "MY_API_INTEGRATION", got.Status.FullyQualifiedName)
}

// --------------------------------------------------------------------------
// Tests: Alter
// --------------------------------------------------------------------------

func TestReconcile_Alter(t *testing.T) {
	t.Parallel()

	obj := newTestAPIIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Spec.Comment = testutil.Ptr("updated comment")
	obj.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()

	var capturedOpts snowflake.AlterAPIIntegrationOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAPIIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_API_INTEGRATION", capturedOpts.Name.Name())
	require.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated comment", *capturedOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestAPIIntegration("myobj", "default")
	obj.DeletionTimestamp = &now
	obj.Finalizers = []string{finalizerName}

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_API_INTEGRATION", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.APIIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

// --------------------------------------------------------------------------
// Tests: Validate immutable fields
// --------------------------------------------------------------------------

func TestValidateImmutableFields(t *testing.T) {
	t.Parallel()

	t.Run("NoShowOutput", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.APIIntegration{}
		obj.Spec.Name = "MY_API"
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.APIIntegration{}
		obj.Spec.Name = "MY_API"
		obj.Status.ShowOutput = &snowplanev1alpha1.APIIntegrationShowOutput{Name: "MY_API"}
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameChanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.APIIntegration{}
		obj.Spec.Name = "NEW_NAME"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.APIIntegrationShowOutput{Name: "OLD_NAME"}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
	})
}

// --------------------------------------------------------------------------
// Tests: Build create options
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	obj := newTestAPIIntegration("x", "default")
	id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
	opts := buildCreateOptions(obj, id)

	assert.Equal(t, "MY_API_INTEGRATION", opts.Name.Name())
	assert.Equal(t, "aws_api_gateway", opts.APIProvider)
	assert.Equal(t, testutil.Ptr(true), opts.Enabled)
	assert.Equal(t, []string{"https://api.example.com/v1/"}, opts.APIAllowedPrefixes)
	assert.Equal(t, "arn:aws:iam::123456789012:role/my-role", *opts.APIAWSRoleARN)
	assert.Equal(t, "test comment", *opts.Comment)
	assert.Nil(t, opts.APIBlockedPrefixes)
	assert.Nil(t, opts.GoogleAudience)
	assert.Nil(t, opts.AzureTenantID)
	assert.Nil(t, opts.AzureADAppID)
	assert.Nil(t, opts.APIKey)
}

// --------------------------------------------------------------------------
// Tests: Build alter options
// --------------------------------------------------------------------------

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.Comment = testutil.Ptr("new comment")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
	})

	t.Run("ListFieldsCopied", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.APIBlockedPrefixes = []string{"https://blocked.example.com/"}
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.APIAllowedPrefixes)
		assert.Equal(t, []string{"https://api.example.com/v1/"}, *opts.APIAllowedPrefixes)
		require.NotNil(t, opts.APIBlockedPrefixes)
		assert.Equal(t, []string{"https://blocked.example.com/"}, *opts.APIBlockedPrefixes)
	})

	t.Run("CommentSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.Comment) // same as observed
	})

	t.Run("EnabledSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.Enabled)
	})

	t.Run("AWSRoleARNCopied", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.APIAWSRoleARN)
		assert.Equal(t, "arn:aws:iam::123456789012:role/my-role", *opts.APIAWSRoleARN)
	})
}

// --------------------------------------------------------------------------
// Tests: Detect drift
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("NameDrift_HasImmutableViolation", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Name = "DIFFERENT_NAME"

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift) // immutable violations are tracked separately
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("CommentDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Comment = "changed by someone"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.False(t, result.HasImmutableViolation)
	})

	t.Run("EnabledDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Enabled = false

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("AllowedPrefixesDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()
		obs.DescribeOutput["API_ALLOWED_PREFIXES"] = "https://other.example.com/"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("AWSRoleARNDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obs := successfulObservation()
		obs.DescribeOutput["API_AWS_ROLE_ARN"] = "arn:aws:iam::999:role/other"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("GoogleAudienceDrift_ImmutableViolation", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.GoogleAudience = testutil.Ptr("my-audience")
		obs := successfulObservation()
		obs.DescribeOutput["GOOGLE_AUDIENCE"] = "different-audience"

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift, "immutable field should not report as mutable drift")
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("AzureTenantIDDrift_ImmutableViolation", func(t *testing.T) {
		t.Parallel()

		obj := newTestAPIIntegration("x", "default")
		obj.Spec.AzureTenantID = testutil.Ptr("tenant-123")
		obs := successfulObservation()
		obs.DescribeOutput["AZURE_TENANT_ID"] = "tenant-456"

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift, "immutable field should not report as mutable drift")
		assert.True(t, result.HasImmutableViolation)
	})
}

// --------------------------------------------------------------------------
// Tests: Compute tracked parameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("AllFieldsSet", func(t *testing.T) {
		t.Parallel()

		spec := &snowplanev1alpha1.APIIntegrationSpec{
			Name:               "x",
			APIProvider:        "aws_api_gateway",
			Enabled:            testutil.Ptr(true),
			APIAllowedPrefixes: []string{"https://x"},
			APIAWSRoleARN:      testutil.Ptr("arn"),
			APIKey:             testutil.Ptr("key"),
			Comment:            testutil.Ptr("c"),
		}
		params := tracked.ComputeTracked(spec)
		assert.Contains(t, params, "ENABLED")
		assert.Contains(t, params, "API_ALLOWED_PREFIXES")
		assert.Contains(t, params, "API_AWS_ROLE_ARN")
		assert.Contains(t, params, "COMMENT")
		assert.Contains(t, params, "API_KEY")
	})

	t.Run("MinimalFields", func(t *testing.T) {
		t.Parallel()

		spec := &snowplanev1alpha1.APIIntegrationSpec{
			Name:               "x",
			APIProvider:        "git_https_api",
			APIAllowedPrefixes: []string{"https://x"},
		}
		params := tracked.ComputeTracked(spec)
		assert.Contains(t, params, "API_ALLOWED_PREFIXES")
		assert.NotContains(t, params, "COMMENT")
		assert.NotContains(t, params, "API_KEY")
	})
}

// --------------------------------------------------------------------------
// Tests: Apply observation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.APIIntegration{}
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "MY_API_INTEGRATION", obj.Status.FullyQualifiedName)
	assert.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "MY_API_INTEGRATION", obj.Status.ShowOutput.Name)
	assert.NotNil(t, obj.Status.DescribeOutput)
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_ObserveFails(t *testing.T) {
	t.Parallel()

	obj := newTestAPIIntegration("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
			return nil, fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	assert.Error(t, err)

	got := &snowplanev1alpha1.APIIntegration{}
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
// Tests: Helpers
// --------------------------------------------------------------------------

func TestParseCommaListFromMap(t *testing.T) {
	t.Parallel()

	t.Run("NilMap", func(t *testing.T) {
		t.Parallel()

		result := parseCommaListFromMap(nil, "KEY")
		assert.Nil(t, result)
	})

	t.Run("MissingKey", func(t *testing.T) {
		t.Parallel()

		result := parseCommaListFromMap(map[string]string{"OTHER": "val"}, "KEY")
		assert.Nil(t, result)
	})

	t.Run("MultipleValues", func(t *testing.T) {
		t.Parallel()

		result := parseCommaListFromMap(map[string]string{"KEY": "a, b, c"}, "KEY")
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()

		result := parseCommaListFromMap(map[string]string{"KEY": "  "}, "KEY")
		assert.Empty(t, result)
	})
}

func TestDescribeValue(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", describeValue(nil, "key"))
	assert.Equal(t, "val", describeValue(map[string]string{"key": "val"}, "key"))
	assert.Equal(t, "", describeValue(map[string]string{"other": "val"}, "key"))
}

func TestStringValueOrEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", stringValueOrEmpty(nil))
	assert.Equal(t, "hello", stringValueOrEmpty(testutil.Ptr("hello")))
}

func TestCompareListFromDescribeMap(t *testing.T) {
	t.Parallel()

	t.Run("Equal", func(t *testing.T) {
		t.Parallel()

		d := drift.New()
		compareListFromDescribeMap(d, "KEY", []string{"a", "b"}, map[string]string{"KEY": "a,b"})
		assert.False(t, d.Result().HasDrift)
	})

	t.Run("Different", func(t *testing.T) {
		t.Parallel()

		d := drift.New()
		compareListFromDescribeMap(d, "KEY", []string{"a", "b"}, map[string]string{"KEY": "c,d"})
		assert.True(t, d.Result().HasDrift)
	})
}
