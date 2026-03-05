package shareddatabase

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return nil, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSharedDatabaseOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSharedDatabaseOptions) error {
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

func newTestSharedDatabase(name, namespace string) *snowplanev1alpha1.SharedDatabase {
	return &snowplanev1alpha1.SharedDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SharedDatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:      "SHARED_DB",
			FromShare: "ab67890.sales_s",
			Comment:   testutil.Ptr("test shared db"),
		},
	}
}

func successfulObservation() *snowflake.SharedDatabaseObservation {
	return &snowflake.SharedDatabaseObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SharedDatabaseShowOutput{
			CreatedOn:     "2025-01-01 00:00:00",
			Name:          "SHARED_DB",
			Kind:          "IMPORTED DATABASE",
			Comment:       "test shared db",
			Owner:         "SYSADMIN",
			RetentionTime: 0,
			Origin:        "ab67890.sales_s",
		},
		Parameters: &snowflake.DatabaseParameters{
			ExternalVolume:             "",
			Catalog:                    "",
			DefaultDDLCollation:        "",
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			TraceLevel:                 "OFF",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SharedDatabase, Service, *snowflake.SharedDatabaseObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.SharedDatabase{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SharedDatabase")

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
			return newTestSharedDatabase(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SharedDatabase{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	obj := newTestSharedDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateSharedDatabaseOptions

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SharedDatabaseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSharedDatabaseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "SHARED_DB", capturedOpts.Name.Name())
	assert.Equal(t, "ab67890.sales_s", capturedOpts.FromShare)

	got := &snowplanev1alpha1.SharedDatabase{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myobj", Namespace: "default"}, got)
	require.NoError(t, err)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "SHARED_DB", got.Status.FullyQualifiedName)
}

// --------------------------------------------------------------------------
// Tests: Alter
// --------------------------------------------------------------------------

func TestReconcile_Alter(t *testing.T) {
	t.Parallel()

	obj := newTestSharedDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}
	obj.Status.ObservedGeneration = 1
	obj.Spec.Comment = testutil.Ptr("updated comment")
	obj.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()

	var capturedAlterOpts snowflake.AlterSharedDatabaseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSharedDatabaseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "SHARED_DB", capturedAlterOpts.Name.Name())
	require.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "updated comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	obj := newTestSharedDatabase("myobj", "default")
	obj.DeletionTimestamp = &now
	obj.Finalizers = []string{finalizerName}

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "SHARED_DB", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.SharedDatabase{}
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

		obj := &snowplanev1alpha1.SharedDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.FromShare = "org.acct.share"
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SharedDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.FromShare = "org.acct.share"
		obj.Status.ShowOutput = &snowplanev1alpha1.SharedDatabaseShowOutput{
			Name:   "MY_DB",
			Origin: "org.acct.share",
		}
		assert.NoError(t, validateImmutableFields(context.Background(), obj))
	})

	t.Run("NameChanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SharedDatabase{}
		obj.Spec.Name = "NEW_NAME"
		obj.Spec.FromShare = "org.acct.share"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.SharedDatabaseShowOutput{
			Name:   "OLD_NAME",
			Origin: "org.acct.share",
		}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
	})

	t.Run("FromShareChanged", func(t *testing.T) {
		t.Parallel()

		obj := &snowplanev1alpha1.SharedDatabase{}
		obj.Spec.Name = "MY_DB"
		obj.Spec.FromShare = "other.org.share"
		obj.Status.ObservedGeneration = 1
		obj.Status.ShowOutput = &snowplanev1alpha1.SharedDatabaseShowOutput{
			Name:   "MY_DB",
			Origin: "org.acct.share",
		}
		err := validateImmutableFields(context.Background(), obj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "immutable")
		assert.Contains(t, err.Error(), "fromShare")
	})
}

// --------------------------------------------------------------------------
// Tests: Build create options
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	obj := newTestSharedDatabase("x", "default")
	id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
	opts := buildCreateOptions(obj, id)

	assert.Equal(t, "SHARED_DB", opts.Name.Name())
	assert.Equal(t, "ab67890.sales_s", opts.FromShare)
}

// --------------------------------------------------------------------------
// Tests: Build alter options
// --------------------------------------------------------------------------

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("CommentChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obj.Spec.Comment = testutil.Ptr("new comment")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
	})

	t.Run("CommentSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.Comment) // same as observed
	})

	t.Run("ExternalVolumeChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obj.Spec.ExternalVolume = testutil.Ptr("my_vol")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.ExternalVolume)
		assert.Equal(t, "my_vol", *opts.ExternalVolume)
	})

	t.Run("ExternalVolumeSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obj.Spec.ExternalVolume = testutil.Ptr("")
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.ExternalVolume)
	})

	t.Run("StorageSerializationPolicyChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		policy := snowplanev1alpha1.StorageSerializationPolicyOptimized
		obj.Spec.StorageSerializationPolicy = &policy
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.StorageSerializationPolicy)
		assert.Equal(t, "OPTIMIZED", *opts.StorageSerializationPolicy)
	})

	t.Run("StorageSerializationPolicySkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		policy := snowplanev1alpha1.StorageSerializationPolicyCompatible
		obj.Spec.StorageSerializationPolicy = &policy
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.StorageSerializationPolicy)
	})

	t.Run("LogLevelChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		lvl := snowplanev1alpha1.LogLevelInfo
		obj.Spec.LogLevel = &lvl
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.LogLevel)
		assert.Equal(t, "INFO", *opts.LogLevel)
	})

	t.Run("TraceLevelChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		lvl := snowplanev1alpha1.TraceLevelAlways
		obj.Spec.TraceLevel = &lvl
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.TraceLevel)
		assert.Equal(t, "ALWAYS", *opts.TraceLevel)
	})

	t.Run("UnsetFieldsComputed", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obj.Spec.Comment = nil // was tracked, now removed
		obj.Status.TrackedParameters = []string{"COMMENT"}
		id := snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
		obs := successfulObservation()

		opts := buildAlterOptions(obj, id, obs)
		assert.Contains(t, opts.UnsetFields, "COMMENT")
	})
}

// --------------------------------------------------------------------------
// Tests: Detect drift
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		policy := snowplanev1alpha1.StorageSerializationPolicyCompatible
		obj.Spec.StorageSerializationPolicy = &policy
		logLvl := snowplanev1alpha1.LogLevelOff
		obj.Spec.LogLevel = &logLvl
		traceLvl := snowplanev1alpha1.TraceLevelOff
		obj.Spec.TraceLevel = &traceLvl
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
		assert.False(t, result.HasImmutableViolation)
	})

	t.Run("NameDrift_HasImmutableViolation", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Name = "DIFFERENT_NAME"

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift) // immutable violations tracked separately
		assert.True(t, result.HasImmutableViolation)
	})

	t.Run("CommentDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obs := successfulObservation()
		obs.ShowOutput.Comment = "changed by someone"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
		assert.False(t, result.HasImmutableViolation)
	})

	t.Run("ExternalVolumeDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		obj.Spec.ExternalVolume = testutil.Ptr("my_vol")
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("StoragePolicyDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestSharedDatabase("x", "default")
		policy := snowplanev1alpha1.StorageSerializationPolicyOptimized
		obj.Spec.StorageSerializationPolicy = &policy
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})
}

// --------------------------------------------------------------------------
// Tests: Compute tracked parameters
// --------------------------------------------------------------------------

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("AllFieldsSet", func(t *testing.T) {
		t.Parallel()

		policy := snowplanev1alpha1.StorageSerializationPolicyOptimized
		logLvl := snowplanev1alpha1.LogLevelInfo
		traceLvl := snowplanev1alpha1.TraceLevelAlways

		spec := &snowplanev1alpha1.SharedDatabaseSpec{
			Name:                       "x",
			FromShare:                  "org.share",
			Comment:                    testutil.Ptr("c"),
			ExternalVolume:             testutil.Ptr("vol"),
			Catalog:                    testutil.Ptr("cat"),
			DefaultDDLCollation:        testutil.Ptr("en-ci"),
			ReplaceInvalidCharacters:   testutil.Ptr(true),
			StorageSerializationPolicy: &policy,
			LogLevel:                   &logLvl,
			TraceLevel:                 &traceLvl,
		}
		params := tracked.ComputeTracked(spec)
		assert.Contains(t, params, "COMMENT")
		assert.Contains(t, params, "EXTERNAL_VOLUME")
		assert.Contains(t, params, "CATALOG")
		assert.Contains(t, params, "DEFAULT_DDL_COLLATION")
		assert.Contains(t, params, "REPLACE_INVALID_CHARACTERS")
		assert.Contains(t, params, "STORAGE_SERIALIZATION_POLICY")
		assert.Contains(t, params, "LOG_LEVEL")
		assert.Contains(t, params, "TRACE_LEVEL")
	})

	t.Run("MinimalFields", func(t *testing.T) {
		t.Parallel()

		spec := &snowplanev1alpha1.SharedDatabaseSpec{
			Name:      "x",
			FromShare: "org.share",
		}
		params := tracked.ComputeTracked(spec)
		assert.NotContains(t, params, "COMMENT")
		assert.NotContains(t, params, "EXTERNAL_VOLUME")
		assert.NotContains(t, params, "STORAGE_SERIALIZATION_POLICY")
	})
}

// --------------------------------------------------------------------------
// Tests: Apply observation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	obj := &snowplanev1alpha1.SharedDatabase{}
	obs := successfulObservation()

	applyObservation(obj, obs)

	assert.Equal(t, "SHARED_DB", obj.Status.FullyQualifiedName)
	require.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "SHARED_DB", obj.Status.ShowOutput.Name)
	assert.Equal(t, "ab67890.sales_s", obj.Status.ShowOutput.Origin)
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_ObserveFails(t *testing.T) {
	t.Parallel()

	obj := newTestSharedDatabase("myobj", "default")
	obj.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.SharedDatabaseObservation, error) {
			return nil, fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myobj", "default"))
	assert.Error(t, err)

	got := &snowplanev1alpha1.SharedDatabase{}
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
