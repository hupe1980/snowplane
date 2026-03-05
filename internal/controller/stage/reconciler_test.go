package stage

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStageOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStageOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.StageObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateStageOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterStageOptions) error {
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

func newTestStage(name, namespace string) *snowplanev1alpha1.Stage {
	return &snowplanev1alpha1.Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.StageSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "DATA_LOAD",
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
			SchemaRef:   &snowplanev1alpha1.ObjectReference{Name: "public-schema"},
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

func successfulObservation() *snowflake.StageObservation {
	return &snowflake.StageObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.StageShowOutput{
			CreatedOn:          "2024-01-01",
			Name:               "DATA_LOAD",
			DatabaseName:       "ANALYTICS",
			SchemaName:         "PUBLIC",
			URL:                "",
			Owner:              "SYSADMIN",
			Comment:            "",
			Type:               "INTERNAL",
			StorageIntegration: "",
			DirectoryEnabled:   false,
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Stage{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("Stage")

	return r
}

func newTestReconcilerWithIndex(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Stage, Service, *snowflake.StageObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Stage{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		).
		WithIndex(&snowplanev1alpha1.Stage{}, ".spec.databaseRef.name", func(o client.Object) []string {
			s, ok := o.(*snowplanev1alpha1.Stage)
			if !ok {
				return nil
			}

			return []string{s.Spec.DatabaseRef.Name}
		}).
		WithIndex(&snowplanev1alpha1.Stage{}, ".spec.schemaRef.name", func(o client.Object) []string {
			s, ok := o.(*snowplanev1alpha1.Stage)
			if !ok {
				return nil
			}

			return []string{s.Spec.SchemaRef.Name}
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("Stage")

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
			return newTestStage(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.Stage{}
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

	stage := newTestStage("mystage", "default")
	r := newTestReconciler(&mockService{}, stage)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)

	got := &snowplanev1alpha1.Stage{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mystage", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_SchemaRefNotFound(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	db := newTestDB("analytics-db", "default")
	r := newTestReconciler(&mockService{}, stage, db)
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.Error(t, err, "should return error for controller-runtime backoff")
	assert.Zero(t, result.RequeueAfter)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateInternalStage(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateStageOptions

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StageObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateStageOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, stage, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.DatabaseName())
	assert.Equal(t, "PUBLIC", capturedOpts.Name.SchemaName())
	assert.Equal(t, "DATA_LOAD", capturedOpts.Name.Name())
	assert.Nil(t, capturedOpts.URL)

	got := &snowplanev1alpha1.Stage{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mystage", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.Equal(t, "ANALYTICS", got.Status.DatabaseName)
	assert.Equal(t, "PUBLIC", got.Status.SchemaName)
}

func TestReconcile_CreateExternalStage(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Finalizers = []string{finalizerName}
	stage.Spec.URL = testutil.Ptr("s3://my-bucket/path/")
	stage.Spec.StorageIntegration = testutil.Ptr("MY_INT")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateStageOptions

	obs := successfulObservation()
	obs.ShowOutput.Type = "EXTERNAL"
	obs.ShowOutput.URL = "s3://my-bucket/path/"
	obs.ShowOutput.StorageIntegration = "MY_INT"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StageObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateStageOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, stage, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.NoError(t, err)
	assert.Equal(t, "s3://my-bucket/path/", *capturedOpts.URL)
	assert.Equal(t, "MY_INT", *capturedOpts.StorageIntegration)
}

func TestReconcile_CreateError(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")
	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateStageOptions) error {
			return fmt.Errorf("create failed")
		},
	}

	r := newTestReconciler(mock, stage, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteStage(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	stage := newTestStage("mystage", "default")
	stage.Finalizers = []string{finalizerName}
	stage.DeletionTimestamp = &now
	stage.Status.DatabaseName = `"ANALYTICS"`
	stage.Status.SchemaName = `"ANALYTICS"."PUBLIC"`
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var dropped bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StageObservation, error) {
			return &snowflake.StageObservation{Exists: true}, nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, stage, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mystage", "default"))
	require.NoError(t, err)
	assert.True(t, dropped)
}

// --------------------------------------------------------------------------
// Tests: Immutability checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_NameChanged(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Status.ObservedGeneration = 1
	stage.Status.ShowOutput = &snowplanev1alpha1.StageShowOutput{
		Name:         "DATA_LOAD",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
		Type:         "INTERNAL",
	}
	stage.Spec.Name = "DIFFERENT_STAGE"
	err := validateImmutableFields(context.Background(), stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

func TestValidateImmutableFields_TypeChanged(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Status.ObservedGeneration = 1
	stage.Status.ShowOutput = &snowplanev1alpha1.StageShowOutput{
		Name:         "DATA_LOAD",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
		Type:         "INTERNAL",
	}
	// Change to external by setting URL.
	stage.Spec.URL = testutil.Ptr("s3://bucket/path/")
	err := validateImmutableFields(context.Background(), stage)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert between internal and external stage types")
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Status.ObservedGeneration = 1
	stage.Status.ShowOutput = &snowplanev1alpha1.StageShowOutput{Name: "DATA_LOAD", Type: "INTERNAL"}
	stage.Annotations = map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"}
	stage.Spec.Name = "DIFFERENT"
	err := validateImmutableFields(context.Background(), stage)
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Build helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Spec.Comment = testutil.Ptr("test stage")
	stage.Spec.URL = testutil.Ptr("s3://bucket/path/")
	stage.Spec.StorageIntegration = testutil.Ptr("MY_INT")
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DATA_LOAD")

	opts := buildCreateOptions(stage, id)
	assert.Equal(t, "DATA_LOAD", opts.Name.Name())
	assert.Equal(t, "test stage", *opts.Comment)
	assert.Equal(t, "s3://bucket/path/", *opts.URL)
	assert.Equal(t, "MY_INT", *opts.StorageIntegration)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Spec.Comment = testutil.Ptr("new comment")
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "DATA_LOAD")
	obs := successfulObservation()

	opts := buildAlterOptions(stage, id, obs)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "new comment", *opts.Comment)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.StageSpec{
		Comment:            testutil.Ptr("test"),
		URL:                testutil.Ptr("s3://bucket/"),
		StorageIntegration: testutil.Ptr("MY_INT"),
	}

	fields := tracked.ComputeTracked(spec)
	assert.Contains(t, fields, "COMMENT")
	assert.Contains(t, fields, "URL")
	assert.Contains(t, fields, "STORAGE_INTEGRATION")
}

func TestDetectDrift_CommentDrift(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	stage.Spec.Comment = testutil.Ptr("expected")

	obs := &snowflake.StageObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.StageShowOutput{
			Comment: "actual",
		},
	}

	result := detectDrift(stage, obs)
	assert.True(t, result.HasDrift)
}

// --------------------------------------------------------------------------
// Tests: Watch mapping
// --------------------------------------------------------------------------

func TestMapDatabaseToStages(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, stage, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.StageList{} }, ".spec.databaseRef.name", "listing stages for database watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), db)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mystage", reqs[0].Name)
}

func TestMapSchemaToStages(t *testing.T) {
	t.Parallel()

	stage := newTestStage("mystage", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, stage, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.StageList{} }, ".spec.schemaRef.name", "listing stages for schema watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), schema)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mystage", reqs[0].Name)
}

// ---------------------------------------------------------------------------
// Tests: computeUnsetFields (M-2)
// ---------------------------------------------------------------------------

func TestComputeUnsetFields_NoPreviousTracking(t *testing.T) {
	t.Parallel()

	stage := newTestStage("s", "default")
	stage.Status.TrackedParameters = nil // no previous tracking

	unset := tracked.ComputeUnset(&stage.Spec, stage.Status.TrackedParameters)
	assert.Empty(t, unset)
}

func TestComputeUnsetFields_CommentRemoved(t *testing.T) {
	t.Parallel()

	stage := newTestStage("s", "default")
	stage.Status.TrackedParameters = []string{"COMMENT", "URL"}
	stage.Spec.Comment = nil // was set → now removed
	stage.Spec.URL = testutil.Ptr("s3://bucket/")

	unset := tracked.ComputeUnset(&stage.Spec, stage.Status.TrackedParameters)
	assert.Contains(t, unset, "COMMENT")
	assert.NotContains(t, unset, "URL")
}

func TestComputeUnsetFields_AllRemoved(t *testing.T) {
	t.Parallel()

	stage := newTestStage("s", "default")
	stage.Status.TrackedParameters = []string{"COMMENT", "URL", "STORAGE_INTEGRATION", "FILE_FORMAT", "DIRECTORY"}
	stage.Spec.Comment = nil
	stage.Spec.URL = nil
	stage.Spec.StorageIntegration = nil
	stage.Spec.FileFormat = nil
	stage.Spec.Directory = nil

	unset := tracked.ComputeUnset(&stage.Spec, stage.Status.TrackedParameters)
	assert.Len(t, unset, 5)
	assert.Contains(t, unset, "COMMENT")
	assert.Contains(t, unset, "URL")
	assert.Contains(t, unset, "STORAGE_INTEGRATION")
	assert.Contains(t, unset, "FILE_FORMAT")
	assert.Contains(t, unset, "DIRECTORY")
}

func TestComputeUnsetFields_NoneRemoved(t *testing.T) {
	t.Parallel()

	stage := newTestStage("s", "default")
	stage.Status.TrackedParameters = []string{"COMMENT"}
	stage.Spec.Comment = testutil.Ptr("still here")

	unset := tracked.ComputeUnset(&stage.Spec, stage.Status.TrackedParameters)
	assert.Empty(t, unset)
}

func TestBuildAlterOptions_IncludesUnsetFields(t *testing.T) {
	t.Parallel()

	stage := newTestStage("s", "default")
	stage.Status.TrackedParameters = []string{"COMMENT", "URL"}
	stage.Spec.Comment = nil
	stage.Spec.URL = nil
	id := snowflake.NewSchemaObjectIdentifier("DB", "SCH", "MYSTAGE")
	obs := successfulObservation()

	opts := buildAlterOptions(stage, id, obs)
	assert.True(t, opts.HasChanges(), "Unset fields should trigger HasChanges")
	assert.Contains(t, opts.UnsetFields, "COMMENT")
	assert.Contains(t, opts.UnsetFields, "URL")
}
