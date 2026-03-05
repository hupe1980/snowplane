package grant

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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
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
	observeFn func(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error)
	grantFn   func(ctx context.Context, opts snowflake.CreateGrantOptions) error
	revokeFn  func(ctx context.Context, opts snowflake.RevokeGrantOptions) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}

	return &snowflake.GrantObservation{Exists: false}, nil
}

func (m *mockService) Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error {
	if m.grantFn != nil {
		return m.grantFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, opts)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestGrantPrivilegesToAccountRole(name, namespace string) *snowplanev1alpha1.GrantPrivilegesToAccountRole {
	return &snowplanev1alpha1.GrantPrivilegesToAccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToAccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Privilege: "USAGE",
			On: snowplanev1alpha1.GrantOn{
				AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
					ObjectType: "DATABASE",
					ObjectName: "MY_DB",
				},
			},
			AccountRole: testutil.Ptr("DATA_READER"),
		},
	}
}

func newSuccessfulObservation() *snowflake.GrantObservation {
	return &snowflake.GrantObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.GrantShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   "USAGE",
			GrantedOn:   "DATABASE",
			Name:        "MY_DB",
			GrantedTo:   "ROLE",
			GranteeName: "DATA_READER",
			GrantOption: false,
			GrantedBy:   "SYSADMIN",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.GrantPrivilegesToAccountRole{},
			&snowplanev1alpha1.ProviderConfig{},
			&snowplanev1alpha1.AccountRole{},
			&snowplanev1alpha1.DatabaseRole{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	recorder := record.NewFakeRecorder(100)

	sf := func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, nil, nil
	}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToAccountRole, Service, *snowflake.GrantObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: recorder,
		Adapter:  newGrantPrivilegesToAccountRoleAdapter(c, recorder, sf),
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("GrantPrivilegesToAccountRole"),
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
			return newTestGrantPrivilegesToAccountRole(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
		},
		FinalizerName: grantPrivilegesToAccountRoleFinalizer,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow (GRANT privilege)
// --------------------------------------------------------------------------

func TestReconcile_GrantPrivilege(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			return newSuccessfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)

	// Verify grant was called with correct options.
	assert.Equal(t, "USAGE", capturedOpts.Privilege)
	assert.Equal(t, `ON DATABASE "MY_DB"`, capturedOpts.OnClause)
	assert.Equal(t, `TO ROLE "DATA_READER"`, capturedOpts.ToClause)
	assert.False(t, capturedOpts.WithGrantOption)

	// Verify status was updated.
	got := &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-grant", Namespace: "default"}, got))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
}

func TestReconcile_GrantWithGrantOption(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.Spec.WithGrantOption = true

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			obs := newSuccessfulObservation()
			obs.ShowOutput.GrantOption = true

			return obs, nil
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)

	assert.True(t, capturedOpts.WithGrantOption)
}

// --------------------------------------------------------------------------
// Tests: Observe existing grant (no changes needed)
// --------------------------------------------------------------------------

func TestReconcile_ExistingGrantInSync(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.Status.ObservedGeneration = 1
	hash, err := grant.ComputeSpecHash()
	require.NoError(t, err)
	grant.Status.LastAppliedSpecHash = hash

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			return newSuccessfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)

	// Should requeue at the normal interval, no immediate requeue.
	assert.NotEqual(t, time.Second, result.RequeueAfter, "should not immediately requeue when in sync")
}

// --------------------------------------------------------------------------
// Tests: Delete flow (REVOKE privilege)
// --------------------------------------------------------------------------

func TestReconcile_RevokeGrant(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.DeletionTimestamp = &now

	revokeCalled := false

	mock := &mockService{
		revokeFn: func(_ context.Context, opts snowflake.RevokeGrantOptions) error {
			revokeCalled = true
			assert.Equal(t, "USAGE", opts.Privilege)
			assert.Equal(t, `ON DATABASE "MY_DB"`, opts.OnClause)
			assert.Equal(t, `FROM ROLE "DATA_READER"`, opts.FromClause)
			return nil
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)
	assert.True(t, revokeCalled)
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.DeletionTimestamp = &now
	grant.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	revokeCalled := false
	mock := &mockService{
		revokeFn: func(_ context.Context, _ snowflake.RevokeGrantOptions) error {
			revokeCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)
	assert.False(t, revokeCalled, "orphan policy should skip Snowflake REVOKE")
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_GrantError(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			return &snowflake.GrantObservation{Exists: false}, nil
		},
		grantFn: func(_ context.Context, _ snowflake.CreateGrantOptions) error {
			return fmt.Errorf("snowflake unavailable")
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snowflake unavailable")
}

func TestReconcile_ObserveError(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			return nil, fmt.Errorf("connection timeout")
		},
	}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection timeout")
}

// --------------------------------------------------------------------------
// Tests: Shared helper functions
// --------------------------------------------------------------------------

func TestGrantAlterOptions_HasChanges(t *testing.T) {
	t.Parallel()

	opts := grantAlterOptions{}
	assert.False(t, opts.HasChanges(), "grants should never report changes")
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	obs := newSuccessfulObservation()
	result := detectGrantDrift("USAGE", false, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_PrivilegeDrift(t *testing.T) {
	t.Parallel()

	obs := newSuccessfulObservation()
	obs.ShowOutput.Privilege = "USAGE"

	result := detectGrantDrift("SELECT", false, obs)
	assert.True(t, result.HasImmutableViolation)
}

func TestDetectDrift_GrantOptionDrift(t *testing.T) {
	t.Parallel()

	obs := newSuccessfulObservation()
	obs.ShowOutput.GrantOption = false

	result := detectGrantDrift("USAGE", true, obs)
	assert.True(t, result.HasImmutableViolation)
}

func TestApplyGrantShowOutput(t *testing.T) {
	t.Parallel()

	obs := newSuccessfulObservation()
	output := applyGrantShowOutput(obs)

	require.NotNil(t, output)
	assert.Equal(t, "2024-01-01", output.CreatedOn)
	assert.Equal(t, "USAGE", output.Privilege)
	assert.Equal(t, "DATABASE", output.GrantedOn)
	assert.Equal(t, "MY_DB", output.Name)
	assert.Equal(t, "ROLE", output.GrantedTo)
	assert.Equal(t, "DATA_READER", output.GranteeName)
	assert.False(t, output.GrantOption)
	assert.Equal(t, "SYSADMIN", output.GrantedBy)
}

func TestApplyGrantShowOutput_NilShowOutput(t *testing.T) {
	t.Parallel()

	obs := &snowflake.GrantObservation{Exists: true}
	output := applyGrantShowOutput(obs)
	assert.Nil(t, output)
}

// --------------------------------------------------------------------------
// Tests: ValidateImmutableFields
// --------------------------------------------------------------------------

func TestValidateImmutableFields_FirstReconcile(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Status.ObservedGeneration = 0

	err := a.ValidateImmutableFields(context.Background(), grant)
	assert.NoError(t, err, "should skip validation on first reconcile")
}

func TestValidateImmutableFields_ForceNew(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Status.ObservedGeneration = 1
	grant.Annotations = map[string]string{
		snowplanev1alpha1.AnnotationForceNew: "true",
	}
	grant.Status.ShowOutput = &snowplanev1alpha1.GrantShowOutput{
		Privilege: "DIFFERENT_PRIV",
	}

	err := a.ValidateImmutableFields(context.Background(), grant)
	assert.NoError(t, err, "should skip validation with force-new annotation")
}

func TestValidateImmutableFields_PrivilegeChanged(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Status.ObservedGeneration = 1
	grant.Status.ShowOutput = &snowplanev1alpha1.GrantShowOutput{
		Privilege: "SELECT",
	}
	grant.Spec.Privilege = "USAGE"

	err := a.ValidateImmutableFields(context.Background(), grant)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.privilege is immutable")
}

func TestValidateImmutableFields_WithGrantOptionChanged(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Status.ObservedGeneration = 1
	grant.Status.ShowOutput = &snowplanev1alpha1.GrantShowOutput{
		Privilege:   "USAGE",
		GrantedOn:   "DATABASE",
		Name:        "MY_DB",
		GrantedTo:   "ROLE",
		GranteeName: "DATA_READER",
		GrantOption: true,
	}
	grant.Spec.WithGrantOption = false

	err := a.ValidateImmutableFields(context.Background(), grant)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.withGrantOption is immutable")
}

func TestValidateImmutableFields_NoChanges(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Status.ObservedGeneration = 1
	grant.Status.ShowOutput = &snowplanev1alpha1.GrantShowOutput{
		Privilege:   "USAGE",
		GrantedOn:   "DATABASE",
		Name:        "MY_DB",
		GrantedTo:   "ROLE",
		GranteeName: "DATA_READER",
		GrantOption: false,
	}

	err := a.ValidateImmutableFields(context.Background(), grant)
	assert.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: BuildIdentifier
// --------------------------------------------------------------------------

func TestBuildIdentifier(t *testing.T) {
	t.Parallel()

	a := newGrantPrivilegesToAccountRoleAdapter(nil, nil, nil)
	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Spec.WithGrantOption = true

	id, err := a.BuildIdentifier(grant)
	require.NoError(t, err)

	grantID, ok := id.(snowflake.GrantIdentifier)
	require.True(t, ok)

	assert.Equal(t, "USAGE", grantID.Privilege)
	assert.Equal(t, `ON DATABASE "MY_DB"`, grantID.OnClause)
	assert.Equal(t, `TO ROLE "DATA_READER"`, grantID.ToClause)
	assert.Equal(t, "DATA_READER", grantID.GranteeName)
	assert.Equal(t, snowflake.GrantKindRegular, grantID.Kind)
	assert.Equal(t, `ON DATABASE "MY_DB"`, grantID.ShowGrantsTarget)
}

// --------------------------------------------------------------------------
// Tests: onToParams
// --------------------------------------------------------------------------

func TestOnToParams_Account(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantOn{Account: true}
	p := onToParams(on)
	assert.True(t, p.Account)
}

func TestOnToParams_AccountObject(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantOn{
		AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
			ObjectType: "DATABASE",
			ObjectName: "MY_DB",
		},
	}
	p := onToParams(on)
	assert.Equal(t, "DATABASE", p.AccountObjectType)
	assert.Equal(t, "MY_DB", p.AccountObjectName)
}

func TestOnToParams_SchemaName(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{SchemaName: testutil.Ptr(`DB.SCH`)},
	}
	p := onToParams(on)
	assert.Equal(t, "DB.SCH", p.SchemaName)
}

func TestOnToParams_SchemaObject(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			ObjectType: "TABLE",
			ObjectName: `MY_DB.PUBLIC.MY_TABLE`,
		},
	}
	p := onToParams(on)
	assert.Equal(t, "TABLE", p.SchemaObjectType)
	assert.Equal(t, "MY_DB.PUBLIC.MY_TABLE", p.SchemaObjectName)
}

// --------------------------------------------------------------------------
// Tests: dbRoleOnToParams
// --------------------------------------------------------------------------

func TestDBRoleOnToParams_Database(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "DATABASE", p.AccountObjectType)
	assert.Equal(t, "MY_DB", p.AccountObjectName)
}

func TestDBRoleOnToParams_SchemaName(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{SchemaName: testutil.Ptr("MY_DB.PUBLIC")},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "MY_DB.PUBLIC", p.SchemaName)
}

func TestDBRoleOnToParams_SchemaAllInDB(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{AllInDatabase: testutil.Ptr("MY_DB")},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "MY_DB", p.AllSchemasInDB)
}

func TestDBRoleOnToParams_SchemaFutureInDB(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{FutureInDatabase: testutil.Ptr("MY_DB")},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "MY_DB", p.FutureSchemasInDB)
}

func TestDBRoleOnToParams_SchemaObject(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			ObjectType: "TABLE",
			ObjectName: "MY_DB.PUBLIC.ORDERS",
		},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "TABLE", p.SchemaObjectType)
	assert.Equal(t, "MY_DB.PUBLIC.ORDERS", p.SchemaObjectName)
}

func TestDBRoleOnToParams_SchemaObjectAll(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "TABLES",
				InDatabase:       testutil.Ptr("MY_DB"),
			},
		},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "TABLES", p.AllObjectsTypePlural)
	assert.Equal(t, "MY_DB", p.AllObjectsInDB)
}

func TestDBRoleOnToParams_SchemaObjectFuture(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			Future: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "VIEWS",
				InSchema:         testutil.Ptr("MY_DB.PUBLIC"),
			},
		},
	}
	p := dbRoleOnToParams(on)
	assert.Equal(t, "VIEWS", p.FutureObjectsTypePlural)
	assert.Equal(t, "MY_DB.PUBLIC", p.FutureObjectsInSchema)
}

func TestDBRoleOnToParams_Empty(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{}
	p := dbRoleOnToParams(on)
	assert.False(t, p.Account)
	assert.Empty(t, p.AccountObjectType)
	assert.Empty(t, p.SchemaName)
}

// --------------------------------------------------------------------------
// Tests: buildDBRoleGrantIdentifier
// --------------------------------------------------------------------------

func TestBuildDBRoleGrantIdentifier_Database(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	id, err := buildDBRoleGrantIdentifier(on, "USAGE", `TO DATABASE ROLE "MY_DB"."READER"`, "MY_DB.READER", snowplanev1alpha1.GrantKindRegular)
	require.NoError(t, err)
	assert.Equal(t, "USAGE", id.Privilege)
	assert.Equal(t, `ON DATABASE "MY_DB"`, id.OnClause)
	assert.Equal(t, `TO DATABASE ROLE "MY_DB"."READER"`, id.ToClause)
	assert.Equal(t, "MY_DB.READER", id.GranteeName)
	assert.Equal(t, snowflake.GrantKindRegular, id.Kind)
}

func TestBuildDBRoleGrantIdentifier_FutureKind(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			Future: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "TABLES",
				InDatabase:       testutil.Ptr("MY_DB"),
			},
		},
	}
	id, err := buildDBRoleGrantIdentifier(on, "SELECT", `TO DATABASE ROLE "MY_DB"."R"`, "MY_DB.R", snowplanev1alpha1.GrantKindRegular)
	require.NoError(t, err)
	assert.Equal(t, snowflake.GrantKindFuture, id.Kind)
}

func TestBuildDBRoleGrantIdentifier_AllKind(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	id, err := buildDBRoleGrantIdentifier(on, "ALL PRIVILEGES", `TO DATABASE ROLE "MY_DB"."R"`, "MY_DB.R", snowplanev1alpha1.GrantKindAll)
	require.NoError(t, err)
	assert.Equal(t, snowflake.GrantKindAll, id.Kind)
}

// --------------------------------------------------------------------------
// Tests: buildGrantPrivilegesToShareIdentifier
// --------------------------------------------------------------------------

func TestBuildGrantPrivilegesToShareIdentifier_Database(t *testing.T) {
	t.Parallel()

	db := "SHARED_DB"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{Database: &db}
	id := buildGrantPrivilegesToShareIdentifier(on, "USAGE", "MY_SHARE")
	assert.Equal(t, "USAGE", id.Privilege)
	assert.Equal(t, `ON DATABASE "SHARED_DB"`, id.OnClause)
	assert.Equal(t, `TO SHARE "MY_SHARE"`, id.ToClause)
	assert.Equal(t, "MY_SHARE", id.GranteeName)
	assert.Equal(t, snowflake.GrantKindShare, id.Kind)
	assert.Equal(t, `TO SHARE "MY_SHARE"`, id.ShowGrantsTarget)
}

func TestBuildGrantPrivilegesToShareIdentifier_Schema(t *testing.T) {
	t.Parallel()

	sch := "MY_DB.PUBLIC"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{Schema: &sch}
	id := buildGrantPrivilegesToShareIdentifier(on, "USAGE", "MY_SHARE")
	assert.Equal(t, `ON SCHEMA "MY_DB"."PUBLIC"`, id.OnClause)
	assert.Equal(t, snowflake.GrantKindShare, id.Kind)
}

func TestBuildGrantPrivilegesToShareIdentifier_Table(t *testing.T) {
	t.Parallel()

	tbl := "MY_DB.PUBLIC.ORDERS"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{Table: &tbl}
	id := buildGrantPrivilegesToShareIdentifier(on, "SELECT", "MY_SHARE")
	assert.Equal(t, `ON TABLE "MY_DB"."PUBLIC"."ORDERS"`, id.OnClause)
}

func TestBuildGrantPrivilegesToShareIdentifier_AllTablesInSchema(t *testing.T) {
	t.Parallel()

	sch := "MY_DB.PUBLIC"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{AllTablesInSchema: &sch}
	id := buildGrantPrivilegesToShareIdentifier(on, "SELECT", "MY_SHARE")
	assert.Equal(t, `ON ALL TABLES IN SCHEMA "MY_DB"."PUBLIC"`, id.OnClause)
}

func TestBuildGrantPrivilegesToShareIdentifier_View(t *testing.T) {
	t.Parallel()

	v := "MY_DB.PUBLIC.MY_VIEW"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{View: &v}
	id := buildGrantPrivilegesToShareIdentifier(on, "SELECT", "MY_SHARE")
	assert.Equal(t, `ON VIEW "MY_DB"."PUBLIC"."MY_VIEW"`, id.OnClause)
}

func TestBuildGrantPrivilegesToShareIdentifier_Function(t *testing.T) {
	t.Parallel()

	fn := "MY_DB.PUBLIC.MY_FUNC"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{Function: &fn}
	id := buildGrantPrivilegesToShareIdentifier(on, "USAGE", "MY_SHARE")
	assert.Equal(t, `ON FUNCTION "MY_DB"."PUBLIC"."MY_FUNC"`, id.OnClause)
}

func TestBuildGrantPrivilegesToShareIdentifier_Tag(t *testing.T) {
	t.Parallel()

	tag := "MY_DB.PUBLIC.MY_TAG"
	on := &snowplanev1alpha1.GrantPrivilegesToShareOn{Tag: &tag}
	id := buildGrantPrivilegesToShareIdentifier(on, "READ", "MY_SHARE")
	assert.Equal(t, `ON TAG "MY_DB"."PUBLIC"."MY_TAG"`, id.OnClause)
}

// --------------------------------------------------------------------------
// Tests: hasDBRoleOnRefs / extractDBRoleOnRefs
// --------------------------------------------------------------------------

func TestHasDBRoleOnRefs_NoRefs(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	assert.False(t, hasDBRoleOnRefs(on))
}

func TestHasDBRoleOnRefs_SchemaRef(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			SchemaRef: &snowplanev1alpha1.ObjectReference{Name: "my-schema"},
		},
	}
	assert.True(t, hasDBRoleOnRefs(on))
}

func TestHasDBRoleOnRefs_AllInDatabaseRef(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			AllInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "my-db"},
		},
	}
	assert.True(t, hasDBRoleOnRefs(on))
}

func TestHasDBRoleOnRefs_FutureInDatabaseRef(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			FutureInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "my-db"},
		},
	}
	assert.True(t, hasDBRoleOnRefs(on))
}

func TestHasDBRoleOnRefs_SchemaObjectAllInDatabaseRef(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "TABLES",
				InDatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "my-db"},
			},
		},
	}
	assert.True(t, hasDBRoleOnRefs(on))
}

func TestHasDBRoleOnRefs_SchemaObjectFutureInSchemaRef(t *testing.T) {
	t.Parallel()

	on := &snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			Future: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "VIEWS",
				InSchemaRef:      &snowplanev1alpha1.ObjectReference{Name: "my-schema"},
			},
		},
	}
	assert.True(t, hasDBRoleOnRefs(on))
}

func TestExtractDatabaseRefsFromDBRoleOn_Deduplicates(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			AllInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "same-db"},
		},
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "TABLES",
				InDatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "same-db"},
			},
		},
	}

	refs := extractDatabaseRefsFromDBRoleOn(&on)
	assert.Equal(t, []string{"same-db"}, refs)
}

func TestExtractDatabaseRefsFromDBRoleOn_MultipleRefs(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			AllInDatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "db-a"},
			FutureInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "db-b"},
		},
	}

	refs := extractDatabaseRefsFromDBRoleOn(&on)
	assert.Len(t, refs, 2)
	assert.Contains(t, refs, "db-a")
	assert.Contains(t, refs, "db-b")
}

func TestExtractDatabaseRefsFromDBRoleOn_NoRefs(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	refs := extractDatabaseRefsFromDBRoleOn(&on)
	assert.Nil(t, refs)
}

func TestExtractSchemaRefsFromDBRoleOn_MultipleRefs(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			SchemaRef: &snowplanev1alpha1.ObjectReference{Name: "schema-1"},
		},
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "VIEWS",
				InSchemaRef:      &snowplanev1alpha1.ObjectReference{Name: "schema-2"},
			},
		},
	}

	refs := extractSchemaRefsFromDBRoleOn(&on)
	assert.Len(t, refs, 2)
	assert.Contains(t, refs, "schema-1")
	assert.Contains(t, refs, "schema-2")
}

func TestExtractSchemaRefsFromDBRoleOn_NoRefs(t *testing.T) {
	t.Parallel()

	db := "MY_DB"
	on := snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{Database: &db}
	refs := extractSchemaRefsFromDBRoleOn(&on)
	assert.Nil(t, refs)
}

// --------------------------------------------------------------------------
// Tests: Ref resolution in PreReconcile
// --------------------------------------------------------------------------

func newReadyAccountRole(name, namespace, sfName string) *snowplanev1alpha1.AccountRole {
	role := &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: sfName,
		},
	}
	role.Status.FullyQualifiedName = sfName
	conditions.SetReady(role, "ok")

	return role
}

func TestPreReconcile_AccountRoleRefResolved(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	// Use AccountRoleRef instead of raw AccountRole.
	grant.Spec.AccountRole = nil
	grant.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "reader-role"}

	accountRole := newReadyAccountRole("reader-role", "default", "DATA_READER")

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			// The resolved role name should appear in the TO clause.
			assert.Equal(t, `TO ROLE "DATA_READER"`, opts.ToClause)
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			return newSuccessfulObservation(), nil
		},
	}

	r := newTestReconciler(mock, grant, accountRole, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-grant", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReferencesResolved))
}

func TestPreReconcile_AccountRoleRefNotFound(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.Spec.AccountRole = nil
	grant.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "missing-role"}

	mock := &mockService{}

	r := newTestReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-grant", Namespace: "default"}, got))

	refCond := conditions.Get(got, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionFalse, refCond.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyNotReady, refCond.Reason)
}

func TestPreReconcile_AccountRoleRefNotReady(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToAccountRole("my-grant", "default")
	grant.Finalizers = []string{grantPrivilegesToAccountRoleFinalizer}
	grant.Spec.AccountRole = nil
	grant.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "unready-role"}

	// Create an AccountRole that exists but is NOT ready.
	role := &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unready-role",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "SOME_ROLE",
		},
	}

	mock := &mockService{}

	r := newTestReconciler(mock, grant, role, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-grant", "default"))
	require.Error(t, err)

	got := &snowplanev1alpha1.GrantPrivilegesToAccountRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-grant", Namespace: "default"}, got))

	refCond := conditions.Get(got, snowplanev1alpha1.TypeReferencesResolved)
	require.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionFalse, refCond.Status)
}

// --------------------------------------------------------------------------
// Helpers: buildIndexedClient
// --------------------------------------------------------------------------

func buildIndexedClient(objs ...runtime.Object) client.Client {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.GrantPrivilegesToAccountRole{},
			&snowplanev1alpha1.ProviderConfig{},
			&snowplanev1alpha1.AccountRole{},
			&snowplanev1alpha1.DatabaseRole{},
		).
		WithIndex(&snowplanev1alpha1.GrantPrivilegesToAccountRole{}, argIndexAccountRoleRef, func(o client.Object) []string {
			g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
			if !ok {
				return nil
			}
			if ref := g.Spec.AccountRoleRef; ref != nil {
				return []string{ref.Name}
			}
			return nil
		}).
		WithIndex(&snowplanev1alpha1.GrantPrivilegesToAccountRole{}, argIndexDatabaseRef, func(o client.Object) []string {
			g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
			if !ok {
				return nil
			}
			return extractDatabaseRefsFromOn(&g.Spec.On)
		}).
		WithIndex(&snowplanev1alpha1.GrantPrivilegesToAccountRole{}, argIndexSchemaRef, func(o client.Object) []string {
			g, ok := o.(*snowplanev1alpha1.GrantPrivilegesToAccountRole)
			if !ok {
				return nil
			}
			return extractSchemaRefsFromOn(&g.Spec.On)
		})

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	return cb.Build()
}

// --------------------------------------------------------------------------
// Tests: SetupWatches — listByIndex (AccountRole to GrantPrivilegesToAccountRoles)
// --------------------------------------------------------------------------

func TestListByIndex_AccountRole_FiltersCorrectly(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.AccountRole = nil
	g1.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "reader-role"}

	g2 := newTestGrantPrivilegesToAccountRole("grant-b", "default")
	g2.Spec.AccountRole = nil
	g2.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "writer-role"}

	c := buildIndexedClient(g1, g2)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexAccountRoleRef, "test")

	role := newReadyAccountRole("reader-role", "default", "DATA_READER")
	requests := mapFn(context.Background(), role)

	require.Len(t, requests, 1)
	assert.Equal(t, "grant-a", requests[0].Name)
	assert.Equal(t, "default", requests[0].Namespace)
}

func TestListByIndex_AccountRole_MultipleMatches(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.AccountRole = nil
	g1.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "reader-role"}

	g2 := newTestGrantPrivilegesToAccountRole("grant-b", "default")
	g2.Spec.AccountRole = nil
	g2.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "reader-role"}

	c := buildIndexedClient(g1, g2)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexAccountRoleRef, "test")

	role := newReadyAccountRole("reader-role", "default", "DATA_READER")
	requests := mapFn(context.Background(), role)

	require.Len(t, requests, 2)
	names := []string{requests[0].Name, requests[1].Name}
	assert.Contains(t, names, "grant-a")
	assert.Contains(t, names, "grant-b")
}

func TestListByIndex_AccountRole_NoMatch(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.AccountRole = nil
	g1.Spec.AccountRoleRef = &snowplanev1alpha1.ObjectReference{Name: "reader-role"}

	c := buildIndexedClient(g1)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexAccountRoleRef, "test")

	role := newReadyAccountRole("unrelated-role", "default", "ADMIN")
	requests := mapFn(context.Background(), role)

	assert.Empty(t, requests)
}

// --------------------------------------------------------------------------
// Tests: SetupWatches — listByIndex (Database to GrantPrivilegesToAccountRoles)
// --------------------------------------------------------------------------

func TestListByIndex_Database_AllInDatabaseRef(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.On.AccountObject = nil
	g1.Spec.On.Schema = &snowplanev1alpha1.GrantOnSchema{
		AllInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
	}

	g2 := newTestGrantPrivilegesToAccountRole("grant-b", "default")
	// grant-b uses AccountObject, no database ref

	c := buildIndexedClient(g1, g2)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexDatabaseRef, "test")

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "analytics-db", Namespace: "default"},
	}
	requests := mapFn(context.Background(), db)

	require.Len(t, requests, 1)
	assert.Equal(t, "grant-a", requests[0].Name)
}

func TestListByIndex_Database_SchemaObjectFutureInDatabaseRef(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.On.AccountObject = nil
	g1.Spec.On.SchemaObject = &snowplanev1alpha1.GrantOnSchemaObject{
		Future: &snowplanev1alpha1.GrantOnBulk{
			ObjectTypePlural: "TABLES",
			InDatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
		},
	}

	c := buildIndexedClient(g1)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexDatabaseRef, "test")

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "analytics-db", Namespace: "default"},
	}
	requests := mapFn(context.Background(), db)

	require.Len(t, requests, 1)
	assert.Equal(t, "grant-a", requests[0].Name)
}

// --------------------------------------------------------------------------
// Tests: SetupWatches — listByIndex (Schema to GrantPrivilegesToAccountRoles)
// --------------------------------------------------------------------------

func TestListByIndex_Schema_SchemaRef(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	g1.Spec.On.AccountObject = nil
	g1.Spec.On.Schema = &snowplanev1alpha1.GrantOnSchema{
		SchemaRef: &snowplanev1alpha1.ObjectReference{Name: "my-schema"},
	}

	c := buildIndexedClient(g1)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexSchemaRef, "test")

	schema := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{Name: "my-schema", Namespace: "default"},
	}
	requests := mapFn(context.Background(), schema)

	require.Len(t, requests, 1)
	assert.Equal(t, "grant-a", requests[0].Name)
}

func TestListByIndex_Schema_NoMatch(t *testing.T) {
	t.Parallel()

	g1 := newTestGrantPrivilegesToAccountRole("grant-a", "default")
	// grant-a has no schema ref

	c := buildIndexedClient(g1)
	mapFn := refresolver.MapByFieldIndex(c,
		func() client.ObjectList { return &snowplanev1alpha1.GrantPrivilegesToAccountRoleList{} },
		argIndexSchemaRef, "test")

	schema := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated-schema", Namespace: "default"},
	}
	requests := mapFn(context.Background(), schema)

	assert.Empty(t, requests)
}

// --------------------------------------------------------------------------
// Tests: extractDatabaseRefsFromOn / extractSchemaRefsFromOn
// --------------------------------------------------------------------------

func TestExtractDatabaseRefsFromOn_Deduplicates(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			AllInDatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "same-db"},
		},
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "TABLES",
				InDatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "same-db"},
			},
		},
	}

	refs := extractDatabaseRefsFromOn(&on)
	assert.Equal(t, []string{"same-db"}, refs)
}

func TestExtractSchemaRefsFromOn_MultipleRefs(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantOn{
		Schema: &snowplanev1alpha1.GrantOnSchema{
			SchemaRef: &snowplanev1alpha1.ObjectReference{Name: "schema-1"},
		},
		SchemaObject: &snowplanev1alpha1.GrantOnSchemaObject{
			All: &snowplanev1alpha1.GrantOnBulk{
				ObjectTypePlural: "VIEWS",
				InSchemaRef:      &snowplanev1alpha1.ObjectReference{Name: "schema-2"},
			},
		},
	}

	refs := extractSchemaRefsFromOn(&on)
	assert.Len(t, refs, 2)
	assert.Contains(t, refs, "schema-1")
	assert.Contains(t, refs, "schema-2")
}

func TestExtractDatabaseRefsFromOn_NoRefs(t *testing.T) {
	t.Parallel()

	on := snowplanev1alpha1.GrantOn{
		AccountObject: &snowplanev1alpha1.GrantOnAccountObject{
			ObjectType: "DATABASE",
			ObjectName: "MY_DB",
		},
	}
	refs := extractDatabaseRefsFromOn(&on)
	assert.Nil(t, refs)
}

func TestDedupStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"no dups", []string{"a", "b"}, []string{"a", "b"}},
		{"with dups", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dedupStrings(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToToFrom(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `FROM ROLE "DATA_READER"`, toToFrom(`TO ROLE "DATA_READER"`))
	assert.Equal(t, `FROM DATABASE ROLE "DB"."R"`, toToFrom(`TO DATABASE ROLE "DB"."R"`))
	assert.Equal(t, `FROM SHARE "SH"`, toToFrom(`TO SHARE "SH"`))
	assert.Equal(t, "short", toToFrom("short"))
}

// ==========================================================================
// GrantPrivilegesToDatabaseRole reconciler tests
// ==========================================================================

func newTestGrantPrivilegesToDatabaseRole(name, namespace string) *snowplanev1alpha1.GrantPrivilegesToDatabaseRole {
	return &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToDatabaseRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Privilege:    "SELECT",
			DatabaseRole: testutil.Ptr("MY_DB.DATA_READER"),
			On: snowplanev1alpha1.GrantPrivilegesToDatabaseRoleOn{
				Schema: &snowplanev1alpha1.GrantOnSchema{
					SchemaName: testutil.Ptr("MY_DB.PUBLIC"),
				},
			},
		},
	}
}

func newTestGrantPrivilegesToDatabaseRoleReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.GrantPrivilegesToDatabaseRole{},
			&snowplanev1alpha1.ProviderConfig{},
			&snowplanev1alpha1.DatabaseRole{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	recorder := record.NewFakeRecorder(100)

	sf := func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, nil, nil
	}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToDatabaseRole, Service, *snowflake.GrantObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: recorder,
		Adapter:  newGrantPrivilegesToDatabaseRoleAdapter(c, recorder, sf),
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("GrantPrivilegesToDatabaseRole"),
	}
}

func newDBRoleGrantSuccessfulObservation() *snowflake.GrantObservation {
	return &snowflake.GrantObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.GrantShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   "SELECT",
			GrantedOn:   "SCHEMA",
			Name:        "MY_DB.PUBLIC",
			GrantedTo:   "DATABASE_ROLE",
			GranteeName: "MY_DB.DATA_READER",
			GrantOption: false,
			GrantedBy:   "SYSADMIN",
		},
	}
}

func TestGrantPrivilegesToDatabaseRole_Reconcile_Grant(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToDatabaseRole("my-drg", "default")
	grant.Finalizers = []string{grantPrivilegesToDatabaseRoleFinalizer}

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			return newDBRoleGrantSuccessfulObservation(), nil
		},
	}

	r := newTestGrantPrivilegesToDatabaseRoleReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-drg", "default"))
	require.NoError(t, err)

	assert.Equal(t, "SELECT", capturedOpts.Privilege)
	assert.Contains(t, capturedOpts.OnClause, "SCHEMA")
	assert.Contains(t, capturedOpts.ToClause, "DATABASE ROLE")
	assert.False(t, capturedOpts.WithGrantOption)

	got := &snowplanev1alpha1.GrantPrivilegesToDatabaseRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-drg", Namespace: "default"}, got))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
}

func TestGrantPrivilegesToDatabaseRole_Reconcile_Revoke(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	grant := newTestGrantPrivilegesToDatabaseRole("my-drg", "default")
	grant.Finalizers = []string{grantPrivilegesToDatabaseRoleFinalizer}
	grant.DeletionTimestamp = &now

	revokeCalled := false

	mock := &mockService{
		revokeFn: func(_ context.Context, opts snowflake.RevokeGrantOptions) error {
			revokeCalled = true
			assert.Equal(t, "SELECT", opts.Privilege)
			assert.Contains(t, opts.OnClause, "SCHEMA")
			assert.Contains(t, opts.FromClause, "DATABASE ROLE")
			return nil
		},
	}

	r := newTestGrantPrivilegesToDatabaseRoleReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-drg", "default"))
	require.NoError(t, err)
	assert.True(t, revokeCalled)
}

func TestGrantPrivilegesToDatabaseRole_Reconcile_ExistingInSync(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToDatabaseRole("my-drg", "default")
	grant.Finalizers = []string{grantPrivilegesToDatabaseRoleFinalizer}
	grant.Status.ObservedGeneration = 1
	hash, err := grant.ComputeSpecHash()
	require.NoError(t, err)
	grant.Status.LastAppliedSpecHash = hash

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			return newDBRoleGrantSuccessfulObservation(), nil
		},
	}

	r := newTestGrantPrivilegesToDatabaseRoleReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, reconcileErr := r.Reconcile(context.Background(), testutil.ReconcileReq("my-drg", "default"))
	require.NoError(t, reconcileErr)
	assert.NotEqual(t, time.Second, result.RequeueAfter)
}

func TestGrantPrivilegesToDatabaseRole_Reconcile_AllPrivileges(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToDatabaseRole("my-drg", "default")
	grant.Finalizers = []string{grantPrivilegesToDatabaseRoleFinalizer}
	grant.Spec.Privilege = ""
	grant.Spec.AllPrivileges = true

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			obs := newDBRoleGrantSuccessfulObservation()
			obs.ShowOutput.Privilege = "ALL"

			return obs, nil
		},
	}

	r := newTestGrantPrivilegesToDatabaseRoleReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-drg", "default"))
	require.NoError(t, err)
	assert.Equal(t, "ALL PRIVILEGES", capturedOpts.Privilege)
}

// ==========================================================================
// GrantPrivilegesToShare reconciler tests
// ==========================================================================

func newTestGrantPrivilegesToShare(name, namespace string) *snowplanev1alpha1.GrantPrivilegesToShare {
	return &snowplanev1alpha1.GrantPrivilegesToShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.GrantPrivilegesToShareSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Privilege: "USAGE",
			On: snowplanev1alpha1.GrantPrivilegesToShareOn{
				Database: testutil.Ptr("MY_DB"),
			},
			Share: "MY_SHARE",
		},
	}
}

func newTestGrantPrivilegesToShareReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.GrantPrivilegesToShare{},
			&snowplanev1alpha1.ProviderConfig{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	recorder := record.NewFakeRecorder(100)

	sf := func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
		return mock, nil, nil
	}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantPrivilegesToShare, Service, *snowflake.GrantObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: recorder,
		Adapter:  newGrantPrivilegesToShareAdapter(c, recorder, sf),
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("GrantPrivilegesToShare"),
	}
}

func newGrantPrivilegesToShareSuccessfulObservation() *snowflake.GrantObservation {
	return &snowflake.GrantObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.GrantShowOutput{
			CreatedOn:   "2024-01-01",
			Privilege:   "USAGE",
			GrantedOn:   "DATABASE",
			Name:        "MY_DB",
			GrantedTo:   "SHARE",
			GranteeName: "MY_SHARE",
			GrantOption: false,
			GrantedBy:   "SYSADMIN",
		},
	}
}

func TestGrantPrivilegesToShare_Reconcile_Grant(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToShare("my-sg", "default")
	grant.Finalizers = []string{grantPrivilegesToShareFinalizer}

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			return newGrantPrivilegesToShareSuccessfulObservation(), nil
		},
	}

	r := newTestGrantPrivilegesToShareReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-sg", "default"))
	require.NoError(t, err)

	assert.Equal(t, "USAGE", capturedOpts.Privilege)
	assert.Contains(t, capturedOpts.OnClause, "DATABASE")
	assert.Contains(t, capturedOpts.ToClause, "SHARE")
	assert.False(t, capturedOpts.WithGrantOption)

	got := &snowplanev1alpha1.GrantPrivilegesToShare{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-sg", Namespace: "default"}, got))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
}

func TestGrantPrivilegesToShare_Reconcile_Revoke(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	grant := newTestGrantPrivilegesToShare("my-sg", "default")
	grant.Finalizers = []string{grantPrivilegesToShareFinalizer}
	grant.DeletionTimestamp = &now

	revokeCalled := false

	mock := &mockService{
		revokeFn: func(_ context.Context, opts snowflake.RevokeGrantOptions) error {
			revokeCalled = true
			assert.Equal(t, "USAGE", opts.Privilege)
			assert.Contains(t, opts.OnClause, "DATABASE")
			assert.Contains(t, opts.FromClause, "SHARE")
			return nil
		},
	}

	r := newTestGrantPrivilegesToShareReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-sg", "default"))
	require.NoError(t, err)
	assert.True(t, revokeCalled)
}

func TestGrantPrivilegesToShare_Reconcile_ExistingInSync(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToShare("my-sg", "default")
	grant.Finalizers = []string{grantPrivilegesToShareFinalizer}
	grant.Status.ObservedGeneration = 1
	hash, err := grant.ComputeSpecHash()
	require.NoError(t, err)
	grant.Status.LastAppliedSpecHash = hash

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			return newGrantPrivilegesToShareSuccessfulObservation(), nil
		},
	}

	r := newTestGrantPrivilegesToShareReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, reconcileErr := r.Reconcile(context.Background(), testutil.ReconcileReq("my-sg", "default"))
	require.NoError(t, reconcileErr)
	assert.NotEqual(t, time.Second, result.RequeueAfter)
}

func TestGrantPrivilegesToShare_Reconcile_OnTable(t *testing.T) {
	t.Parallel()

	grant := newTestGrantPrivilegesToShare("my-sg", "default")
	grant.Finalizers = []string{grantPrivilegesToShareFinalizer}
	grant.Spec.Privilege = "SELECT"
	grant.Spec.On = snowplanev1alpha1.GrantPrivilegesToShareOn{
		Table: testutil.Ptr("MY_DB.PUBLIC.ORDERS"),
	}

	var capturedOpts snowflake.CreateGrantOptions

	firstCall := true

	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.CreateGrantOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.GrantObservation{Exists: false}, nil
			}

			obs := newGrantPrivilegesToShareSuccessfulObservation()
			obs.ShowOutput.Privilege = "SELECT"
			obs.ShowOutput.GrantedOn = "TABLE"
			obs.ShowOutput.Name = "MY_DB.PUBLIC.ORDERS"

			return obs, nil
		},
	}

	r := newTestGrantPrivilegesToShareReconciler(mock, grant, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-sg", "default"))
	require.NoError(t, err)
	assert.Equal(t, "SELECT", capturedOpts.Privilege)
	assert.Contains(t, capturedOpts.OnClause, "TABLE")
}
