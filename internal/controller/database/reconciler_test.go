package database

import (
	"context"
	"fmt"
	"testing"
	"time"

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

const testOriginalComment = "original"

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn     func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error)
	createFn      func(ctx context.Context, opts snowflake.CreateDatabaseOptions) error
	alterFn       func(ctx context.Context, opts snowflake.AlterDatabaseOptions) error
	dropFn        func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
	dropCascadeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.DatabaseObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateDatabaseOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterDatabaseOptions) error {
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

func (m *mockService) DropCascade(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	if m.dropCascadeFn != nil {
		return m.dropCascadeFn(ctx, name)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestDB(name, namespace string) *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "ANALYTICS",
		},
	}
}

// successfulObservation returns a standard existing-database observation.
func successfulObservation() *snowflake.DatabaseObservation {
	return &snowflake.DatabaseObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.DatabaseShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          "ANALYTICS",
			Kind:          "STANDARD",
			Comment:       "",
			Owner:         "SYSADMIN",
			RetentionTime: 1,
		},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays:    testutil.Ptr(int32(1)),
			MaxDataExtensionTimeInDays: testutil.Ptr(int32(14)),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   testutil.Ptr(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

// newTestReconciler builds a reconciler with a fake k8s client and injected mock service.
func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Database{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()

	r := NewReconcilerWithServiceFactory(c, factory, record.NewFakeRecorder(100), nil,
		func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("Database")

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
			return newTestDB(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.Database{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateDatabase(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	// Pre-add finalizer so we skip the finalizer-add requeue.
	db.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateDatabaseOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil // first: not found
				}

				return obs, nil // second: post-create verify
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	// Verify create options.
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.Name())
	assert.False(t, capturedOpts.Transient)

	// Verify status.
	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
}

func TestReconcile_CreateTransientDatabase(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.Transient = true

	var capturedOpts snowflake.CreateDatabaseOptions
	obs := successfulObservation()
	obs.ShowOutput.Kind = "TRANSIENT"

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.Transient)
}

func TestReconcile_CreateWithAllOptions(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	ssp := snowplanev1alpha1.StorageSerializationPolicyOptimized
	ll := snowplanev1alpha1.LogLevelInfo
	ml := snowplanev1alpha1.MetricLevelAll
	tl := snowplanev1alpha1.TraceLevelAlways

	db.Spec.Comment = testutil.Ptr("test db")
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(7))
	db.Spec.MaxDataExtensionTimeInDays = testutil.Ptr(int32(28))
	db.Spec.Catalog = testutil.Ptr("iceberg_cat")
	db.Spec.ExternalVolume = testutil.Ptr("vol1")
	db.Spec.ReplaceInvalidCharacters = testutil.Ptr(true)
	db.Spec.DefaultDDLCollation = testutil.Ptr("en-ci")
	db.Spec.StorageSerializationPolicy = &ssp
	db.Spec.LogLevel = &ll
	db.Spec.MetricLevel = &ml
	db.Spec.TraceLevel = &tl

	var capturedOpts snowflake.CreateDatabaseOptions

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil
				}

				return successfulObservation(), nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	assert.Equal(t, "test db", *capturedOpts.Comment)
	assert.Equal(t, int32(7), *capturedOpts.DataRetentionTimeInDays)
	assert.Equal(t, int32(28), *capturedOpts.MaxDataExtensionTimeInDays)
	assert.Equal(t, "iceberg_cat", *capturedOpts.Catalog)
	assert.Equal(t, "vol1", *capturedOpts.ExternalVolume)
	assert.Equal(t, true, *capturedOpts.ReplaceInvalidCharacters)
	assert.Equal(t, "en-ci", *capturedOpts.DefaultDDLCollation)
	assert.Equal(t, "OPTIMIZED", *capturedOpts.StorageSerializationPolicy)
	assert.Equal(t, "INFO", *capturedOpts.LogLevel)
	assert.Equal(t, "ALL", *capturedOpts.MetricLevel)
	assert.Equal(t, "ALWAYS", *capturedOpts.TraceLevel)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return &snowflake.DatabaseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err) // original error returned for controller-runtime backoff
	assert.Contains(t, err.Error(), "permission denied")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return &snowflake.DatabaseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid SQL"))
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Status.ObservedGeneration = 1
	db.Generation = 2
	db.Spec.Comment = testutil.Ptr("new comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterDatabaseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDatabaseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	// ObservedGeneration == Generation, but observed state differs from spec → drift.
	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Generation = 1
	db.Status.ObservedGeneration = 1 // same generation → drift path
	db.Spec.Comment = testutil.Ptr("desired comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted comment" // different from desired

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDatabaseOptions) error {
			alterCalled = true
			assert.Equal(t, "desired comment", *opts.Comment)
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter should be called for drift correction")
}

func TestReconcile_DriftCorrection_DataRetention(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Generation = 1
	db.Status.ObservedGeneration = 1
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(30)) // desired

	obs := successfulObservation()
	obs.Parameters.DataRetentionTimeInDays = testutil.Ptr(int32(1)) // actual

	var capturedAlterOpts snowflake.AlterDatabaseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDatabaseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	require.NotNil(t, capturedAlterOpts.DataRetentionTimeInDays)
	assert.Equal(t, int32(30), *capturedAlterOpts.DataRetentionTimeInDays)
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Status.ObservedGeneration = 1
	db.Spec.Comment = testutil.Ptr("changed")

	obs := successfulObservation()
	obs.ShowOutput.Comment = testOriginalComment

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
			return fmt.Errorf("alter failed")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter failed")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_AlterTerminalError(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Status.ObservedGeneration = 1
	db.Spec.Comment = testutil.Ptr("bad")

	obs := successfulObservation()
	obs.ShowOutput.Comment = testOriginalComment

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("terminal: bad syntax"))
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Observe errors
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err) // original error returned for backoff
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteDatabase(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "ANALYTICS", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	// After finalizer removal, the object should be gone (garbage collected by fake client).
	got := &snowplanev1alpha1.Database{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err), "object should be deleted after finalizer removal")
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	db.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled, "should not drop when orphan policy")
}

func TestReconcile_DeleteAlreadyGone(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return snowflake.ErrObjectNotFound
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")

	// Finalizer should still be present.
	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

func TestReconcile_DeleteNoFinalizer(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{"some-other-finalizer"} // has a finalizer so fake client accepts it, but not ours
	now := metav1.Now()
	db.DeletionTimestamp = &now

	mock := &mockService{}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_DeleteCascadeWithForceDestroy(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceDestroy: "true",
	}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	var cascadeCalled bool

	mock := &mockService{
		dropCascadeFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			cascadeCalled = true
			assert.Equal(t, "ANALYTICS", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, cascadeCalled, "should use cascade drop with force-destroy annotation")
}

func TestReconcile_DeleteCascadeDropFails(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceDestroy: "true",
	}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	mock := &mockService{
		dropCascadeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("cascade drop failed")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cascade drop failed")

	// Finalizer should still be present.
	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

func TestReconcile_DeleteWithoutForceDestroyUsesRegularDrop(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	// No force-destroy annotation
	now := metav1.Now()
	db.DeletionTimestamp = &now

	var dropCalled bool
	var cascadeCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
		dropCascadeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			cascadeCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled, "should use regular drop without force-destroy annotation")
	assert.False(t, cascadeCalled, "should NOT use cascade drop without force-destroy annotation")
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableTransientChange(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Generation = 2
	db.Status.ObservedGeneration = 1 // past initial creation
	db.Spec.Transient = true         // changed from STANDARD to transient
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{
		Kind: "STANDARD", // was standard
	}

	mock := &mockService{} // observe/alter should never be called

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	termCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, termCond)
	assert.Contains(t, termCond.Message, "spec.transient is immutable")
}

func TestReconcile_ImmutableField_FirstReconcile_Skipped(t *testing.T) {
	t.Parallel()

	// ObservedGeneration == 0 means first reconcile; immutable check should be skipped.
	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.Transient = true
	db.Status.ObservedGeneration = 0

	observeCall := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			observeCall++
			if observeCall == 1 {
				return &snowflake.DatabaseObservation{Exists: false}, nil
			}

			return &snowflake.DatabaseObservation{Exists: true}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	// Should not hit a terminal error for transient mismatch on first reconcile.
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: buildAlterOptions (unit)
// --------------------------------------------------------------------------

func TestBuildAlterOptions_NilParameters(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(30))

	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")
	obs := &snowflake.DatabaseObservation{
		Exists:     true,
		ShowOutput: &snowplanev1alpha1.DatabaseShowOutput{Name: "ANALYTICS"},
		Parameters: nil, // nil → early return
	}

	opts := buildAlterOptions(db, id, obs)
	// With nil Parameters, only comment diff is computed.
	assert.Nil(t, opts.DataRetentionTimeInDays, "should not set retention when Parameters is nil")
}

func TestBuildAlterOptions_AllDiffs(t *testing.T) {
	t.Parallel()

	ssp := snowplanev1alpha1.StorageSerializationPolicyOptimized
	ll := snowplanev1alpha1.LogLevelInfo
	ml := snowplanev1alpha1.MetricLevelAll
	tl := snowplanev1alpha1.TraceLevelAlways

	db := newTestDB("mydb", "default")
	db.Spec.Comment = testutil.Ptr("new")
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(30))
	db.Spec.MaxDataExtensionTimeInDays = testutil.Ptr(int32(28))
	db.Spec.DefaultDDLCollation = testutil.Ptr("en-ci")
	db.Spec.ReplaceInvalidCharacters = testutil.Ptr(true)
	db.Spec.Catalog = testutil.Ptr("cat")
	db.Spec.ExternalVolume = testutil.Ptr("vol")
	db.Spec.StorageSerializationPolicy = &ssp
	db.Spec.LogLevel = &ll
	db.Spec.MetricLevel = &ml
	db.Spec.TraceLevel = &tl

	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")
	obs := successfulObservation() // returns defaults that differ from spec

	opts := buildAlterOptions(db, id, obs)

	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new", *opts.Comment)
	assert.Equal(t, int32(30), *opts.DataRetentionTimeInDays)
	assert.Equal(t, int32(28), *opts.MaxDataExtensionTimeInDays)
	assert.Equal(t, "en-ci", *opts.DefaultDDLCollation)
	assert.Equal(t, true, *opts.ReplaceInvalidCharacters)
	assert.Equal(t, "cat", *opts.Catalog)
	assert.Equal(t, "vol", *opts.ExternalVolume)
	assert.Equal(t, "OPTIMIZED", *opts.StorageSerializationPolicy)
	assert.Equal(t, "INFO", *opts.LogLevel)
	assert.Equal(t, "ALL", *opts.MetricLevel)
	assert.Equal(t, "ALWAYS", *opts.TraceLevel)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	// Leave all spec fields nil — nothing to diff.

	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")
	obs := successfulObservation()

	opts := buildAlterOptions(db, id, obs)
	assert.False(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: buildCreateOptions (unit)
// --------------------------------------------------------------------------

func TestBuildCreateOptions_Minimal(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")

	opts := buildCreateOptions(db, id)
	assert.Equal(t, "ANALYTICS", opts.Name.Name())
	assert.False(t, opts.Transient)
	assert.Nil(t, opts.Comment)
	assert.Nil(t, opts.DataRetentionTimeInDays)
}

func TestBuildCreateOptions_Full(t *testing.T) {
	t.Parallel()

	ssp := snowplanev1alpha1.StorageSerializationPolicyOptimized
	ll := snowplanev1alpha1.LogLevelDebug
	ml := snowplanev1alpha1.MetricLevelAll
	tl := snowplanev1alpha1.TraceLevelOnEvent

	db := newTestDB("mydb", "default")
	db.Spec.Comment = testutil.Ptr("c")
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(7))
	db.Spec.MaxDataExtensionTimeInDays = testutil.Ptr(int32(14))
	db.Spec.Transient = true
	db.Spec.Catalog = testutil.Ptr("cat")
	db.Spec.ExternalVolume = testutil.Ptr("vol")
	db.Spec.ReplaceInvalidCharacters = testutil.Ptr(true)
	db.Spec.DefaultDDLCollation = testutil.Ptr("en-ci")
	db.Spec.StorageSerializationPolicy = &ssp
	db.Spec.LogLevel = &ll
	db.Spec.MetricLevel = &ml
	db.Spec.TraceLevel = &tl

	id := snowflake.NewAccountObjectIdentifier("DB1")
	opts := buildCreateOptions(db, id)

	assert.True(t, opts.Transient)
	assert.Equal(t, "c", *opts.Comment)
	assert.Equal(t, int32(7), *opts.DataRetentionTimeInDays)
	assert.Equal(t, int32(14), *opts.MaxDataExtensionTimeInDays)
	assert.Equal(t, "cat", *opts.Catalog)
	assert.Equal(t, "vol", *opts.ExternalVolume)
	assert.Equal(t, true, *opts.ReplaceInvalidCharacters)
	assert.Equal(t, "en-ci", *opts.DefaultDDLCollation)
	assert.Equal(t, "OPTIMIZED", *opts.StorageSerializationPolicy)
	assert.Equal(t, "DEBUG", *opts.LogLevel)
	assert.Equal(t, "ALL", *opts.MetricLevel)
	assert.Equal(t, "ON_EVENT", *opts.TraceLevel)
}

// --------------------------------------------------------------------------
// Tests: applyObservation (unit)
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	obs := successfulObservation()

	applyObservation(db, obs)

	assert.NotEmpty(t, db.Status.FullyQualifiedName)
	assert.Equal(t, "SYSADMIN", db.Status.ShowOutput.Owner)
	assert.Equal(t, "2024-01-01", db.Status.ShowOutput.CreatedOn)
	require.NotNil(t, db.Status.ShowOutput)
	assert.Equal(t, "ANALYTICS", db.Status.ShowOutput.Name)
	assert.Equal(t, "STANDARD", db.Status.ShowOutput.Kind)
}

func TestApplyObservation_PreservesCreatedOn(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")

	obs := successfulObservation()
	obs.ShowOutput.CreatedOn = "2024-01-01"

	applyObservation(db, obs)

	// CreatedOn is now always taken from ShowOutput.
	assert.Equal(t, "2024-01-01", db.Status.ShowOutput.CreatedOn)
}

// --------------------------------------------------------------------------
// Tests: validateImmutableFields (unit)
// --------------------------------------------------------------------------

func TestValidateImmutableFields_FirstReconcile(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Status.ObservedGeneration = 0

	err := validateImmutableFields(context.Background(), db)
	assert.NoError(t, err, "should skip on first reconcile")
}

func TestValidateImmutableFields_TransientChanged(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Status.ObservedGeneration = 1
	db.Spec.Transient = true
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{Kind: "STANDARD"}

	err := validateImmutableFields(context.Background(), db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spec.transient is immutable")
}

func TestValidateImmutableFields_TransientUnchanged(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Status.ObservedGeneration = 1
	db.Spec.Transient = true
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{Kind: "TRANSIENT"}

	err := validateImmutableFields(context.Background(), db)
	assert.NoError(t, err)
}

func TestValidateImmutableFields_NoShowOutput(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Status.ObservedGeneration = 1
	db.Status.ShowOutput = nil // not yet observed

	err := validateImmutableFields(context.Background(), db)
	assert.NoError(t, err, "should skip when showOutput is nil")
}

// --------------------------------------------------------------------------
// Tests: ClearTerminal only after successful sync
// --------------------------------------------------------------------------

func TestReconcile_ClearTerminal_OnlyOnSuccess(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Status.ObservedGeneration = 1
	// Simulate a previous terminal condition.
	conditions.SetNotReady(db, "PreviousError", "old error")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	// Terminal should be cleared after successful sync.
	assert.False(t, conditions.IsTerminal(got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: RequeueAfter constant
// --------------------------------------------------------------------------

func TestRequeueInterval(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5*time.Minute, reconciler.DefaultRequeueInterval)
}

// --------------------------------------------------------------------------
// Tests: Post-create observe error (F13)
// --------------------------------------------------------------------------

func TestReconcile_CreatePostObserveError(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil // first: not found
				}

				return nil, fmt.Errorf("observe timeout") // second: post-create fails
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "post-create observe")
	assert.Zero(t, result.RequeueAfter, "error return should let controller-runtime apply exponential backoff")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

// --------------------------------------------------------------------------
// Tests: Event emission verification (F15)
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
	assert.Contains(t, events[0], "created")
}

func TestReconcile_EventEmission_Update(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Generation = 2
	db.Spec.Comment = testutil.Ptr("new comment")
	db.Status.ObservedGeneration = 1

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "ReconcileSuccess")
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Deleting")
}

func TestReconcile_EventEmission_CreateFails(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return &snowflake.DatabaseObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Warning")
	assert.Contains(t, events[0], "ReconcileError")
}

// --------------------------------------------------------------------------
// Tests: UNSET support
// --------------------------------------------------------------------------

func TestBuildAlterOptions_UnsetDetection(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Status.TrackedParameters = []string{"COMMENT", "LOG_LEVEL", "DATA_RETENTION_TIME_IN_DAYS"}
	// Spec has no comment, no log level, no retention — all three should be unset.

	obs := successfulObservation()
	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")

	opts := buildAlterOptions(db, id, obs)
	assert.ElementsMatch(t, []string{"COMMENT", "LOG_LEVEL", "DATA_RETENTION_TIME_IN_DAYS"}, opts.UnsetFields)
}

func TestBuildAlterOptions_NoUnsetWhenFieldStillSet(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Spec.Comment = testutil.Ptr("still here")
	db.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")

	opts := buildAlterOptions(db, id, obs)
	assert.Empty(t, opts.UnsetFields)
}

func TestBuildAlterOptions_NoUnsetWhenNoTrackedParameters(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	// No TrackedParameters in status — should never attempt UNSET.

	obs := successfulObservation()
	id := snowflake.NewAccountObjectIdentifier("ANALYTICS")

	opts := buildAlterOptions(db, id, obs)
	assert.Empty(t, opts.UnsetFields)
}

func TestComputeDatabaseTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.DatabaseSpec{
		Comment:                 testutil.Ptr("x"),
		DataRetentionTimeInDays: testutil.Ptr(int32(7)),
	}

	fields := tracked.ComputeTracked(spec)
	assert.ElementsMatch(t, []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"}, fields)
}

func TestComputeDatabaseTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.DatabaseSpec{}
	fields := tracked.ComputeTracked(spec)
	assert.Empty(t, fields)
}

func TestReconcile_TrackedParametersPersistedOnCreate(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.Comment = testutil.Ptr("hello")
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(7))

	obs := successfulObservation()
	obs.ShowOutput.Comment = "hello"
	obs.Parameters.DataRetentionTimeInDays = testutil.Ptr(int32(7))

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DatabaseObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.ElementsMatch(t, []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"}, got.Status.TrackedParameters)
}

func TestReconcile_UnsetTriggered(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Generation = 2
	db.Status.ObservedGeneration = 1
	// Previously managed comment, now removed from spec.
	db.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	obs.ShowOutput.Comment = testOriginalComment

	var capturedAlterOpts snowflake.AlterDatabaseOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDatabaseOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	assert.Contains(t, capturedAlterOpts.UnsetFields, "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Recoverable condition
// --------------------------------------------------------------------------

func TestReconcile_RecoverableConditionOnTransientError(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_RecoverableClearedOnSuccess(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Status.ObservedGeneration = 1

	// Pre-set the Recoverable condition to simulate a previous transient error.
	conditions.SetNotReady(db, snowplanev1alpha1.ReasonReconcileError, "previous error")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: Deletion with missing ProviderConfig
// --------------------------------------------------------------------------

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	// No ProviderConfig or Secret — provider resolution will fail.
	mock := &mockService{}
	r := newTestReconciler(mock, db)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Finalizer should be removed — object should be gone.
	got := &snowplanev1alpha1.Database{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err), "finalizer should be removed when PC is missing during deletion")
}

// --------------------------------------------------------------------------
// Tests: Immutable name validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.Name = "NEW_NAME"
	db.Status.ObservedGeneration = 1
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{
		Name: "OLD_NAME",
		Kind: "STANDARD",
	}

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Drop sets Recoverable condition
// --------------------------------------------------------------------------

func TestReconcile_DeleteDropFailsSetsRecoverable(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	now := metav1.Now()
	db.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			return fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: Drift detection — DriftDetected condition & events
// --------------------------------------------------------------------------

func TestReconcile_DriftCorrection_SetsDriftDetectedCondition(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	db.Generation = 1
	db.Status.ObservedGeneration = 1 // drift path
	db.Spec.Comment = testutil.Ptr("desired")
	hash, err := snowplanev1alpha1.ComputeSpecHash(db.Spec)
	require.NoError(t, err)
	db.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted"

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	// After successful correction, DriftDetected should be cleared.
	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeDriftDetected),
		"DriftDetected condition should be cleared after successful correction")

	// Check events.
	recorder := r.Recorder.(*record.FakeRecorder)
	var events []string
	for len(recorder.Events) > 0 {
		events = append(events, <-recorder.Events)
	}

	// We expect both a DriftDetected warning and a DriftCorrected normal event.
	assert.True(t, testutil.ContainsEvent(events, "DriftDetected"), "expected DriftDetected event, got: %v", events)
	assert.True(t, testutil.ContainsEvent(events, "DriftCorrected"), "expected DriftCorrected event, got: %v", events)
}

func TestReconcile_DriftDetectOnlyPolicy(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Generation = 1
	db.Status.ObservedGeneration = 1 // drift path
	db.Spec.ManagementPolicies.DriftPolicy = snowplanev1alpha1.DriftPolicyDetectOnly
	db.Spec.Comment = testutil.Ptr("desired")
	hash, err := snowplanev1alpha1.ComputeSpecHash(db.Spec)
	require.NoError(t, err)
	db.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted"

	alterCalled := false

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterDatabaseOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))

	// DriftDetected condition should remain set.
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeDriftDetected),
		"DriftDetected condition should be set with detect-only policy")

	// Check events — should have DriftDetected, but NOT DriftCorrected.
	recorder := r.Recorder.(*record.FakeRecorder)
	var events []string
	for len(recorder.Events) > 0 {
		events = append(events, <-recorder.Events)
	}

	assert.True(t, testutil.ContainsEvent(events, "DriftDetected"), "expected DriftDetected event, got: %v", events)
	assert.False(t, testutil.ContainsEvent(events, "DriftCorrected"), "should NOT have DriftCorrected event, got: %v", events)
}

func TestReconcile_DriftClearedAfterCorrection(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Generation = 1
	db.Status.ObservedGeneration = 1
	db.Spec.Comment = testutil.Ptr("desired")

	// Pre-set DriftDetected condition (simulating a previous reconcile).
	conditions.SetDriftDetected(db, "previous drift")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "desired" // no drift — already matches

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeDriftDetected),
		"DriftDetected should be cleared when no drift is present")
}

func TestDetectDatabaseDrift_NoDrift(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		Spec: snowplanev1alpha1.DatabaseSpec{
			Comment:                 testutil.Ptr("test"),
			DataRetentionTimeInDays: testutil.Ptr(int32(1)),
		},
	}

	obs := &snowflake.DatabaseObservation{
		ShowOutput: &snowplanev1alpha1.DatabaseShowOutput{Comment: "test"},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays: testutil.Ptr(int32(1)),
		},
	}

	result := detectDrift(db, obs)
	assert.False(t, result.HasDrift)
	assert.Empty(t, result.Changes)
}

func TestDetectDatabaseDrift_WithDrift(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		Spec: snowplanev1alpha1.DatabaseSpec{
			Comment:                 testutil.Ptr("desired"),
			DataRetentionTimeInDays: testutil.Ptr(int32(30)),
		},
	}

	obs := &snowflake.DatabaseObservation{
		ShowOutput: &snowplanev1alpha1.DatabaseShowOutput{Comment: "drifted"},
		Parameters: &snowflake.DatabaseParameters{
			DataRetentionTimeInDays: testutil.Ptr(int32(1)),
		},
	}

	result := detectDrift(db, obs)
	assert.True(t, result.HasDrift)
	assert.Len(t, result.Changes, 2)
	assert.Contains(t, result.Summary(), "COMMENT")
	assert.Contains(t, result.Summary(), "DATA_RETENTION_TIME_IN_DAYS")
}

// --------------------------------------------------------------------------
// Tests: UNSET computed before nil-params guard
// --------------------------------------------------------------------------

func TestBuildAlterOptions_UnsetComputedWhenParamsNil(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		Spec: snowplanev1alpha1.DatabaseSpec{Name: "DB"},
		Status: snowplanev1alpha1.DatabaseStatus{
			TrackedParameters: []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"},
		},
	}

	id := snowflake.NewAccountObjectIdentifier("DB")
	obs := &snowflake.DatabaseObservation{
		Exists:     true,
		ShowOutput: &snowplanev1alpha1.DatabaseShowOutput{Name: "DB"},
		Parameters: nil, // Parameters are nil
	}

	opts := buildAlterOptions(db, id, obs)
	// Even with nil parameters, UNSET should still be computed from TrackedParameters.
	assert.Contains(t, opts.UnsetFields, "COMMENT")
	assert.Contains(t, opts.UnsetFields, "DATA_RETENTION_TIME_IN_DAYS")
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: Ownership drift
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Generation = 1
	db.Status.ObservedGeneration = 1
	db.Spec.UseRole = testutil.Ptr("DATA_ADMIN")

	obs := successfulObservation()
	obs.ShowOutput.Owner = "DATA_ADMIN" // matches to avoid drift noise

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	var capturedUseRole string

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Database{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(db, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	rec := record.NewFakeRecorder(100)

	r := NewReconcilerWithServiceFactory(c, testutil.NewTestClientFactory(), rec, nil,
		func(_ context.Context, _ clientfactory.SnowflakeClient, useRole string) (Service, func(context.Context), error) {
			capturedUseRole = useRole
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("Database")

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	require.NoError(t, err)

	assert.Equal(t, "DATA_ADMIN", capturedUseRole, "useRole from spec should be passed to ServiceFactory")
}

// --------------------------------------------------------------------------
// Tests: ForceNew annotation bypasses immutable field checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_ForceNewBypass(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "true",
	}
	db.Status.ObservedGeneration = 1
	db.Spec.Name = "NEW_NAME"
	db.Spec.Transient = true
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{
		Name: "OLD_NAME",
		Kind: "STANDARD",
	}

	err := validateImmutableFields(context.Background(), db)
	assert.NoError(t, err, "force-new should bypass immutable checks")
}

func TestValidateImmutableFields_ForceNewFalse_StillRejects(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "false",
	}
	db.Status.ObservedGeneration = 1
	db.Spec.Name = "NEW_NAME"
	db.Status.ShowOutput = &snowplanev1alpha1.DatabaseShowOutput{
		Name: "OLD_NAME",
		Kind: "STANDARD",
	}

	err := validateImmutableFields(context.Background(), db)
	assert.Error(t, err, "force-new=false should still reject immutable changes")
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

// --------------------------------------------------------------------------
// Tests: Spec validation defense-in-depth
// --------------------------------------------------------------------------

func TestReconcile_SpecValidation_RejectsEmptyUseRole(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	empty := ""
	db.Spec.UseRole = &empty

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	assert.NoError(t, err, "should return nil (terminal, no requeue)")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))

	termCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, termCond)
	assert.Contains(t, termCond.Message, "spec.useRole must not be an empty string")
}

func TestReconcile_SpecValidation_RejectsRetentionOutOfRange(t *testing.T) {
	t.Parallel()

	db := newTestDB("mydb", "default")
	db.Finalizers = []string{finalizerName}
	db.Spec.DataRetentionTimeInDays = testutil.Ptr(int32(100))

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydb", "default"))
	assert.NoError(t, err, "should return nil (terminal, no requeue)")

	got := &snowplanev1alpha1.Database{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydb", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))

	termCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, termCond)
	assert.Contains(t, termCond.Message, "spec.dataRetentionTimeInDays")
}
