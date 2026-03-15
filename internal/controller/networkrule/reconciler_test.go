package networkrule

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.NetworkRuleObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateNetworkRuleOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterNetworkRuleOptions) error {
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

func newTestNetworkRule(name, namespace string) *snowplanev1alpha1.NetworkRule {
	dbName := "MY_DB"
	schemaName := "PUBLIC"

	return &snowplanev1alpha1.NetworkRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.NetworkRuleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_RULE",
			DatabaseName: &dbName,
			SchemaName:   &schemaName,
			Type:         snowplanev1alpha1.NetworkRuleTypeIPV4,
			Mode:         snowplanev1alpha1.NetworkRuleModeIngress,
			ValueList:    []string{"10.0.0.0/24", "192.168.1.0/24"},
		},
	}
}

func successfulObservation() *snowflake.NetworkRuleObservation {
	return &snowflake.NetworkRuleObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.NetworkRuleShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_RULE",
			DatabaseName: "MY_DB",
			SchemaName:   "PUBLIC",
			Owner:        "SYSADMIN",
			Type:         "IPV4",
			Mode:         "INGRESS",
			Comment:      "",
		},
		DescribeOutput: map[string]string{
			"value_list": "10.0.0.0/24,192.168.1.0/24",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkRule, Service, *snowflake.NetworkRuleObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.NetworkRule{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("NetworkRule")

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
			return newTestNetworkRule(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.NetworkRule{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	nr.Finalizers = []string{finalizerName}
	nr.Status.DatabaseName = "MY_DB"
	nr.Status.SchemaName = "MY_DB.PUBLIC"

	var capturedOpts snowflake.CreateNetworkRuleOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NetworkRuleObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateNetworkRuleOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, nr, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynr", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_RULE", capturedOpts.Name.Name())
	assert.Equal(t, "IPV4", capturedOpts.Type)
	assert.Equal(t, "INGRESS", capturedOpts.Mode)
	assert.Equal(t, []string{"10.0.0.0/24", "192.168.1.0/24"}, capturedOpts.ValueList)

	got := &snowplanev1alpha1.NetworkRule{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynr", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	nr.Finalizers = []string{finalizerName}
	nr.Status.DatabaseName = "MY_DB"
	nr.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
			return &snowflake.NetworkRuleObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNetworkRuleOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, nr, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynr", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	nr.Finalizers = []string{finalizerName}
	nr.Status.DatabaseName = "MY_DB"
	nr.Status.SchemaName = "MY_DB.PUBLIC"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
			return &snowflake.NetworkRuleObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNetworkRuleOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, nr, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynr", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	nr.Finalizers = []string{finalizerName}
	nr.Status.DatabaseName = "MY_DB"
	nr.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	nr.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_RULE", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, nr, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynr", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.NetworkRule{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mynr", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	nr.Finalizers = []string{finalizerName}
	nr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	nr.Status.DatabaseName = "MY_DB"
	nr.Status.SchemaName = "MY_DB.PUBLIC"
	now := metav1.Now()
	nr.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, nr, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynr", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_RULE")

	opts := buildCreateOptions(nr, id)
	assert.Equal(t, "MY_RULE", opts.Name.Name())
	assert.Equal(t, "IPV4", opts.Type)
	assert.Equal(t, "INGRESS", opts.Mode)
	assert.Equal(t, []string{"10.0.0.0/24", "192.168.1.0/24"}, opts.ValueList)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("ValueListAlwaysTracked", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NetworkRuleSpec{
			ValueList: []string{"10.0.0.1"},
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "VALUE_LIST")
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NetworkRuleSpec{
			ValueList: []string{"10.0.0.1"},
			Comment:   testutil.Ptr("test"),
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "VALUE_LIST")
		assert.Contains(t, fields, "COMMENT")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	nr := newTestNetworkRule("mynr", "default")
	obs := successfulObservation()

	applyObservation(nr, obs)

	assert.NotEmpty(t, nr.Status.FullyQualifiedName)
	assert.Equal(t, "MY_RULE", nr.Status.ShowOutput.Name)
	assert.Equal(t, "MY_DB", nr.Status.ShowOutput.DatabaseName)
	assert.Equal(t, "SYSADMIN", nr.Status.ShowOutput.Owner)
	assert.Equal(t, "IPV4", nr.Status.ShowOutput.Type)
	assert.Equal(t, "INGRESS", nr.Status.ShowOutput.Mode)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	nr := &snowplanev1alpha1.NetworkRule{
		Spec: snowplanev1alpha1.NetworkRuleSpec{
			Name:      "MY_RULE",
			Type:      snowplanev1alpha1.NetworkRuleTypeIPV4,
			Mode:      snowplanev1alpha1.NetworkRuleModeIngress,
			ValueList: []string{"10.0.0.0/24"},
		},
	}

	obs := &snowflake.NetworkRuleObservation{
		ShowOutput: &snowplanev1alpha1.NetworkRuleShowOutput{
			Name: "MY_RULE",
			Type: "IPV4",
			Mode: "INGRESS",
		},
		DescribeOutput: map[string]string{
			"value_list": "10.0.0.0/24",
		},
	}

	result := detectDrift(nr, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	nr := &snowplanev1alpha1.NetworkRule{
		Spec: snowplanev1alpha1.NetworkRuleSpec{
			Name:      "MY_RULE",
			Type:      snowplanev1alpha1.NetworkRuleTypeIPV4,
			Mode:      snowplanev1alpha1.NetworkRuleModeIngress,
			ValueList: []string{"10.0.0.0/24"},
			Comment:   testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.NetworkRuleObservation{
		ShowOutput: &snowplanev1alpha1.NetworkRuleShowOutput{
			Name:    "MY_RULE",
			Type:    "IPV4",
			Mode:    "INGRESS",
			Comment: "drifted",
		},
	}

	result := detectDrift(nr, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: buildAlterOptions diff-checking
// --------------------------------------------------------------------------

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("ValueListSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		nr := newTestNetworkRule("mynr", "default")
		id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_RULE")
		obs := successfulObservation()

		opts := buildAlterOptions(nr, id, obs)
		assert.Nil(t, opts.ValueList, "ValueList should be nil when it matches DESCRIBE output")
	})

	t.Run("ValueListSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		nr := newTestNetworkRule("mynr", "default")
		nr.Spec.ValueList = []string{"172.16.0.0/16"}
		id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_RULE")
		obs := successfulObservation()

		opts := buildAlterOptions(nr, id, obs)
		require.NotNil(t, opts.ValueList)
		assert.Equal(t, []string{"172.16.0.0/16"}, *opts.ValueList)
	})

	t.Run("ValueListSentWhenNoDescribeOutput", func(t *testing.T) {
		t.Parallel()

		nr := newTestNetworkRule("mynr", "default")
		id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_RULE")
		obs := &snowflake.NetworkRuleObservation{
			Exists:     true,
			ShowOutput: successfulObservation().ShowOutput,
		}

		opts := buildAlterOptions(nr, id, obs)
		require.NotNil(t, opts.ValueList, "ValueList should be sent when DESCRIBE output is unavailable")
	})

	t.Run("ValueListOrderIndependent", func(t *testing.T) {
		t.Parallel()

		nr := newTestNetworkRule("mynr", "default")
		nr.Spec.ValueList = []string{"192.168.1.0/24", "10.0.0.0/24"} // reversed order
		id := snowflake.NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_RULE")
		obs := successfulObservation() // describe has "10.0.0.0/24,192.168.1.0/24"

		opts := buildAlterOptions(nr, id, obs)
		assert.Nil(t, opts.ValueList, "ValueList should be nil when values match regardless of order")
	})
}
