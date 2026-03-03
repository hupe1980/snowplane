package table

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTableOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTableOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.TableObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateTableOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterTableOptions) error {
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

func newTestTable(name, namespace string) *snowplanev1alpha1.Table {
	return &snowplanev1alpha1.Table{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.TableSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "EVENTS",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "analytics-db"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "public-schema"},
			Columns: []snowplanev1alpha1.ColumnDefinition{
				{Name: "ID", Type: "NUMBER(38,0)"},
				{Name: "PAYLOAD", Type: "VARIANT"},
			},
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

func successfulObservation() *snowflake.TableObservation {
	return &snowflake.TableObservation{
		Exists: true,
		ShowOutput: &snowflake.TableShowOutput{
			CreatedOn:             "2024-01-01",
			Name:                  "EVENTS",
			DatabaseName:          "ANALYTICS",
			SchemaName:            "PUBLIC",
			Kind:                  "TABLE",
			Comment:               "",
			Owner:                 "SYSADMIN",
			RetentionTime:         1,
			ClusterBy:             "",
			ChangeTracking:        false,
			EnableSchemaEvolution: false,
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Table{},
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

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Table"),
	}
}

func newTestReconcilerWithIndex(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.Table{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		).
		WithIndex(&snowplanev1alpha1.Table{}, ".spec.databaseRef.name", func(o client.Object) []string {
			t, ok := o.(*snowplanev1alpha1.Table)
			if !ok {
				return nil
			}

			return []string{t.Spec.DatabaseRef.Name}
		}).
		WithIndex(&snowplanev1alpha1.Table{}, ".spec.schemaRef.name", func(o client.Object) []string {
			t, ok := o.(*snowplanev1alpha1.Table)
			if !ok {
				return nil
			}

			return []string{t.Spec.SchemaRef.Name}
		})

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Table, Service, *snowflake.TableObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Table"),
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
			return newTestTable(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.Table{}
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
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateTable(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateTableOptions

	obs := successfulObservation()
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TableObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateTableOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "ANALYTICS", capturedOpts.Name.DatabaseName())
	assert.Equal(t, "PUBLIC", capturedOpts.Name.SchemaName())
	assert.Equal(t, "EVENTS", capturedOpts.Name.Name())
	assert.Len(t, capturedOpts.Columns, 2)
	assert.Equal(t, "ID", capturedOpts.Columns[0].Name)
	assert.False(t, capturedOpts.Transient)

	got := &snowplanev1alpha1.Table{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mytable", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
	assert.Equal(t, "SYSADMIN", got.Status.ShowOutput.Owner)
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.Equal(t, "ANALYTICS", got.Status.DatabaseName)
	assert.Equal(t, "PUBLIC", got.Status.SchemaName)
}

func TestReconcile_CreateTransientTable(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	table.Spec.Transient = true
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var capturedOpts snowflake.CreateTableOptions

	obs := successfulObservation()
	obs.ShowOutput.Kind = "TRANSIENT"
	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
				call++
				if call == 1 {
					return &snowflake.TableObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateTableOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.NoError(t, err)
	assert.True(t, capturedOpts.Transient)
}

func TestReconcile_CreateError(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")
	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateTableOptions) error {
			return fmt.Errorf("create failed")
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateComment(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	table.Spec.ManagementPolicies.CreateOrAlter = testutil.Ptr(false)
	table.Spec.Comment = testutil.Ptr("updated comment")
	table.Status.ObservedGeneration = 1
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	obs := successfulObservation()

	var capturedOpts snowflake.AlterTableOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterTableOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.NoError(t, err)
	require.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated comment", *capturedOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteTable(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	table.DeletionTimestamp = &now
	table.Status.DatabaseName = `"ANALYTICS"`
	table.Status.SchemaName = `"ANALYTICS"."PUBLIC"`
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var dropped bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
			return &snowflake.TableObservation{Exists: true}, nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.NoError(t, err)
	assert.True(t, dropped)
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	table := newTestTable("mytable", "default")
	table.Finalizers = []string{finalizerName}
	table.DeletionTimestamp = &now
	table.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	table.Status.DatabaseName = `"ANALYTICS"`
	table.Status.SchemaName = `"ANALYTICS"."PUBLIC"`
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	var dropped bool

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TableObservation, error) {
			return &snowflake.TableObservation{Exists: true}, nil
		},
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropped = true
			return nil
		},
	}

	r := newTestReconciler(mock, table, db, schema, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytable", "default"))
	require.NoError(t, err)
	assert.False(t, dropped)
}

// --------------------------------------------------------------------------
// Tests: Immutability checks
// --------------------------------------------------------------------------

func TestValidateImmutableFields_NameChanged(t *testing.T) {
	t.Parallel()

	a := &adapter{}
	table := newTestTable("mytable", "default")
	table.Status.ObservedGeneration = 1
	table.Status.ShowOutput = &snowplanev1alpha1.TableShowOutput{
		Name:         "EVENTS",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
		Kind:         "TABLE",
	}
	table.Spec.Name = "EVENTS_V2"
	err := a.ValidateImmutableFields(context.Background(), table)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.name is immutable")
}

func TestValidateImmutableFields_TransientChanged(t *testing.T) {
	t.Parallel()

	a := &adapter{}
	table := newTestTable("mytable", "default")
	table.Status.ObservedGeneration = 1
	table.Status.ShowOutput = &snowplanev1alpha1.TableShowOutput{
		Name:         "EVENTS",
		DatabaseName: "ANALYTICS",
		SchemaName:   "PUBLIC",
		Kind:         "TABLE",
	}
	table.Spec.Transient = true
	err := a.ValidateImmutableFields(context.Background(), table)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.transient is immutable")
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	a := &adapter{}
	table := newTestTable("mytable", "default")
	table.Status.ObservedGeneration = 1
	table.Status.ShowOutput = &snowplanev1alpha1.TableShowOutput{
		Name: "EVENTS",
		Kind: "TABLE",
	}
	table.Annotations = map[string]string{snowplanev1alpha1.AnnotationForceNew: "true"}
	table.Spec.Name = "DIFFERENT"
	err := a.ValidateImmutableFields(context.Background(), table)
	require.NoError(t, err) // forceNew bypasses
}

// --------------------------------------------------------------------------
// Tests: Build helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Comment = testutil.Ptr("test table")
	table.Spec.ChangeTracking = testutil.Ptr(true)
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "EVENTS")

	opts := buildCreateOptions(table, id)
	assert.Equal(t, "EVENTS", opts.Name.Name())
	assert.Len(t, opts.Columns, 2)
	assert.Equal(t, "test table", *opts.Comment)
	assert.True(t, *opts.ChangeTracking)
}

func TestBuildCreateOptions_WithConstraints(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Constraints = []snowplanev1alpha1.InlineTableConstraint{
		{
			Name:    "pk_id",
			Type:    snowplanev1alpha1.TableConstraintPrimaryKey,
			Columns: []string{"ID"},
		},
		{
			Name:    "fk_payload",
			Type:    snowplanev1alpha1.TableConstraintForeignKey,
			Columns: []string{"PAYLOAD"},
			ForeignKey: &snowplanev1alpha1.ForeignKeyReference{
				Table:   "OTHER_TABLE",
				Columns: []string{"REF_COL"},
			},
		},
	}
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "EVENTS")

	opts := buildCreateOptions(table, id)
	require.Len(t, opts.Constraints, 2)

	assert.Equal(t, "pk_id", opts.Constraints[0].Name)
	assert.Equal(t, "PRIMARY KEY", opts.Constraints[0].Type)
	assert.Equal(t, []string{"ID"}, opts.Constraints[0].Columns)

	assert.Equal(t, "fk_payload", opts.Constraints[1].Name)
	assert.Equal(t, "FOREIGN KEY", opts.Constraints[1].Type)
	assert.Equal(t, []string{"PAYLOAD"}, opts.Constraints[1].Columns)
	assert.Equal(t, "OTHER_TABLE", opts.Constraints[1].ForeignKeyTable)
	assert.Equal(t, []string{"REF_COL"}, opts.Constraints[1].ForeignKeyColumns)
}

func TestBuildAlterOptions_CommentChanged(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Comment = testutil.Ptr("new comment")
	id := snowflake.NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "EVENTS")
	obs := successfulObservation()

	opts := buildAlterOptions(table, id, obs)
	require.NotNil(t, opts.Comment)
	assert.Equal(t, "new comment", *opts.Comment)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.TableSpec{
		Comment:                 testutil.Ptr("test"),
		ChangeTracking:          testutil.Ptr(true),
		DataRetentionTimeInDays: testutil.Ptr(int32(7)),
	}

	fields := tracked.ComputeTracked(spec)
	assert.Contains(t, fields, "COMMENT")
	assert.Contains(t, fields, "CHANGE_TRACKING")
	assert.Contains(t, fields, "DATA_RETENTION_TIME_IN_DAYS")
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Status.TrackedParameters = []string{"COMMENT", "CHANGE_TRACKING"}
	// Spec has no comment or changeTracking → both should be unset.

	unset := tracked.ComputeUnset(&table.Spec, table.Status.TrackedParameters)
	assert.Contains(t, unset, "COMMENT")
	assert.Contains(t, unset, "CHANGE_TRACKING")
}

func TestDetectDrift_CommentDrift(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Comment = testutil.Ptr("expected")

	obs := &snowflake.TableObservation{
		Exists: true,
		ShowOutput: &snowflake.TableShowOutput{
			Comment: "actual",
		},
	}

	result := detectDrift(table, obs)
	assert.True(t, result.HasDrift)
	assert.NotEmpty(t, result.Changes)
}

// --------------------------------------------------------------------------
// Tests: Column drift detection
// --------------------------------------------------------------------------

func TestDetectColumnDrift_NoDrift(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Columns = []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
		{Name: "PAYLOAD", Type: "VARIANT"},
	}

	obs := &snowflake.TableObservation{
		Exists:     true,
		ShowOutput: &snowflake.TableShowOutput{Name: "MYTABLE", Kind: "TABLE"},
		Columns: []snowflake.ColumnInfo{
			{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
			{Name: "PAYLOAD", Type: "VARIANT", Kind: "COLUMN", Null: "Y"},
		},
	}

	result := detectDrift(table, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectColumnDrift_MissingColumn(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Columns = []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
		{Name: "NEW_COL", Type: "VARCHAR(100)"},
	}

	obs := &snowflake.TableObservation{
		Exists:     true,
		ShowOutput: &snowflake.TableShowOutput{Name: "MYTABLE", Kind: "TABLE"},
		Columns: []snowflake.ColumnInfo{
			{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
		},
	}

	result := detectDrift(table, obs)
	assert.True(t, result.HasDrift)
}

func TestDetectColumnDrift_ExtraColumn(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Columns = []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
	}

	obs := &snowflake.TableObservation{
		Exists:     true,
		ShowOutput: &snowflake.TableShowOutput{Name: "MYTABLE", Kind: "TABLE"},
		Columns: []snowflake.ColumnInfo{
			{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
			{Name: "STALE_COL", Type: "VARCHAR", Kind: "COLUMN", Null: "Y"},
		},
	}

	result := detectDrift(table, obs)
	assert.True(t, result.HasDrift)
}

func TestDetectColumnDrift_TypeChange(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	table.Spec.Columns = []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
		{Name: "AMOUNT", Type: "NUMBER(18,2)"},
	}

	obs := &snowflake.TableObservation{
		Exists:     true,
		ShowOutput: &snowflake.TableShowOutput{Name: "MYTABLE", Kind: "TABLE"},
		Columns: []snowflake.ColumnInfo{
			{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
			{Name: "AMOUNT", Type: "NUMBER(10,0)", Kind: "COLUMN", Null: "Y"},
		},
	}

	result := detectDrift(table, obs)
	assert.True(t, result.HasDrift)
}

func TestDetectColumnDrift_NullableChange(t *testing.T) {
	t.Parallel()

	notNull := false
	table := newTestTable("mytable", "default")
	table.Spec.Columns = []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)", Nullable: &notNull},
	}

	obs := &snowflake.TableObservation{
		Exists:     true,
		ShowOutput: &snowflake.TableShowOutput{Name: "MYTABLE", Kind: "TABLE"},
		Columns: []snowflake.ColumnInfo{
			{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
		},
	}

	result := detectDrift(table, obs)
	assert.True(t, result.HasDrift)
}

// --------------------------------------------------------------------------
// Tests: computeColumnChanges
// --------------------------------------------------------------------------

func TestComputeColumnChanges_AddColumn(t *testing.T) {
	t.Parallel()

	specCols := []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
		{Name: "NEW_COL", Type: "VARCHAR(200)", Comment: testutil.Ptr("added")},
	}
	obsCols := []snowflake.ColumnInfo{
		{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
	}

	add, drop, alter := computeColumnChanges(specCols, obsCols)
	assert.Len(t, add, 1)
	assert.Equal(t, "NEW_COL", add[0].Name)
	assert.Equal(t, "VARCHAR(200)", add[0].Type)
	assert.Empty(t, drop)
	assert.Empty(t, alter)
}

func TestComputeColumnChanges_DropColumn(t *testing.T) {
	t.Parallel()

	specCols := []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
	}
	obsCols := []snowflake.ColumnInfo{
		{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
		{Name: "STALE", Type: "VARCHAR", Kind: "COLUMN", Null: "Y"},
	}

	add, drop, alter := computeColumnChanges(specCols, obsCols)
	assert.Empty(t, add)
	assert.Equal(t, []string{"STALE"}, drop)
	assert.Empty(t, alter)
}

func TestComputeColumnChanges_AlterComment(t *testing.T) {
	t.Parallel()

	specCols := []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)", Comment: testutil.Ptr("primary key")},
	}
	obsCols := []snowflake.ColumnInfo{
		{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y", Comment: "old comment"},
	}

	add, drop, alter := computeColumnChanges(specCols, obsCols)
	assert.Empty(t, add)
	assert.Empty(t, drop)
	require.Len(t, alter, 1)
	assert.Equal(t, "ID", alter[0].Name)
	require.NotNil(t, alter[0].SetComment)
	assert.Equal(t, "primary key", *alter[0].SetComment)
}

func TestComputeColumnChanges_NoChanges(t *testing.T) {
	t.Parallel()

	specCols := []snowplanev1alpha1.ColumnDefinition{
		{Name: "ID", Type: "NUMBER(38,0)"},
	}
	obsCols := []snowflake.ColumnInfo{
		{Name: "ID", Type: "NUMBER(38,0)", Kind: "COLUMN", Null: "Y"},
	}

	add, drop, alter := computeColumnChanges(specCols, obsCols)
	assert.Empty(t, add)
	assert.Empty(t, drop)
	assert.Empty(t, alter)
}

func TestComputeColumnChanges_TypeChange(t *testing.T) {
	t.Parallel()

	specCols := []snowplanev1alpha1.ColumnDefinition{
		{Name: "AMOUNT", Type: "NUMBER(18,2)"},
	}
	obsCols := []snowflake.ColumnInfo{
		{Name: "AMOUNT", Type: "NUMBER(10,0)", Kind: "COLUMN", Null: "Y"},
	}

	add, drop, alter := computeColumnChanges(specCols, obsCols)
	assert.Empty(t, add)
	assert.Empty(t, drop)
	require.Len(t, alter, 1)
	assert.Equal(t, "AMOUNT", alter[0].Name)
	require.NotNil(t, alter[0].SetType)
	assert.Equal(t, "NUMBER(18,2)", *alter[0].SetType)
}

func TestNormaliseType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "NUMBER(38,0)", normaliseType("NUMBER(38,0)"))
	assert.Equal(t, "NUMBER(38,0)", normaliseType("NUMBER (38,0)"))
	assert.Equal(t, "NUMBER(38,0)", normaliseType(" NUMBER(38,0) "))
	assert.Equal(t, "VARCHAR", normaliseType("VARCHAR"))
}

// --------------------------------------------------------------------------
// Tests: Watch mapping
// --------------------------------------------------------------------------

func TestMapDatabaseToTables(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, table, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.databaseRef.name", "listing tables for database watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), db)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mytable", reqs[0].Name)
}

func TestMapSchemaToTables(t *testing.T) {
	t.Parallel()

	table := newTestTable("mytable", "default")
	db := newTestDB("analytics-db", "default")
	schema := newTestSchema("public-schema", "default")

	r := newTestReconcilerWithIndex(&mockService{}, table, db, schema)
	mapFunc := refresolver.MapByFieldIndex(r.Client, func() client.ObjectList { return &snowplanev1alpha1.TableList{} }, ".spec.schemaRef.name", "listing tables for schema watch")
	//nolint:staticcheck
	reqs := mapFunc(context.Background(), schema)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "mytable", reqs[0].Name)
}
