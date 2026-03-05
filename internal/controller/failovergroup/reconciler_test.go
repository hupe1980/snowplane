package failovergroup

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFailoverGroupOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.FailoverGroupObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateFailoverGroupOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterFailoverGroupOptions) error {
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

func newTestFailoverGroup(name, namespace string) *snowplanev1alpha1.FailoverGroup {
	return &snowplanev1alpha1.FailoverGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.FailoverGroupSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:            "MY_FG",
			ObjectTypes:     []string{"DATABASES", "ROLES"},
			AllowedAccounts: []string{"MYORG.ACCT2"},
		},
	}
}

func successfulObservation() *snowflake.FailoverGroupObservation {
	return &snowflake.FailoverGroupObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.FailoverGroupShowOutput{
			CreatedOn:           "2024-01-01",
			Name:                "MY_FG",
			Type:                "FAILOVER",
			Comment:             "",
			IsPrimary:           true,
			Primary:             "MYORG.ACCT1.MY_FG",
			ObjectTypes:         "DATABASES, ROLES",
			AllowedAccounts:     "MYORG.ACCT2",
			ReplicationSchedule: "",
			Owner:               "ACCOUNTADMIN",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.FailoverGroup, Service, *snowflake.FailoverGroupObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.FailoverGroup{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("FailoverGroup")

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
			return newTestFailoverGroup(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.FailoverGroup{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestFailoverGroup("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateFailoverGroupOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
				call++
				if call == 1 {
					return &snowflake.FailoverGroupObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateFailoverGroupOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_FG", capturedOpts.Name.Name())
	assert.Equal(t, []string{"DATABASES", "ROLES"}, capturedOpts.ObjectTypes)
	assert.Equal(t, []string{"MYORG.ACCT2"}, capturedOpts.AllowedAccounts)

	got := &snowplanev1alpha1.FailoverGroup{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "MY_FG", got.Status.FullyQualifiedName)
}

// --------------------------------------------------------------------------
// Tests: Update (alter) flow
// --------------------------------------------------------------------------

func TestReconcile_Alter(t *testing.T) {
	t.Parallel()

	obj := newTestFailoverGroup("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	newComment := "updated comment"
	obj.Spec.Comment = &newComment
	obj.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()

	var capturedAlterOpts snowflake.AlterFailoverGroupOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterFailoverGroupOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_FG", capturedAlterOpts.Name.Name())
	require.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "updated comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestFailoverGroup("myobj", "default")
	obj.DeletionTimestamp = &now
	obj.Finalizers = []string{finalizerName}

	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_FG", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.FailoverGroup{}
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
		obj := &snowplanev1alpha1.FailoverGroup{}
		obj.Spec.Name = "A"
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obj.Spec.Name = "MY_FG"
		obj.Status.ShowOutput = &snowplanev1alpha1.FailoverGroupShowOutput{Name: "MY_FG"}
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameChanged", func(t *testing.T) {
		t.Parallel()
		obj := &snowplanev1alpha1.FailoverGroup{}
		obj.Spec.Name = "NEW_NAME"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.FailoverGroupShowOutput{Name: "OLD_NAME"}
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

	obj := newTestFailoverGroup("x", "default")
	obj.Spec.AllowedDatabases = []string{"DB1", "DB2"}
	obj.Spec.AllowedShares = []string{"SHARE1"}
	obj.Spec.AllowedIntegrationTypes = []string{"SECURITY INTEGRATIONS"}
	obj.Spec.IgnoreEditionCheck = testutil.Ptr(true)
	obj.Spec.ReplicationSchedule = testutil.Ptr("10 MINUTE")
	obj.Spec.ErrorIntegration = testutil.Ptr("MY_NOTIF")
	obj.Spec.Comment = testutil.Ptr("DR group")

	id := snowflake.NewAccountObjectIdentifier("MY_FG")
	opts := buildCreateOptions(obj, id)

	assert.Equal(t, "MY_FG", opts.Name.Name())
	assert.Equal(t, []string{"DATABASES", "ROLES"}, opts.ObjectTypes)
	assert.Equal(t, []string{"MYORG.ACCT2"}, opts.AllowedAccounts)
	assert.Equal(t, []string{"DB1", "DB2"}, opts.AllowedDatabases)
	assert.Equal(t, []string{"SHARE1"}, opts.AllowedShares)
	assert.Equal(t, []string{"SECURITY INTEGRATIONS"}, opts.AllowedIntegrationTypes)
	require.NotNil(t, opts.IgnoreEditionCheck)
	assert.True(t, *opts.IgnoreEditionCheck)
	require.NotNil(t, opts.ReplicationSchedule)
	assert.Equal(t, "10 MINUTE", *opts.ReplicationSchedule)
	require.NotNil(t, opts.ErrorIntegration)
	assert.Equal(t, "MY_NOTIF", *opts.ErrorIntegration)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "DR group", *opts.Comment)
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.Comment = testutil.Ptr("new comment")
		obj.Status.TrackedParameters = []string{"COMMENT"}

		id := snowflake.NewAccountObjectIdentifier("MY_FG")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
		// ObjectTypes and AllowedAccounts are always sent.
		require.NotNil(t, opts.ObjectTypes)
		assert.Equal(t, []string{"DATABASES", "ROLES"}, *opts.ObjectTypes)
		require.NotNil(t, opts.AllowedAccounts)
		assert.Equal(t, []string{"MYORG.ACCT2"}, *opts.AllowedAccounts)
	})

	t.Run("ListFieldsCopied", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.AllowedDatabases = []string{"DB1", "DB2"}
		obj.Spec.AllowedShares = []string{"SHARE1"}
		obj.Spec.AllowedIntegrationTypes = []string{"API INTEGRATIONS"}

		id := snowflake.NewAccountObjectIdentifier("MY_FG")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.AllowedDatabases)
		assert.Equal(t, []string{"DB1", "DB2"}, *opts.AllowedDatabases)
		require.NotNil(t, opts.AllowedShares)
		assert.Equal(t, []string{"SHARE1"}, *opts.AllowedShares)
		require.NotNil(t, opts.AllowedIntegrationTypes)
		assert.Equal(t, []string{"API INTEGRATIONS"}, *opts.AllowedIntegrationTypes)
	})

	t.Run("CommentSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.Comment = testutil.Ptr("existing comment")

		obs := successfulObservation()
		obs.ShowOutput.Comment = "existing comment"

		id := snowflake.NewAccountObjectIdentifier("MY_FG")
		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.Comment)
	})
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("NameDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.Name = "NEW_NAME"
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("CommentDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.Comment = testutil.Ptr("expected comment")
		obs := successfulObservation()
		obs.ShowOutput.Comment = "actual comment"
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("ObjectTypesDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.ObjectTypes = []string{"DATABASES", "ROLES", "USERS"}
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("AllowedAccountsDrift", func(t *testing.T) {
		t.Parallel()
		obj := newTestFailoverGroup("x", "default")
		obj.Spec.AllowedAccounts = []string{"MYORG.ACCT2", "MYORG.ACCT3"}
		obs := successfulObservation()
		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})
}

// --------------------------------------------------------------------------
// Tests: Tracked parameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("RequiredFieldsOnly", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FailoverGroupSpec{
			Name:            "MY_FG",
			ObjectTypes:     []string{"DATABASES"},
			AllowedAccounts: []string{"MYORG.ACCT1"},
		}
		fields := tracked.ComputeTracked(spec)
		// ObjectTypes and AllowedAccounts have nounset tag, so they should not
		// be tracked. Only optional fields with values get tracked.
		assert.NotContains(t, fields, "COMMENT")
		assert.NotContains(t, fields, "REPLICATION_SCHEDULE")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FailoverGroupSpec{
			Name:            "MY_FG",
			ObjectTypes:     []string{"DATABASES"},
			AllowedAccounts: []string{"MYORG.ACCT1"},
			Comment:         testutil.Ptr("hello"),
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "COMMENT")
	})

	t.Run("WithReplicationSchedule", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.FailoverGroupSpec{
			Name:                "MY_FG",
			ObjectTypes:         []string{"DATABASES"},
			AllowedAccounts:     []string{"MYORG.ACCT1"},
			ReplicationSchedule: testutil.Ptr("10 MINUTE"),
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "REPLICATION_SCHEDULE")
	})
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_ObserveFails(t *testing.T) {
	t.Parallel()

	obj := newTestFailoverGroup("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.FailoverGroupObservation, error) {
			return nil, fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	assert.Error(t, err)

	got := &snowplanev1alpha1.FailoverGroup{}
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

	obj := &snowplanev1alpha1.FailoverGroup{}
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "MY_FG", obj.Status.FullyQualifiedName)
	assert.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "MY_FG", obj.Status.ShowOutput.Name)
	assert.True(t, obj.Status.ShowOutput.IsPrimary)
	assert.Equal(t, "ACCOUNTADMIN", obj.Status.ShowOutput.Owner)
}

// --------------------------------------------------------------------------
// Tests: parseCommaList helper
// --------------------------------------------------------------------------

func TestParseCommaList(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, parseCommaList(""))
	})

	t.Run("SingleValue", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"DATABASES"}, parseCommaList("DATABASES"))
	})

	t.Run("MultipleValues", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"DATABASES", "ROLES", "USERS"}, parseCommaList("DATABASES, ROLES, USERS"))
	})

	t.Run("WhitespaceOnly", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, parseCommaList("  "))
	})
}
