package materializedview

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/tracked"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.MaterializedViewObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateMaterializedViewOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterMaterializedViewOptions) error {
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

func newTestMaterializedView(name, namespace string) *snowplanev1alpha1.MaterializedView {
	return &snowplanev1alpha1.MaterializedView{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.MaterializedViewSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "DAILY_SALES",
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: "public-schema"},
			Statement:   "SELECT sale_date, SUM(amount) AS total FROM sales GROUP BY sale_date",
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
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
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

func newTestSchema(name, namespace string) *snowplanev1alpha1.Schema {
	s := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "PUBLIC",
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
		},
		Status: snowplanev1alpha1.SchemaStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"ANALYTICS"."PUBLIC"`,
				ObservedGeneration: 1,
			},
			DatabaseName: `"ANALYTICS"`,
		},
	}
	conditions.SetReady(s, "ok")

	return s
}

func successfulObservation() *snowflake.MaterializedViewObservation {
	return &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			CreatedOn:          "2024-01-01",
			Name:               "DAILY_SALES",
			DatabaseName:       "ANALYTICS",
			SchemaName:         "PUBLIC",
			Comment:            "",
			Owner:              "SYSADMIN",
			IsSecure:           "false",
			Text:               "SELECT sale_date, SUM(amount) AS total FROM sales GROUP BY sale_date",
			SourceDatabaseName: "ANALYTICS",
			SourceSchemaName:   "PUBLIC",
			SourceTableName:    "SALES",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.MaterializedView{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	r := NewReconcilerWithServiceFactory(c, factory, rec, nil,
		func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("MaterializedView")

	return r
}

func newTestReconcilerWithIndex(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.MaterializedView, Service, *snowflake.MaterializedViewObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.MaterializedView{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		).
		WithIndex(&snowplanev1alpha1.MaterializedView{}, ".spec.databaseRef.name", func(o client.Object) []string {
			mv, ok := o.(*snowplanev1alpha1.MaterializedView)
			if !ok || mv.Spec.DatabaseRef == nil {
				return nil
			}

			return []string{mv.Spec.DatabaseRef.Name}
		}).
		WithIndex(&snowplanev1alpha1.MaterializedView{}, ".spec.schemaRef.name", func(o client.Object) []string {
			mv, ok := o.(*snowplanev1alpha1.MaterializedView)
			if !ok || mv.Spec.SchemaRef == nil {
				return nil
			}

			return []string{mv.Spec.SchemaRef.Name}
		})

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	r := NewReconcilerWithServiceFactory(c, factory, rec, nil,
		func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("MaterializedView")

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
			return newTestMaterializedView(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.MaterializedView{}
		},
		FinalizerName: finalizerName,
		PrereqObjects: func() []runtime.Object {
			db := newTestDB("analytics-db", "default")
			sch := newTestSchema("public-schema", "default")
			return []runtime.Object{db, sch}
		},
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Reference resolution
// --------------------------------------------------------------------------

func TestReconcile_DatabaseRefNotFound(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	r := newTestReconciler(&mockService{}, mv)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)

	got := &snowplanev1alpha1.MaterializedView{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mymv", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_SchemaRefNotFound(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	db := newTestDB("analytics-db", "default")
	r := newTestReconciler(&mockService{}, mv, db)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateMaterializedView(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateMaterializedViewOptions

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
				call++
				if call == 1 {
					return &snowflake.MaterializedViewObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateMaterializedViewOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, mv, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.DatabaseName())
	assert.Equal(t, "PUBLIC", capturedOpts.Name.SchemaName())
	assert.Equal(t, "DAILY_SALES", capturedOpts.Name.Name())
	assert.Equal(t, "SELECT sale_date, SUM(amount) AS total FROM sales GROUP BY sale_date", capturedOpts.Statement)
	assert.False(t, capturedOpts.Secure)

	got := &snowplanev1alpha1.MaterializedView{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mymv", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.Equal(t, "ANALYTICS", got.Status.DatabaseName)
	assert.Equal(t, "PUBLIC", got.Status.SchemaName)
}

func TestReconcile_CreateSecureMaterializedView(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Finalizers = []string{finalizerName}
	mv.Spec.Secure = true
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateMaterializedViewOptions

	obs := successfulObservation()
	obs.ShowOutput.IsSecure = "true"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
				call++
				if call == 1 {
					return &snowflake.MaterializedViewObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateMaterializedViewOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, mv, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.Secure)
}

func TestReconcile_CreateWithClusterBy(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Finalizers = []string{finalizerName}
	mv.Spec.ClusterBy = []string{"sale_date", "region"}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateMaterializedViewOptions

	obs := successfulObservation()
	obs.ShowOutput.ClusterBy = "LINEAR(sale_date, region)"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
				call++
				if call == 1 {
					return &snowflake.MaterializedViewObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateMaterializedViewOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, mv, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.NoError(t, err)
	assert.Equal(t, []string{"sale_date", "region"}, capturedOpts.ClusterBy)
}

func TestReconcile_CreateError(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")
	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateMaterializedViewOptions) error {
			return fmt.Errorf("create failed")
		},
	}

	r := newTestReconciler(mock, mv, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteMaterializedView(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	mv := newTestMaterializedView("mymv", "default")
	mv.Finalizers = []string{finalizerName}
	mv.DeletionTimestamp = &now
	mv.Status.DatabaseName = `"ANALYTICS"`
	mv.Status.SchemaName = `"ANALYTICS"."PUBLIC"`
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var dropped bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
			return &snowflake.MaterializedViewObservation{Exists: true}, nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, mv, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mymv", "default"))
	require.NoError(t, err)
	assert.True(t, dropped)
}

// --------------------------------------------------------------------------
// Tests: Immutability checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_NameChanged(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Status.ObservedGeneration = 1
	mv.Status.ShowOutput = &snowplanev1alpha1.MaterializedViewShowOutput{
		Name:         "DAILY_SALES",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
	}
	mv.Spec.Name = "MONTHLY_SALES"
	err := validateImmutableFields(context.Background(), mv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Status.ObservedGeneration = 1
	mv.Status.ShowOutput = &snowplanev1alpha1.MaterializedViewShowOutput{Name: "DAILY_SALES"}
	mv.Annotations = map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"}
	mv.Spec.Name = "DIFFERENT"
	err := validateImmutableFields(context.Background(), mv)
	require.NoError(t, err)
}

func TestValidateImmutableFields_DatabaseChanged(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Status.ObservedGeneration = 1
	mv.Status.DatabaseName = `"NEWDB"`
	mv.Status.ShowOutput = &snowplanev1alpha1.MaterializedViewShowOutput{
		Name:         "DAILY_SALES",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
	}
	err := validateImmutableFields(context.Background(), mv)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.databaseRef is immutable")
}

// --------------------------------------------------------------------------
// Tests: Build helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Comment = testutil.Ptr("test mv")
	mv.Spec.Secure = true
	mv.Spec.ClusterBy = []string{"sale_date"}
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DAILY_SALES")

	opts := buildCreateOptions(mv, id)
	assert.Equal(t, "DAILY_SALES", opts.Name.Name())
	assert.Equal(t, "SELECT sale_date, SUM(amount) AS total FROM sales GROUP BY sale_date", opts.Statement)
	assert.True(t, opts.Secure)
	assert.Equal(t, "test mv", *opts.Comment)
	assert.Equal(t, []string{"sale_date"}, opts.ClusterBy)
}

func TestBuildAlterOptions_SecureToggle(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Secure = true
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DAILY_SALES")
	obs := successfulObservation() // IsSecure = "false"

	opts := buildAlterOptions(mv, id, obs)
	require.NotNil(t, opts.Secure)
	assert.True(t, *opts.Secure)
}

func TestBuildAlterOptions_SecureUnset(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Secure = false // want UNSET
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DAILY_SALES")
	obs := successfulObservation()
	obs.ShowOutput.IsSecure = "true" // currently secure

	opts := buildAlterOptions(mv, id, obs)
	require.NotNil(t, opts.Secure)
	assert.False(t, *opts.Secure)
}

func TestBuildAlterOptions_CommentChange(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Comment = testutil.Ptr("new comment")
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DAILY_SALES")
	obs := successfulObservation() // Comment = ""

	opts := buildAlterOptions(mv, id, obs)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "new comment", *opts.Comment)
}

func TestBuildAlterOptions_NoChanges(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Secure = false
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DAILY_SALES")
	obs := successfulObservation() // Matches spec exactly

	opts := buildAlterOptions(mv, id, obs)
	assert.False(t, opts.HasChanges(), "nothing changed")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.MaterializedViewSpec{
		Comment: testutil.Ptr("test"),
	}

	fields := tracked.ComputeTracked(spec)
	assert.Contains(t, fields, "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift_CommentDrift(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Comment = testutil.Ptr("expected")

	obs := &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			Comment: "actual",
		},
	}

	result := detectDrift(mv, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestDetectDrift_SecureDrift(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Secure = true

	obs := &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			IsSecure: "false",
		},
	}

	result := detectDrift(mv, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "IS_SECURE")
}

func TestDetectDrift_StatementDrift(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")

	obs := &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			Text: "SELECT * FROM old_sales", // externally changed
		},
	}

	result := detectDrift(mv, obs)
	assert.True(t, result.HasImmutableViolation)
	assert.Contains(t, result.Summary(), "STATEMENT")
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	mv.Spec.Secure = false

	obs := &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			Name:     "DAILY_SALES",
			IsSecure: "false",
			Text:     mv.Spec.Statement,
		},
	}

	result := detectDrift(mv, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_ImmutableNameChanged(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")

	obs := &snowflake.MaterializedViewObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.MaterializedViewShowOutput{
			Name: "DIFFERENT_NAME",
		},
	}

	result := detectDrift(mv, obs)
	assert.True(t, result.HasImmutableViolation)
}

// --------------------------------------------------------------------------
// Tests: Watch mapping
// --------------------------------------------------------------------------

func TestMapDatabaseToMaterializedViews(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, mv, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.MaterializedViewList{} }, ".spec.databaseRef.name", "listing materialized views for database watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), db)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mymv", reqs[0].Name)
}

func TestMapSchemaToMaterializedViews(t *testing.T) {
	t.Parallel()

	mv := newTestMaterializedView("mymv", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, mv, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.MaterializedViewList{} }, ".spec.schemaRef.name", "listing materialized views for schema watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), schema)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mymv", reqs[0].Name)
}

// --------------------------------------------------------------------------
// Tests: SupportsCreateOrAlter
// --------------------------------------------------------------------------

func TestAdapter_SupportsCreateOrAlter(t *testing.T) {
	t.Parallel()

	a := newAdapter(nil, nil, nil)
	assert.False(t, a.SupportsCreateOrAlter(), "materialized views do not support CREATE OR ALTER")
}
