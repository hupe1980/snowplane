package dynamictable

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
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.DynamicTableObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error {
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

func newTestDynamicTable(name, namespace string) *snowplanev1alpha1.DynamicTable {
	return &snowplanev1alpha1.DynamicTable{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DynamicTableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:          "MY_DT",
			DatabaseName:  testutil.Ptr("MY_DB"),
			SchemaName:    testutil.Ptr("MY_SCHEMA"),
			Query:         "SELECT * FROM src",
			TargetLag:     "1 minute",
			WarehouseName: testutil.Ptr("MY_WH"),
		},
	}
}

func successfulObservation() *snowflake.DynamicTableObservation {
	return &snowflake.DynamicTableObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.DynamicTableShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_DT",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			TargetLag:    "1 minute",
			Warehouse:    "MY_WH",
			RefreshMode:  "AUTO",
			Text:         "SELECT * FROM src",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.DynamicTable{}, &snowplanev1alpha1.ProviderConfig{})
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("DynamicTable")

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
			return newTestDynamicTable(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.DynamicTable{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create terminal error
// --------------------------------------------------------------------------

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
			return &snowflake.DynamicTableObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDynamicTableOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.ObservedGeneration = 1
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.DynamicTable{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mydt", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdateComment(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.ObservedGeneration = 1
	dt.Generation = 2
	dt.Spec.Comment = testutil.Ptr("new comment")
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterDynamicTableOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDynamicTableOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"
	now := metav1.Now()
	dt.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_DT", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.DynamicTable{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mydt", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"
	dt.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	dt.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: ApplyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	obs := successfulObservation()

	applyObservation(dt, obs)

	assert.NotEmpty(t, dt.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", dt.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", dt.Status.SchemaName)
	assert.Equal(t, "MY_DT", dt.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", dt.Status.ShowOutput.Owner)
	assert.Equal(t, "1 minute", dt.Status.ShowOutput.TargetLag)
	assert.Equal(t, "MY_WH", dt.Status.ShowOutput.Warehouse)
	assert.Equal(t, "AUTO", dt.Status.ShowOutput.RefreshMode)
	assert.Equal(t, "SELECT * FROM src", dt.Status.ShowOutput.Text)
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	dt := newTestDynamicTable("mydt", "default")
	dt.Finalizers = []string{finalizerName}
	dt.Status.DatabaseName = "MY_DB"
	dt.Status.SchemaName = "MY_SCHEMA"
	dt.Status.WarehouseName = "MY_WH"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
				call++
				if call == 1 {
					return &snowflake.DynamicTableObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateDynamicTableOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, dt, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mydt", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
