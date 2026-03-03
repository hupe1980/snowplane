package networkpolicy

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

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.NetworkPolicyObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateNetworkPolicyOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterNetworkPolicyOptions) error {
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

func newTestNetworkPolicy(name, namespace string) *snowplanev1alpha1.NetworkPolicy {
	return &snowplanev1alpha1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:          "MY_POLICY",
			AllowedIPList: []string{"192.168.1.0/24"},
		},
	}
}

func successfulObservation() *snowflake.NetworkPolicyObservation {
	return &snowflake.NetworkPolicyObservation{
		Exists: true,
		ShowOutput: &snowflake.NetworkPolicyShowOutput{
			CreatedOn:              "2024-01-01",
			Name:                   "MY_POLICY",
			Comment:                "",
			EntriesInAllowedIPList: "192.168.1.0/24",
			EntriesInBlockedIPList: "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.NetworkPolicy{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: record.NewFakeRecorder(100),
		Adapter: &adapter{
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("NetworkPolicy"),
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
			return newTestNetworkPolicy(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.NetworkPolicy{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func TestReconcile_Create(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	obs := successfulObservation()
	var capturedOpts snowflake.CreateNetworkPolicyOptions
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NetworkPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateNetworkPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "MY_POLICY", capturedOpts.Name.Name())
	assert.Equal(t, []string{"192.168.1.0/24"}, capturedOpts.AllowedIPList)
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateWithAllFields(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Spec.AllowedIPList = []string{"10.0.0.0/8"}
	np.Spec.BlockedIPList = []string{"10.0.0.1"}
	np.Spec.AllowedNetworkRuleList = []string{"rule1"}
	np.Spec.BlockedNetworkRuleList = []string{"rule2"}
	np.Spec.Comment = testutil.Ptr("test policy")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "test policy"
	var capturedOpts snowflake.CreateNetworkPolicyOptions
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NetworkPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateNetworkPolicyOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8"}, capturedOpts.AllowedIPList)
	assert.Equal(t, []string{"10.0.0.1"}, capturedOpts.BlockedIPList)
	assert.Equal(t, []string{"rule1"}, capturedOpts.AllowedNetworkRuleList)
	assert.Equal(t, []string{"rule2"}, capturedOpts.BlockedNetworkRuleList)
	assert.Equal(t, "test policy", *capturedOpts.Comment)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return &snowflake.NetworkPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
			return fmt.Errorf("permission denied")
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return &snowflake.NetworkPolicyObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid SQL"))
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Status.ObservedGeneration = 1
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return obs, nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Status.ObservedGeneration = 1
	np.Generation = 2
	np.Spec.Comment = testutil.Ptr("new comment")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"
	var capturedAlterOpts snowflake.AlterNetworkPolicyOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterNetworkPolicyOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Status.ObservedGeneration = 1
	np.Spec.Comment = testutil.Ptr("changed")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "original"
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterNetworkPolicyOptions) error {
			return fmt.Errorf("alter failed")
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")
}

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	now := metav1.Now()
	np.DeletionTimestamp = &now
	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_POLICY", name.Name())
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)
	got := &snowplanev1alpha1.NetworkPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	np.DeletionTimestamp = &now
	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled, "should not drop when orphan policy")
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	now := metav1.Now()
	np.DeletionTimestamp = &now
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	now := metav1.Now()
	np.DeletionTimestamp = &now
	r := newTestReconciler(&mockService{}, np)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	got := &snowplanev1alpha1.NetworkPolicy{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Spec.Name = "NEW_NAME"
	np.Status.ObservedGeneration = 1
	np.Status.ShowOutput = &snowplanev1alpha1.NetworkPolicyShowOutput{
		Name: "OLD_NAME",
	}
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			return obs, nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Spec.Comment = testutil.Ptr("my policy")
	np.Spec.BlockedIPList = []string{"10.0.0.1"}
	id := snowflake.NewAccountObjectIdentifier("MY_POLICY")
	opts := buildCreateOptions(np, id)
	assert.Equal(t, "MY_POLICY", opts.Name.Name())
	assert.Equal(t, "my policy", *opts.Comment)
	assert.Equal(t, []string{"192.168.1.0/24"}, opts.AllowedIPList)
	assert.Equal(t, []string{"10.0.0.1"}, opts.BlockedIPList)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Spec.Comment = testutil.Ptr("new")
	id := snowflake.NewAccountObjectIdentifier("MY_POLICY")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"
	opts := buildAlterOptions(np, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new", *opts.Comment)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Spec.AllowedIPList = nil
	id := snowflake.NewAccountObjectIdentifier("MY_POLICY")
	obs := successfulObservation()
	opts := buildAlterOptions(np, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_UnsetComment(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Spec.AllowedIPList = nil
	np.Status.TrackedParameters = []string{"COMMENT"}
	id := snowflake.NewAccountObjectIdentifier("MY_POLICY")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"
	opts := buildAlterOptions(np, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Contains(t, opts.UnsetFields, "COMMENT")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()
	spec := &snowplanev1alpha1.NetworkPolicySpec{
		Comment:                testutil.Ptr("x"),
		AllowedIPList:          []string{"1.2.3.4"},
		BlockedIPList:          []string{"5.6.7.8"},
		AllowedNetworkRuleList: []string{"rule1"},
		BlockedNetworkRuleList: []string{"rule2"},
	}
	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"COMMENT", "ALLOWED_IP_LIST", "BLOCKED_IP_LIST", "ALLOWED_NETWORK_RULE_LIST", "BLOCKED_NETWORK_RULE_LIST"}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()
	spec := &snowplanev1alpha1.NetworkPolicySpec{}
	fields := tracked.ComputeTracked(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	obs := successfulObservation()
	applyObservation(np, obs)
	assert.NotEmpty(t, np.Status.FullyQualifiedName)
	assert.Equal(t, "MY_POLICY", np.Status.ShowOutput.Name)
	assert.Equal(t, "2024-01-01", np.Status.ShowOutput.CreatedOn)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()
	np := &snowplanev1alpha1.NetworkPolicy{
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			Name:          "MY_POLICY",
			AllowedIPList: []string{"192.168.1.0/24"},
			Comment:       testutil.Ptr("test"),
		},
	}
	obs := &snowflake.NetworkPolicyObservation{
		ShowOutput: &snowflake.NetworkPolicyShowOutput{
			Name:                   "MY_POLICY",
			Comment:                "test",
			EntriesInAllowedIPList: "192.168.1.0/24",
			EntriesInBlockedIPList: "",
		},
	}
	result := detectDrift(np, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()
	np := &snowplanev1alpha1.NetworkPolicy{
		Spec: snowplanev1alpha1.NetworkPolicySpec{
			Name:    "MY_POLICY",
			Comment: testutil.Ptr("desired"),
		},
	}
	obs := &snowflake.NetworkPolicyObservation{
		ShowOutput: &snowflake.NetworkPolicyShowOutput{
			Name:    "MY_POLICY",
			Comment: "drifted",
		},
	}
	result := detectDrift(np, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestReconcile_TrackedParametersPersistedOnCreate(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	np.Spec.Comment = testutil.Ptr("hello")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "hello"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NetworkPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	got := &snowplanev1alpha1.NetworkPolicy{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mynp", Namespace: "default"}, got))
	assert.Contains(t, got.Status.TrackedParameters, "COMMENT")
	assert.Contains(t, got.Status.TrackedParameters, "ALLOWED_IP_LIST")
}

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NetworkPolicyObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NetworkPolicyObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateNetworkPolicyOptions) error {
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()
	np := newTestNetworkPolicy("mynp", "default")
	np.Finalizers = []string{finalizerName}
	now := metav1.Now()
	np.DeletionTimestamp = &now
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return nil
		},
	}
	r := newTestReconciler(mock, np, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mynp", "default"))
	require.NoError(t, err)
	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Deleting")
}
