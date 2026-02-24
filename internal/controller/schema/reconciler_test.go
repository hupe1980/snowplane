package schema

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSchemaOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSchemaOptions) error
	dropFn    func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.SchemaObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSchemaOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSchemaOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name)
	}
	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestSchema(name, namespace string) *snowplanev1alpha1.Schema {
	return &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "PUBLIC",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "analytics-db"},
		},
	}
}

func newTestDB(name, namespace string) *snowplanev1alpha1.Database {
	db := &snowplanev1alpha1.Database{
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
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"ANALYTICS"`,
				ObservedGeneration: 1,
			},
		},
	}
	conditions.SetReady(db, "ok")
	return db
}

func successfulObservation() *snowflake.SchemaObservation {
	return &snowflake.SchemaObservation{
		Exists: true,
		ShowOutput: &snowflake.SchemaShowOutput{
			CreatedOn:     "2024-01-01",
			Name:          "PUBLIC",
			DatabaseName:  "ANALYTICS",
			Kind:          "STANDARD",
			Comment:       "",
			Owner:         "SYSADMIN",
			RetentionTime: 1,
		},
		Parameters: &snowflake.SchemaParameters{
			DataRetentionTimeInDays:    testutil.PtrInt32(1),
			MaxDataExtensionTimeInDays: testutil.PtrInt32(14),
			DefaultDDLCollation:        "",
			ReplaceInvalidCharacters:   testutil.PtrBool(false),
			StorageSerializationPolicy: "COMPATIBLE",
			LogLevel:                   "OFF",
			MetricLevel:                "NONE",
			TraceLevel:                 "OFF",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.ProviderConfig{},
		)
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Schema"),
	}
}

// newTestReconcilerWithIndex creates a reconciler with the field indexer on
// .spec.databaseRef.name so MapByFieldIndex can be tested.
func newTestReconcilerWithIndex(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.ProviderConfig{},
		).
		WithIndex(&snowplanev1alpha1.Schema{}, ".spec.databaseRef.name", func(o client.Object) []string {
			sch, ok := o.(*snowplanev1alpha1.Schema)
			if !ok {
				return nil
			}
			return []string{sch.Spec.DatabaseRef.Name}
		})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Schema"),
	}
}

// --------------------------------------------------------------------------
// Tests: CR not found
// --------------------------------------------------------------------------

func TestReconcile_CRNotFound(t *testing.T) {
	t.Parallel()
	r := newTestReconciler(&mockService{})
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("gone", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --------------------------------------------------------------------------
// Tests: Database reference resolution
// --------------------------------------------------------------------------

func TestReconcile_DatabaseRefNotFound(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	r := newTestReconciler(&mockService{}, schema)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
}

func TestReconcile_DatabaseRefNotReady(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	db.Status.Conditions = nil
	r := newTestReconciler(&mockService{}, schema, db)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
}

func TestReconcile_DatabaseRefEmptyFQN(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	db.Status.FullyQualifiedName = ""
	conditions.SetReady(db, "ok")
	r := newTestReconciler(&mockService{}, schema, db)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
}

// --------------------------------------------------------------------------
// Tests: ProviderConfig resolution
// --------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	r := newTestReconciler(&mockService{}, schema, db)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

func TestReconcile_ProviderConfigNotReady(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	pc := testutil.NewTestPC("default")
	pc.Status.Conditions = nil
	r := newTestReconciler(&mockService{}, schema, db, pc, testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProviderConfig not ready")
}

func TestReconcile_SecretNotFound(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	pc := testutil.NewTestPC("default")
	r := newTestReconciler(&mockService{}, schema, db, pc)
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching secret")
}

// --------------------------------------------------------------------------
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	db := newTestDB("analytics-db", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateSchema(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	var capturedOpts snowflake.CreateSchemaOptions
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSchemaOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.DatabaseName())
	assert.Equal(t, "PUBLIC", capturedOpts.Name.Name())
	assert.False(t, capturedOpts.Transient)
	assert.False(t, capturedOpts.ManagedAccess)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.Equal(t, "ANALYTICS", got.Status.DatabaseName)
}

func TestReconcile_CreateTransientSchema(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.Transient = true
	db := newTestDB("analytics-db", "default")
	var capturedOpts snowflake.CreateSchemaOptions
	obs := successfulObservation()
	obs.ShowOutput.Kind = "TRANSIENT"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSchemaOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.Transient)
}

func TestReconcile_CreateManagedAccessSchema(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.ManagedAccess = true
	db := newTestDB("analytics-db", "default")
	var capturedOpts snowflake.CreateSchemaOptions
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil
				}
				return successfulObservation(), nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateSchemaOptions) error {
			capturedOpts = opts
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.ManagedAccess)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return &snowflake.SchemaObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return fmt.Errorf("permission denied")
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return &snowflake.SchemaObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("schema name invalid"))
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Update / drift correction
// --------------------------------------------------------------------------

func TestReconcile_UpdateSchema(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	schema.Status.ObservedGeneration = 1
	schema.Spec.Comment = testutil.PtrString("updated comment")
	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	var capturedAlter snowflake.AlterSchemaOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSchemaOptions) error {
			capturedAlter = opts
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	require.NotNil(t, capturedAlter.Comment)
	assert.Equal(t, "updated comment", *capturedAlter.Comment)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_NoChangeNoAlter(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Status.ObservedGeneration = 1
	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	alterCalled := false
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			alterCalled = true
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "alter should not be called when there are no changes")
}

func TestReconcile_AlterFails(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	schema.Status.ObservedGeneration = 1
	schema.Spec.Comment = testutil.PtrString("boom")
	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			return fmt.Errorf("alter denied")
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alter denied")
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteSchema(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now
	db := newTestDB("analytics-db", "default")
	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, id snowflake.DatabaseObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "ANALYTICS", id.DatabaseName())
			assert.Equal(t, "PUBLIC", id.Name())
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)
	// After removing the finalizer with DeletionTimestamp set, the fake client
	// deletes the object — verify it's gone.
	got := &snowplanev1alpha1.Schema{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got)
	assert.True(t, err != nil, "schema should be deleted after finalizer removal")
}

func TestReconcile_DeleteOrphan(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	schema.DeletionTimestamp = &now
	db := newTestDB("analytics-db", "default")
	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled, "should not drop when policy is Orphan")
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now
	db := newTestDB("analytics-db", "default")
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable field validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableTransient(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.Transient = true
	schema.Status.ObservedGeneration = 1
	schema.Status.ShowOutput = &snowplanev1alpha1.SchemaShowOutput{
		Kind: "STANDARD",
	}
	db := newTestDB("analytics-db", "default")
	mock := &mockService{}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: buildCreateOptions
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()
	ssp := snowplanev1alpha1.StorageSerializationPolicyOptimized
	ll := snowplanev1alpha1.LogLevelInfo
	ml := snowplanev1alpha1.MetricLevelAll
	tl := snowplanev1alpha1.TraceLevelAlways
	schema := newTestSchema("myschema", "default")
	schema.Spec.Transient = true
	schema.Spec.ManagedAccess = true
	schema.Spec.Comment = testutil.PtrString("test")
	schema.Spec.DataRetentionTimeInDays = testutil.PtrInt32(7)
	schema.Spec.MaxDataExtensionTimeInDays = testutil.PtrInt32(28)
	schema.Spec.ReplaceInvalidCharacters = testutil.PtrBool(true)
	schema.Spec.DefaultDDLCollation = testutil.PtrString("en-ci")
	schema.Spec.StorageSerializationPolicy = &ssp
	schema.Spec.LogLevel = &ll
	schema.Spec.MetricLevel = &ml
	schema.Spec.TraceLevel = &tl
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	opts := buildCreateOptions(schema, id)
	assert.True(t, opts.Transient)
	assert.True(t, opts.ManagedAccess)
	assert.Equal(t, "test", *opts.Comment)
	assert.Equal(t, int32(7), *opts.DataRetentionTimeInDays)
	assert.Equal(t, int32(28), *opts.MaxDataExtensionTimeInDays)
	assert.Equal(t, true, *opts.ReplaceInvalidCharacters)
	assert.Equal(t, "en-ci", *opts.DefaultDDLCollation)
	assert.Equal(t, "OPTIMIZED", *opts.StorageSerializationPolicy)
	assert.Equal(t, "INFO", *opts.LogLevel)
	assert.Equal(t, "ALL", *opts.MetricLevel)
	assert.Equal(t, "ALWAYS", *opts.TraceLevel)
}

// --------------------------------------------------------------------------
// Tests: buildAlterOptions
// --------------------------------------------------------------------------

func TestBuildAlterOptions_NoDiff(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	obs := successfulObservation()
	opts := buildAlterOptions(schema, id, obs)
	assert.False(t, opts.HasChanges())
}

func TestBuildAlterOptions_CommentDiff(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Spec.Comment = testutil.PtrString("new comment")
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	obs := successfulObservation()
	opts := buildAlterOptions(schema, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new comment", *opts.Comment)
}

func TestBuildAlterOptions_RetentionDiff(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Spec.DataRetentionTimeInDays = testutil.PtrInt32(30)
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	obs := successfulObservation()
	opts := buildAlterOptions(schema, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, int32(30), *opts.DataRetentionTimeInDays)
}

func TestBuildAlterOptions_AllDiffs(t *testing.T) {
	t.Parallel()

	ssp := snowplanev1alpha1.StorageSerializationPolicyOptimized
	ll := snowplanev1alpha1.LogLevelInfo
	ml := snowplanev1alpha1.MetricLevelAll
	tl := snowplanev1alpha1.TraceLevelAlways

	schema := newTestSchema("myschema", "default")
	schema.Spec.Comment = testutil.PtrString("new")
	schema.Spec.DataRetentionTimeInDays = testutil.PtrInt32(30)
	schema.Spec.MaxDataExtensionTimeInDays = testutil.PtrInt32(28)
	schema.Spec.DefaultDDLCollation = testutil.PtrString("en-ci")
	schema.Spec.ReplaceInvalidCharacters = testutil.PtrBool(true)
	schema.Spec.StorageSerializationPolicy = &ssp
	schema.Spec.LogLevel = &ll
	schema.Spec.MetricLevel = &ml
	schema.Spec.TraceLevel = &tl
	schema.Spec.ManagedAccess = true

	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	obs := successfulObservation()

	opts := buildAlterOptions(schema, id, obs)
	assert.True(t, opts.HasChanges())
	assert.Equal(t, "new", *opts.Comment)
	assert.Equal(t, int32(30), *opts.DataRetentionTimeInDays)
	assert.Equal(t, int32(28), *opts.MaxDataExtensionTimeInDays)
	assert.Equal(t, "en-ci", *opts.DefaultDDLCollation)
	assert.Equal(t, true, *opts.ReplaceInvalidCharacters)
	assert.Equal(t, "OPTIMIZED", *opts.StorageSerializationPolicy)
	assert.Equal(t, "INFO", *opts.LogLevel)
	assert.Equal(t, "ALL", *opts.MetricLevel)
	assert.Equal(t, "ALWAYS", *opts.TraceLevel)
	assert.NotNil(t, opts.SetManagedAccess)
	assert.True(t, *opts.SetManagedAccess)
}

func TestBuildAlterOptions_ManagedAccessDisableDiff(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Spec.ManagedAccess = false

	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	obs := successfulObservation()
	obs.ShowOutput.Options = "MANAGED ACCESS"

	opts := buildAlterOptions(schema, id, obs)
	assert.True(t, opts.HasChanges())
	assert.NotNil(t, opts.SetManagedAccess)
	assert.False(t, *opts.SetManagedAccess)
}

// --------------------------------------------------------------------------
// Tests: Observe error
// --------------------------------------------------------------------------

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection timeout")
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: References resolved condition
// --------------------------------------------------------------------------

func TestReconcile_ReferencesResolvedCondition(t *testing.T) {
	t.Parallel()
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Status.ObservedGeneration = 1
	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
	}
	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
	c := conditions.Get(got, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, c)
	assert.Contains(t, c.Message, "ANALYTICS")
}

// --------------------------------------------------------------------------
// Tests: UNSET support (C-2)
// --------------------------------------------------------------------------

func TestBuildAlterOptions_UnsetDetection(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Status.TrackedParameters = []string{"COMMENT", "LOG_LEVEL"}
	// Spec has no comment, no log level — both should be unset.

	obs := successfulObservation()
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")

	opts := buildAlterOptions(schema, id, obs)
	assert.ElementsMatch(t, []string{"COMMENT", "LOG_LEVEL"}, opts.UnsetFields)
}

func TestBuildAlterOptions_NoUnsetWhenFieldStillSet(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Spec.Comment = testutil.PtrString("still here")
	schema.Status.TrackedParameters = []string{"COMMENT"}

	obs := successfulObservation()
	id := snowflake.NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")

	opts := buildAlterOptions(schema, id, obs)
	assert.Empty(t, opts.UnsetFields)
}

func TestComputeSchemaTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.SchemaSpec{
		Comment:                 testutil.PtrString("x"),
		DataRetentionTimeInDays: testutil.PtrInt32(7),
	}

	fields := computeTrackedParameters(spec)
	assert.ElementsMatch(t, []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"}, fields)
}

func TestReconcile_TrackedParametersPersistedOnCreate(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.Comment = testutil.PtrString("hello")
	schema.Spec.DataRetentionTimeInDays = testutil.PtrInt32(7)

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "hello"
	obs.Parameters.DataRetentionTimeInDays = testutil.PtrInt32(7)

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.ElementsMatch(t, []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"}, got.Status.TrackedParameters)
}

func TestReconcile_UnsetTriggered(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	schema.Generation = 2
	schema.Status.ObservedGeneration = 1
	schema.Status.TrackedParameters = []string{"COMMENT"}

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var capturedAlterOpts snowflake.AlterSchemaOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSchemaOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	assert.Contains(t, capturedAlterOpts.UnsetFields, "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Recoverable condition (H-4)
// --------------------------------------------------------------------------

func TestReconcile_RecoverableConditionOnTransientError(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

func TestReconcile_RecoverableClearedOnSuccess(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Status.ObservedGeneration = 1
	conditions.SetNotReady(schema, snowplanev1alpha1.ReasonReconcileError, "previous error")

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: Deletion with missing Database CR (Bug #1 fix)
// --------------------------------------------------------------------------

func TestReconcile_DeleteUnblockedWhenDatabaseCRDeleted(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now
	// Status has cached database name from a previous successful reconciliation.
	schema.Status.DatabaseName = "ANALYTICS"

	// No Database CR exists — reference resolution will fail with ErrReferenceNotFound.
	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, id snowflake.DatabaseObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "ANALYTICS", id.DatabaseName())
			assert.Equal(t, "PUBLIC", id.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled, "should drop using cached database name")
}

// --------------------------------------------------------------------------
// Tests: Deletion with missing ProviderConfig (Bug #2 fix)
// --------------------------------------------------------------------------

func TestReconcile_DeleteUnblockedWhenProviderConfigMissing(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now
	schema.Status.DatabaseName = "ANALYTICS"

	db := newTestDB("analytics-db", "default")

	// No ProviderConfig or Secret — provider resolution will fail.
	mock := &mockService{}
	r := newTestReconciler(mock, schema, db)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Finalizer should be removed.
	got := &snowplanev1alpha1.Schema{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got)
	assert.True(t, err != nil, "finalizer should be removed when PC is missing during deletion")
}

// --------------------------------------------------------------------------
// Tests: Immutable name and databaseRef validation (Bug #4 fix)
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.Name = "NEW_SCHEMA"
	schema.Status.ObservedGeneration = 1
	schema.Status.ShowOutput = &snowplanev1alpha1.SchemaShowOutput{
		Name:         "OLD_SCHEMA",
		DatabaseName: "ANALYTICS",
		Kind:         "STANDARD",
	}

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Drop sets Recoverable condition
// --------------------------------------------------------------------------

func TestReconcile_DeleteDropFailsSetsRecoverable(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now

	db := newTestDB("analytics-db", "default")
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
			return fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsRecoverable(got))
}

// --------------------------------------------------------------------------
// Tests: UNSET computed before nil-params guard
// --------------------------------------------------------------------------

// --------------------------------------------------------------------------
// Tests: Alter terminal error
// --------------------------------------------------------------------------

func TestReconcile_AlterTerminalError(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	schema.Status.ObservedGeneration = 1
	schema.Spec.Comment = testutil.PtrString("bad")

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("terminal: bad syntax"))
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)
	assert.True(t, snowflake.IsTerminalError(err))

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Post-create observe error
// --------------------------------------------------------------------------

func TestReconcile_CreatePostObserveError(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}

	db := newTestDB("analytics-db", "default")

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil // first: not found
				}

				return nil, fmt.Errorf("observe timeout") // second: post-create fails
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err) // should NOT propagate — short requeue instead
	assert.Equal(t, 5*time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

// --------------------------------------------------------------------------
// Tests: Drift correction (same generation, observed differs from spec)
// --------------------------------------------------------------------------

func TestReconcile_DriftCorrection(t *testing.T) {
	t.Parallel()

	// ObservedGeneration == Generation, but observed state differs → drift.
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	schema.Generation = 1
	schema.Status.ObservedGeneration = 1 // same generation → drift path
	schema.Spec.Comment = testutil.PtrString("desired comment")

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted comment" // different from desired

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterSchemaOptions) error {
			alterCalled = true
			assert.Equal(t, "desired comment", *opts.Comment)
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled, "Alter should be called for drift correction")
}

// --------------------------------------------------------------------------
// Tests: Immutable databaseRef validation
// --------------------------------------------------------------------------

func TestReconcile_ImmutableDatabaseRef(t *testing.T) {
	t.Parallel()

	// Schema was originally created in database ANALYTICS, but spec now points
	// to a different Database CR that resolves to DIFFERENT_DB.
	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Spec.DatabaseRef = &snowplanev1alpha1.LocalObjectReference{Name: "other-db"}
	schema.Status.ObservedGeneration = 1
	schema.Status.ShowOutput = &snowplanev1alpha1.SchemaShowOutput{
		Name:         "PUBLIC",
		DatabaseName: "ANALYTICS", // originally created in ANALYTICS
		Kind:         "STANDARD",
	}

	// Create a different Database CR with a different FQN.
	otherDB := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "other-db",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "DIFFERENT_DB",
		},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"DIFFERENT_DB"`,
				ObservedGeneration: 1,
			},
		},
	}
	conditions.SetReady(otherDB, "ok")

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, schema, otherDB, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "immutable violation should not requeue")

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.True(t, conditions.IsTerminal(got))
}

// --------------------------------------------------------------------------
// Tests: Event emission verification
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
				call++
				if call == 1 {
					return &snowflake.SchemaObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
	assert.Contains(t, events[0], "created")
}

func TestReconcile_EventEmission_Update(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Generation = 2
	schema.Spec.Comment = testutil.PtrString("new comment")
	schema.Status.ObservedGeneration = 1

	db := newTestDB("analytics-db", "default")
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "ReconcileSuccess")
}

func TestReconcile_EventEmission_Delete(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	now := metav1.Now()
	schema.DeletionTimestamp = &now

	db := newTestDB("analytics-db", "default")

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
			return nil
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Deleting")
}

func TestReconcile_EventEmission_CreateFails(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}

	db := newTestDB("analytics-db", "default")

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return &snowflake.SchemaObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateSchemaOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.Error(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Warning")
	assert.Contains(t, events[0], "ReconcileError")
}

// --------------------------------------------------------------------------
// Tests: UNSET computed before nil-params guard
// --------------------------------------------------------------------------

func TestBuildAlterOptions_UnsetComputedWhenParamsNil(t *testing.T) {
	t.Parallel()

	schema := &snowplanev1alpha1.Schema{
		Spec: snowplanev1alpha1.SchemaSpec{Name: "S"},
		Status: snowplanev1alpha1.SchemaStatus{
			TrackedParameters: []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"},
		},
	}

	id := snowflake.NewDatabaseObjectIdentifier("DB", "S")
	obs := &snowflake.SchemaObservation{
		Exists:     true,
		ShowOutput: &snowflake.SchemaShowOutput{Name: "S", DatabaseName: "DB"},
		Parameters: nil,
	}

	opts := buildAlterOptions(schema, id, obs)
	assert.Contains(t, opts.UnsetFields, "COMMENT")
	assert.Contains(t, opts.UnsetFields, "DATA_RETENTION_TIME_IN_DAYS")
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: mapDatabaseToSchemas with field indexer
// --------------------------------------------------------------------------

func TestMapDatabaseToSchemas_FiltersCorrectly(t *testing.T) {
	t.Parallel()

	// newTestSchema default: DatabaseRef.Name = "analytics-db"
	s1 := newTestSchema("schema-a", "default")

	s2 := newTestSchema("schema-b", "default")
	s2.Spec.DatabaseRef.Name = "other-db"

	r := newTestReconcilerWithIndex(&mockService{}, s1, s2)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.SchemaList{} }, ".spec.databaseRef.name", "listing schemas for database watch")

	// Trigger with analytics-db — only schema-a should match.
	db := newTestDB("analytics-db", "default")
	requests := mapFunc(context.Background(), db)

	require.Len(t, requests, 1)
	assert.Equal(t, "schema-a", requests[0].Name)
	assert.Equal(t, "default", requests[0].Namespace)
}

func TestMapDatabaseToSchemas_MultipleMatches(t *testing.T) {
	t.Parallel()

	// Both schemas reference the default "analytics-db".
	s1 := newTestSchema("schema-a", "default")
	s2 := newTestSchema("schema-b", "default")

	r := newTestReconcilerWithIndex(&mockService{}, s1, s2)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.SchemaList{} }, ".spec.databaseRef.name", "listing schemas for database watch")

	db := newTestDB("analytics-db", "default")
	requests := mapFunc(context.Background(), db)

	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "schema-a")
	assert.Contains(t, names, "schema-b")
}

func TestMapDatabaseToSchemas_NoMatch(t *testing.T) {
	t.Parallel()

	// newTestSchema default: DatabaseRef.Name = "analytics-db"
	s1 := newTestSchema("schema-a", "default")

	r := newTestReconcilerWithIndex(&mockService{}, s1)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.SchemaList{} }, ".spec.databaseRef.name", "listing schemas for database watch")

	db := newTestDB("unrelated-db", "default")
	requests := mapFunc(context.Background(), db)

	assert.Empty(t, requests)
}

// --------------------------------------------------------------------------
// Tests: Drift detection — DriftDetected condition & events
// --------------------------------------------------------------------------

func TestReconcile_DriftCorrection_SetsDriftDetectedCondition(t *testing.T) {
	t.Parallel()

	s := newTestSchema("myschema", "default")
	s.Finalizers = []string{finalizerName}
	s.Annotations = map[string]string{snowplanev1alpha1.AnnotationUseCreateOrAlter: "false"}
	s.Generation = 1
	s.Status.ObservedGeneration = 1 // drift path
	s.Status.DatabaseName = `"ANALYTICS"`
	s.Spec.Comment = testutil.PtrString("desired")
	hash, err := snowplanev1alpha1.ComputeSpecHash(s.Spec)
	require.NoError(t, err)
	s.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted"

	var alterCalled bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, s, newTestDB("analytics-db", "default"), testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.True(t, alterCalled)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	// After successful correction, DriftDetected should be cleared.
	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeDriftDetected),
		"DriftDetected condition should be cleared after successful correction")

	// Check events.
	recorder := r.Recorder.(*record.FakeRecorder)
	var events []string
	for len(recorder.Events) > 0 {
		events = append(events, <-recorder.Events)
	}

	assert.True(t, testutil.ContainsEvent(events, "DriftDetected"), "expected DriftDetected event, got: %v", events)
	assert.True(t, testutil.ContainsEvent(events, "DriftCorrected"), "expected DriftCorrected event, got: %v", events)
}

func TestReconcile_DriftDetectOnlyPolicy(t *testing.T) {
	t.Parallel()

	s := newTestSchema("myschema", "default")
	s.Finalizers = []string{finalizerName}
	s.Generation = 1
	s.Status.ObservedGeneration = 1 // drift path
	s.Status.DatabaseName = `"ANALYTICS"`
	s.Annotations = map[string]string{
		"snowplane.hupe1980.github.io/drift-policy": "detect-only",
	}
	s.Spec.Comment = testutil.PtrString("desired")
	hash, err := snowplanev1alpha1.ComputeSpecHash(s.Spec)
	require.NoError(t, err)
	s.Status.LastAppliedSpecHash = hash

	obs := successfulObservation()
	obs.ShowOutput.Comment = "drifted"

	alterCalled := false

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, _ snowflake.AlterSchemaOptions) error {
			alterCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, s, newTestDB("analytics-db", "default"), testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)
	assert.False(t, alterCalled, "Alter should NOT be called with detect-only policy")
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.Schema{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myschema", Namespace: "default"}, got))

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

func TestDetectSchemaDrift_NoDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.Schema{
		Spec: snowplanev1alpha1.SchemaSpec{
			Comment:                 testutil.PtrString("test"),
			DataRetentionTimeInDays: testutil.PtrInt32(1),
		},
	}

	obs := &snowflake.SchemaObservation{
		ShowOutput: &snowflake.SchemaShowOutput{Comment: "test"},
		Parameters: &snowflake.SchemaParameters{
			DataRetentionTimeInDays: testutil.PtrInt32(1),
		},
	}

	result := detectDrift(s, obs)
	assert.False(t, result.HasDrift)
	assert.Empty(t, result.Changes)
}

func TestDetectSchemaDrift_WithDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.Schema{
		Spec: snowplanev1alpha1.SchemaSpec{
			Comment:                 testutil.PtrString("desired"),
			DataRetentionTimeInDays: testutil.PtrInt32(30),
		},
	}

	obs := &snowflake.SchemaObservation{
		ShowOutput: &snowflake.SchemaShowOutput{Comment: "drifted"},
		Parameters: &snowflake.SchemaParameters{
			DataRetentionTimeInDays: testutil.PtrInt32(1),
		},
	}

	result := detectDrift(s, obs)
	assert.True(t, result.HasDrift)
	assert.Len(t, result.Changes, 2)
	assert.Contains(t, result.Summary(), "COMMENT")
	assert.Contains(t, result.Summary(), "DATA_RETENTION_TIME_IN_DAYS")
}

// --------------------------------------------------------------------------
// Tests: Ownership (USE ROLE)
// --------------------------------------------------------------------------

func TestReconcile_UseRole_PassedToServiceFactory(t *testing.T) {
	t.Parallel()

	schema := newTestSchema("myschema", "default")
	schema.Finalizers = []string{finalizerName}
	schema.Generation = 1
	schema.Status.ObservedGeneration = 1
	schema.Status.DatabaseName = "ANALYTICS"
	schema.Spec.UseRole = testutil.PtrString("DATA_ADMIN")

	db := newTestDB("analytics-db", "default")
	db.Status.FullyQualifiedName = "ANALYTICS"
	db.Status.Conditions = []metav1.Condition{{
		Type: snowplanev1alpha1.TypeReady, Status: metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(), Reason: "Ready",
	}}

	obs := successfulObservation()
	obs.ShowOutput.Owner = "DATA_ADMIN"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.SchemaObservation, error) {
			return obs, nil
		},
	}

	var capturedUseRole string

	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Schema{}, &snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(schema, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default")).
		Build()

	rec := record.NewFakeRecorder(100)

	r := &reconciler.GenericReconciler[*snowplanev1alpha1.Schema, Service, *snowflake.SchemaObservation]{
		Client:   c,
		Factory:  clientfactory.NewClientFactory(),
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, useRole string) (Service, func(context.Context), error) {
				capturedUseRole = useRole
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Schema"),
	}

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myschema", "default"))
	require.NoError(t, err)

	assert.Equal(t, "DATA_ADMIN", capturedUseRole, "useRole from spec should be passed to ServiceFactory")
}
