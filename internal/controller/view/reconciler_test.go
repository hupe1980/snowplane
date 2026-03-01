package view

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateViewOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterViewOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.ViewObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateViewOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterViewOptions) error {
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

func newTestView(name, namespace string) *snowplanev1alpha1.View {
	return &snowplanev1alpha1.View{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.ViewSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "ACTIVE_USERS",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "analytics-db"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "public-schema"},
			Statement:   "SELECT * FROM users WHERE active = TRUE",
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

func newTestSchema(name, namespace string) *snowplanev1alpha1.Schema {
	s := &snowplanev1alpha1.Schema{
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

func successfulObservation() *snowflake.ViewObservation {
	return &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			CreatedOn:      "2024-01-01",
			Name:           "ACTIVE_USERS",
			DatabaseName:   "ANALYTICS",
			SchemaName:     "PUBLIC",
			Comment:        "",
			Owner:          "SYSADMIN",
			IsSecure:       false,
			Text:           "SELECT * FROM users WHERE active = TRUE",
			ChangeTracking: false,
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.View{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("View"),
	}
}

func newTestReconcilerWithIndex(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.View{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		).
		WithIndex(&snowplanev1alpha1.View{}, ".spec.databaseRef.name", func(o client.Object) []string {
			v, ok := o.(*snowplanev1alpha1.View)
			if !ok {
				return nil
			}

			return []string{v.Spec.DatabaseRef.Name}
		}).
		WithIndex(&snowplanev1alpha1.View{}, ".spec.schemaRef.name", func(o client.Object) []string {
			v, ok := o.(*snowplanev1alpha1.View)
			if !ok {
				return nil
			}

			return []string{v.Spec.SchemaRef.Name}
		})

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.View, Service, *snowflake.ViewObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("View"),
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
			return newTestView(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.View{}
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

	view := newTestView("myview", "default")
	r := newTestReconciler(&mockService{}, view)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)

	got := &snowplanev1alpha1.View{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myview", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_SchemaRefNotFound(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	db := newTestDB("analytics-db", "default")
	r := newTestReconciler(&mockService{}, view, db)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateView(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateViewOptions

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ViewObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateViewOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, view, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.DatabaseName())
	assert.Equal(t, "PUBLIC", capturedOpts.Name.SchemaName())
	assert.Equal(t, "ACTIVE_USERS", capturedOpts.Name.Name())
	assert.Equal(t, "SELECT * FROM users WHERE active = TRUE", capturedOpts.Statement)
	assert.False(t, capturedOpts.Secure)

	got := &snowplanev1alpha1.View{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myview", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.Equal(t, "ANALYTICS", got.Status.DatabaseName)
	assert.Equal(t, "PUBLIC", got.Status.SchemaName)
}

func TestReconcile_CreateSecureView(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Finalizers = []string{finalizerName}
	view.Spec.Secure = true
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateViewOptions

	obs := successfulObservation()
	obs.ShowOutput.IsSecure = true
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
				call++
				if call == 1 {
					return &snowflake.ViewObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateViewOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, view, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.Secure)
}

func TestReconcile_CreateError(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")
	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateViewOptions) error {
			return fmt.Errorf("create failed")
		},
	}

	r := newTestReconciler(mock, view, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteView(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	view := newTestView("myview", "default")
	view.Finalizers = []string{finalizerName}
	view.DeletionTimestamp = &now
	view.Status.DatabaseName = `"ANALYTICS"`
	view.Status.SchemaName = `"ANALYTICS"."PUBLIC"`
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var dropped bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.ViewObservation, error) {
			return &snowflake.ViewObservation{Exists: true}, nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, view, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myview", "default"))
	require.NoError(t, err)
	assert.True(t, dropped)
}

// --------------------------------------------------------------------------
// Tests: Immutability checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_NameChanged(t *testing.T) {
	t.Parallel()

	a := &adapter{}
	view := newTestView("myview", "default")
	view.Status.ObservedGeneration = 1
	view.Status.ShowOutput = &snowplanev1alpha1.ViewShowOutput{
		Name:         "ACTIVE_USERS",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
	}
	view.Spec.Name = "INACTIVE_USERS"
	err := a.ValidateImmutableFields(context.Background(), view)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	a := &adapter{}
	view := newTestView("myview", "default")
	view.Status.ObservedGeneration = 1
	view.Status.ShowOutput = &snowplanev1alpha1.ViewShowOutput{Name: "ACTIVE_USERS"}
	view.Annotations = map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"}
	view.Spec.Name = "DIFFERENT"
	err := a.ValidateImmutableFields(context.Background(), view)
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Build helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Comment = testutil.PtrString("test view")
	view.Spec.Secure = true
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "ACTIVE_USERS")

	opts := buildCreateOptions(view, id)
	assert.Equal(t, "ACTIVE_USERS", opts.Name.Name())
	assert.Equal(t, "SELECT * FROM users WHERE active = TRUE", opts.Statement)
	assert.True(t, opts.Secure)
	assert.Equal(t, "test view", *opts.Comment)
}

func TestBuildAlterOptions_SecureToggle(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Secure = true
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "ACTIVE_USERS")
	obs := successfulObservation()

	opts := buildAlterOptions(view, id, obs)
	require.NotNil(t, opts.Secure)
	assert.True(t, *opts.Secure)
	assert.Nil(t, opts.ReplaceStatement, "no statement change → ReplaceStatement should be nil")
}

func TestBuildAlterOptions_StatementChange(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Statement = "SELECT id, name FROM users WHERE active = TRUE"
	view.Spec.Secure = true
	view.Spec.Comment = testutil.PtrString("updated comment")
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "ACTIVE_USERS")
	obs := successfulObservation() // Text = "SELECT * FROM users WHERE active = TRUE"

	opts := buildAlterOptions(view, id, obs)
	require.NotNil(t, opts.ReplaceStatement, "statement changed → ReplaceStatement must be set")
	assert.Equal(t, "SELECT id, name FROM users WHERE active = TRUE", opts.ReplaceStatement.Statement)
	assert.True(t, opts.ReplaceStatement.Secure)
	assert.Equal(t, "updated comment", *opts.ReplaceStatement.Comment)
	assert.True(t, opts.HasChanges())
	// Other ALTER fields should be nil — they're carried by CREATE OR REPLACE.
	assert.Nil(t, opts.Secure)
	assert.Nil(t, opts.Comment)
	assert.Nil(t, opts.ChangeTracking)
}

func TestBuildAlterOptions_StatementUnchanged(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	// Statement matches obs.ShowOutput.Text exactly.
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "ACTIVE_USERS")
	obs := successfulObservation()

	opts := buildAlterOptions(view, id, obs)
	assert.Nil(t, opts.ReplaceStatement)
	assert.False(t, opts.HasChanges(), "nothing changed")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.ViewSpec{
		Comment:        testutil.PtrString("test"),
		ChangeTracking: testutil.PtrBool(true),
	}

	fields := tracked.ComputeTracked(spec)
	assert.Contains(t, fields, "COMMENT")
	assert.Contains(t, fields, "CHANGE_TRACKING")
}

func TestDetectDrift_CommentDrift(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Comment = testutil.PtrString("expected")

	obs := &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			Comment: "actual",
		},
	}

	result := detectDrift(view, obs)
	assert.True(t, result.HasDrift)
}

func TestDetectDrift_StatementDrift(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Statement = "SELECT * FROM users WHERE active = TRUE"

	obs := &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			Text: "SELECT * FROM old_users", // externally changed
		},
	}

	result := detectDrift(view, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "STATEMENT")
}

func TestDetectDrift_StatementMatch(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	view.Spec.Statement = "SELECT * FROM users WHERE active = TRUE"

	obs := &snowflake.ViewObservation{
		Exists: true,
		ShowOutput: &snowflake.ViewShowOutput{
			Text: "SELECT * FROM users WHERE active = TRUE",
		},
	}

	result := detectDrift(view, obs)
	assert.False(t, result.HasDrift)
}

// --------------------------------------------------------------------------
// Tests: Watch mapping
// --------------------------------------------------------------------------

func TestMapDatabaseToViews(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, view, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.databaseRef.name", "listing views for database watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), db)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "myview", reqs[0].Name)
}

func TestMapSchemaToViews(t *testing.T) {
	t.Parallel()

	view := newTestView("myview", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, view, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.ViewList{} }, ".spec.schemaRef.name", "listing views for schema watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), schema)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "myview", reqs[0].Name)
}
