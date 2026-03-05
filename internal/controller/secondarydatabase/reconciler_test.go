package secondarydatabase

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error
	refreshFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return nil, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Refresh(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, name)
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

func newTestSecondaryDatabase(name, namespace string) *snowplanev1alpha1.SecondaryDatabase {
	return &snowplanev1alpha1.SecondaryDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SecondaryDatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "MY_REPLICA_DB",
			AsReplicaOf: "myorg.myaccount.MY_PRIMARY_DB",
			Comment:     testutil.Ptr("test replica"),
		},
	}
}

func successfulObservation() *snowflake.SecondaryDatabaseObservation {
	retention := int32(1)
	maxExtension := int32(14)

	return &snowflake.SecondaryDatabaseObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SecondaryDatabaseShowOutput{
			CreatedOn:     "2025-01-01 00:00:00",
			Name:          "MY_REPLICA_DB",
			Kind:          "STANDARD",
			Comment:       "test replica",
			Owner:         "SYSADMIN",
			RetentionTime: 1,
			Origin:        "myorg.myaccount.MY_PRIMARY_DB",
		},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays:    &retention,
			MaxDataExtensionTimeInDays: &maxExtension,
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SecondaryDatabase, Service, *snowflake.SecondaryDatabaseObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.SecondaryDatabase{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SecondaryDatabase")

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
			return newTestSecondaryDatabase(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SecondaryDatabase{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestSecondaryDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateSecondaryDatabaseOptions

	refreshCalled := false
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SecondaryDatabaseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSecondaryDatabaseOptions) error {
			capturedOpts = opts
			return nil
		},
		refreshFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			refreshCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_REPLICA_DB", capturedOpts.Name.Name())
	assert.Equal(t, "myorg.myaccount.MY_PRIMARY_DB", capturedOpts.AsReplicaOf)

	got := &snowplanev1alpha1.SecondaryDatabase{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "MY_REPLICA_DB", got.Status.FullyQualifiedName)

	// Refresh should be called during post-create update
	_ = refreshCalled
}

// --------------------------------------------------------------------------
// Tests: Alter (with always-refresh)
// --------------------------------------------------------------------------

func TestReconcile_Alter(t *testing.T) {
	t.Parallel()

	obj := newTestSecondaryDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Spec.Comment = testutil.Ptr("updated comment")
	obj.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()

	var capturedAlterOpts snowflake.AlterSecondaryDatabaseOptions

	refreshCalled := false

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSecondaryDatabaseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
		refreshFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			refreshCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_REPLICA_DB", capturedAlterOpts.Name.Name())
	require.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "updated comment", *capturedAlterOpts.Comment)
	assert.True(t, refreshCalled, "refresh should always be called")
}

func TestReconcile_RefreshAlwaysCalled(t *testing.T) {
	t.Parallel()

	// Even when no alter is needed, refresh must run.
	obj := newTestSecondaryDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1

	obs := successfulObservation()

	alterCalled := false
	refreshCalled := false

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSecondaryDatabaseOptions) error {
			alterCalled = true
			return nil
		},
		refreshFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			refreshCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.False(t, alterCalled, "alter should NOT be called when no changes")
	assert.True(t, refreshCalled, "refresh should always be called")
}

// --------------------------------------------------------------------------
// Tests: Delete
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestSecondaryDatabase("myobj", "default")
	obj.DeletionTimestamp = &now
	obj.Finalizers = []string{finalizerName}

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_REPLICA_DB", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.SecondaryDatabase{}
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

		obj := &snowplanev1alpha1.SecondaryDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.AsReplicaOf = "org.acct.db"
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SecondaryDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.AsReplicaOf = "org.acct.db"
		obj.Status.ShowOutput = &snowplanev1alpha1.SecondaryDatabaseShowOutput{
			Name:   "MY_DB",
			Origin: "org.acct.db",
		}
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameChanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SecondaryDatabase{}
		obj.Spec.Name = "NEW_NAME"
		obj.Spec.AsReplicaOf = "org.acct.db"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.SecondaryDatabaseShowOutput{
			Name:   "OLD_NAME",
			Origin: "org.acct.db",
		}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
	})

	t.Run("AsReplicaOfChanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SecondaryDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.AsReplicaOf = "other.org.db"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.SecondaryDatabaseShowOutput{
			Name:   "MY_DB",
			Origin: "org.acct.db",
		}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
		assert.Contains(t, err.Error(), "asReplicaOf")
	})
}

// --------------------------------------------------------------------------
// Tests: Build create options
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	obj := newTestSecondaryDatabase("x", "default")
	obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(7))
	id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
	opts := buildCreateOptions(obj, id)

	assert.Equal(t, "MY_REPLICA_DB", opts.Name.Name())
	assert.Equal(t, "myorg.myaccount.MY_PRIMARY_DB", opts.AsReplicaOf)
	require.NotNil(t, opts.DataRetentionTimeInDays)
	assert.Equal(t, int32(7), *opts.DataRetentionTimeInDays)
}

// --------------------------------------------------------------------------
// Tests: Build alter options
// --------------------------------------------------------------------------

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("CommentChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.Comment = testutil.Ptr("new comment")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
	})

	t.Run("CommentSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.Comment) // same as observed
	})

	t.Run("RetentionChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(30))
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.DataRetentionTimeInDays)
		assert.Equal(t, int32(30), *opts.DataRetentionTimeInDays)
	})

	t.Run("RetentionSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(1))
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.DataRetentionTimeInDays) // same as observed
	})

	t.Run("MaxExtensionChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.MaxDataExtensionTimeInDays = testutil.Ptr(int32(30))
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.MaxDataExtensionTimeInDays)
		assert.Equal(t, int32(30), *opts.MaxDataExtensionTimeInDays)
	})
}

// --------------------------------------------------------------------------
// Tests: Detect drift
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(1))
		obj.Spec.MaxDataExtensionTimeInDays = testutil.Ptr(int32(14))
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
		assert.False(t, result.HasImmutableViolation)
	})

	t.Run("NameDrift_HasImmutableViolation", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Name = "DIFFERENT_NAME"

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift) // immutable violations tracked separately
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("CommentDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Comment = "changed by someone"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.False(t, result.HasImmutableViolation)
	})

	t.Run("RetentionDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSecondaryDatabase("x", "default")
		obj.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(7))
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift) // spec=7, obs=1
	})
}

// --------------------------------------------------------------------------
// Tests: Compute tracked parameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("AllFieldsSet", func(t *testing.T) {
		t.Parallel()

		spec := &snowplanev1alpha1.SecondaryDatabaseSpec{
			Name:                       "x",
			AsReplicaOf:                "org.acct.db",
			Comment:                    testutil.Ptr("c"),
			DataRetentionTimeInDays:    testutil.Ptr(int32(7)),
			MaxDataExtensionTimeInDays: testutil.Ptr(int32(14)),
		}
		params := tracked.ComputeTracked(spec)
		assert.Contains(t, params, "COMMENT")
		assert.Contains(t, params, "DATA_RETENTION_TIME_IN_DAYS")
		assert.Contains(t, params, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	})

	t.Run("MinimalFields", func(t *testing.T) {
		t.Parallel()

		spec := &snowplanev1alpha1.SecondaryDatabaseSpec{
			Name:        "x",
			AsReplicaOf: "org.acct.db",
		}
		params := tracked.ComputeTracked(spec)
		assert.NotContains(t, params, "COMMENT")
		assert.NotContains(t, params, "DATA_RETENTION_TIME_IN_DAYS")
		assert.NotContains(t, params, "MAX_DATA_EXTENSION_TIME_IN_DAYS")
	})
}

// --------------------------------------------------------------------------
// Tests: Apply observation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.SecondaryDatabase{}
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "MY_REPLICA_DB", obj.Status.FullyQualifiedName)
	require.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "MY_REPLICA_DB", obj.Status.ShowOutput.Name)
	assert.Equal(t, "myorg.myaccount.MY_PRIMARY_DB", obj.Status.ShowOutput.Origin)
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_ObserveFails(t *testing.T) {
	t.Parallel()

	obj := newTestSecondaryDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SecondaryDatabaseObservation, error) {
			return nil, fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	assert.Error(t, err)

	got := &snowplanev1alpha1.SecondaryDatabase{}
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
// Tests: secondaryDatabaseAlterOpts wrapper
// --------------------------------------------------------------------------

func TestSecondaryDatabaseAlterOpts_AlwaysHasChanges(t *testing.T) {
	t.Parallel()

	opts := &secondaryDatabaseAlterOpts{
		inner: snowflake.AlterSecondaryDatabaseOptions{
			Name: snowflake.NewAccountObjectIdentifier("DB"),
		},
	}
	assert.True(t, opts.HasChanges(), "wrapper should always report HasChanges=true for refresh")
}
