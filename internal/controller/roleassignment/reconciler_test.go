package roleassignment

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

type mockService struct {
	observeFn func(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error)
	grantFn   func(ctx context.Context, opts snowflake.GrantRoleOptions) error
	revokeFn  func(ctx context.Context, opts snowflake.RevokeRoleOptions) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}
	return &snowflake.RoleAssignmentObservation{Exists: false}, nil
}

func (m *mockService) GrantRole(ctx context.Context, opts snowflake.GrantRoleOptions) error {
	if m.grantFn != nil {
		return m.grantFn(ctx, opts)
	}
	return nil
}

func (m *mockService) RevokeRole(ctx context.Context, opts snowflake.RevokeRoleOptions) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, opts)
	}
	return nil
}


func newTestARA(name, ns string) *snowplanev1alpha1.AccountRoleAssignment {
	return &snowplanev1alpha1.AccountRoleAssignment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.AccountRoleAssignmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			RoleName: testutil.Ptr("ANALYST"),
			ToRole:   testutil.Ptr("SYSADMIN"),
		},
	}
}

func newTestDRA(name, ns string) *snowplanev1alpha1.DatabaseRoleAssignment {
	return &snowplanev1alpha1.DatabaseRoleAssignment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.DatabaseRoleAssignmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			DatabaseRoleName: testutil.Ptr("MY_DB.READER"),
			ToRole:           testutil.Ptr("SYSADMIN"),
		},
	}
}

func okAccountObs() *snowflake.RoleAssignmentObservation {
	return &snowflake.RoleAssignmentObservation{
		Exists: true,
		ShowOutput: &snowflake.RoleAssignmentShowOutput{
			CreatedOn: "2024-01-01", Role: "ANALYST",
			GrantedTo: "ROLE", GranteeName: "SYSADMIN", GrantedBy: "SECURITYADMIN",
		},
	}
}

func okDatabaseObs() *snowflake.RoleAssignmentObservation {
	return &snowflake.RoleAssignmentObservation{
		Exists: true,
		ShowOutput: &snowflake.RoleAssignmentShowOutput{
			CreatedOn: "2024-01-01", Role: "READER",
			GrantedTo: "ROLE", GranteeName: "SYSADMIN", GrantedBy: "SECURITYADMIN",
		},
	}
}

func araReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
		&snowplanev1alpha1.AccountRoleAssignment{},
		&snowplanev1alpha1.ProviderConfig{},
		&snowplanev1alpha1.AccountRole{},
		&snowplanev1alpha1.User{},
	)
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	recorder := record.NewFakeRecorder(100)
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client: c, Factory: factory, Recorder: recorder,
		Adapter: &accountRoleAssignmentAdapter{
			client: c, recorder: recorder,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("AccountRoleAssignment"),
	}
}

func draReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(
		&snowplanev1alpha1.DatabaseRoleAssignment{},
		&snowplanev1alpha1.ProviderConfig{},
		&snowplanev1alpha1.DatabaseRole{},
		&snowplanev1alpha1.AccountRole{},
	)
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	recorder := record.NewFakeRecorder(100)
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client: c, Factory: factory, Recorder: recorder,
		Adapter: &databaseRoleAssignmentAdapter{
			client: c, recorder: recorder,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("DatabaseRoleAssignment"),
	}
}

func TestARA_CRNotFound(t *testing.T) {
	t.Parallel()
	r := araReconciler(&mockService{})
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("gone", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestARA_AddsFinalizer(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	r := araReconciler(&mockService{}, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-ara", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)
	got := &snowplanev1alpha1.AccountRoleAssignment{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-ara", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, accountRoleAssignmentFinalizer)
}

func TestARA_Grant_ToRole(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	obj.Finalizers = []string{accountRoleAssignmentFinalizer}
	var captured snowflake.GrantRoleOptions
	first := true
	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.GrantRoleOptions) error {
			captured = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
			if first {
				first = false
				return &snowflake.RoleAssignmentObservation{Exists: false}, nil
			}
			return okAccountObs(), nil
		},
	}
	r := araReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-ara", "default"))
	require.NoError(t, err)
	assert.Equal(t, "ANALYST", captured.RoleName)
	assert.False(t, captured.IsDatabaseRole)
	assert.Equal(t, "SYSADMIN", captured.ToRole)
	assert.Empty(t, captured.ToUser)
	got := &snowplanev1alpha1.AccountRoleAssignment{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-ara", Namespace: "default"}, got))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
}

func TestARA_Grant_ToUser(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	obj.Finalizers = []string{accountRoleAssignmentFinalizer}
	obj.Spec.ToRole = nil
	obj.Spec.ToUser = testutil.Ptr("john")
	var captured snowflake.GrantRoleOptions
	first := true
	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.GrantRoleOptions) error {
			captured = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
			if first {
				first = false
				return &snowflake.RoleAssignmentObservation{Exists: false}, nil
			}
			return &snowflake.RoleAssignmentObservation{
				Exists: true,
				ShowOutput: &snowflake.RoleAssignmentShowOutput{
					CreatedOn: "2024-01-01", Role: "ANALYST",
					GrantedTo: "USER", GranteeName: "john", GrantedBy: "SECURITYADMIN",
				},
			}, nil
		},
	}
	r := araReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-ara", "default"))
	require.NoError(t, err)
	assert.Equal(t, "john", captured.ToUser)
	assert.Empty(t, captured.ToRole)
}

func TestARA_Observe_NoRequeue(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	obj.Finalizers = []string{accountRoleAssignmentFinalizer}
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
			return okAccountObs(), nil
		},
	}
	r := araReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-ara", "default"))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestARA_Revoke(t *testing.T) {
	t.Parallel()
	now := metav1.Now()
	obj := newTestARA("my-ara", "default")
	obj.Finalizers = []string{accountRoleAssignmentFinalizer}
	obj.DeletionTimestamp = &now
	var captured snowflake.RevokeRoleOptions
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
			return okAccountObs(), nil
		},
		revokeFn: func(_ context.Context, opts snowflake.RevokeRoleOptions) error {
			captured = opts
			return nil
		},
	}
	r := araReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-ara", "default"))
	require.NoError(t, err)
	assert.Equal(t, "ANALYST", captured.RoleName)
	assert.Equal(t, "SYSADMIN", captured.FromRole)
}

func TestARA_BuildIdentifier_ToRole(t *testing.T) {
	t.Parallel()
	a := &accountRoleAssignmentAdapter{}
	obj := newTestARA("t", "default")
	id, err := a.BuildIdentifier(obj)
	require.NoError(t, err)
	raID := id.(snowflake.RoleAssignmentIdentifier)
	assert.Equal(t, "ANALYST", raID.RoleName)
	assert.False(t, raID.IsDatabaseRole)
	assert.Equal(t, "ROLE", raID.GrantedTo)
	assert.Equal(t, "SYSADMIN", raID.GranteeName)
}

func TestARA_BuildIdentifier_ToUser(t *testing.T) {
	t.Parallel()
	a := &accountRoleAssignmentAdapter{}
	obj := newTestARA("t", "default")
	obj.Spec.ToRole = nil
	obj.Spec.ToUser = testutil.Ptr("john")
	id, err := a.BuildIdentifier(obj)
	require.NoError(t, err)
	raID := id.(snowflake.RoleAssignmentIdentifier)
	assert.Equal(t, "USER", raID.GrantedTo)
	assert.Equal(t, "john", raID.GranteeName)
}

func TestResolveAccountTarget(t *testing.T) {
	t.Parallel()
	t.Run("ToRole", func(t *testing.T) {
		t.Parallel()
		obj := newTestARA("t", "d")
		gt, gn := resolveAccountTarget(obj)
		assert.Equal(t, "ROLE", gt)
		assert.Equal(t, "SYSADMIN", gn)
	})
	t.Run("ToUser", func(t *testing.T) {
		t.Parallel()
		obj := newTestARA("t", "d")
		obj.Spec.ToRole = nil
		obj.Spec.ToUser = testutil.Ptr("john")
		gt, gn := resolveAccountTarget(obj)
		assert.Equal(t, "USER", gt)
		assert.Equal(t, "john", gn)
	})
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		obj := newTestARA("t", "d")
		obj.Spec.ToRole = nil
		gt, gn := resolveAccountTarget(obj)
		assert.Empty(t, gt)
		assert.Empty(t, gn)
	})
}

func TestARA_PreReconcile_RoleRef(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	obj.Spec.RoleName = nil
	obj.Spec.RoleRef = &snowplanev1alpha1.LocalObjectReference{Name: "my-role-cr"}
	role := readyAccountRole("my-role-cr", "default", "ANALYST")
	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj, role).WithStatusSubresource(
		&snowplanev1alpha1.AccountRoleAssignment{}, &snowplanev1alpha1.AccountRole{},
	).Build()
	a := &accountRoleAssignmentAdapter{client: c, recorder: record.NewFakeRecorder(10)}
	err := a.PreReconcile(context.Background(), obj)
	require.NoError(t, err)
	assert.Equal(t, `"ANALYST"`, *obj.Spec.RoleName)
	assert.Nil(t, obj.Spec.RoleRef)
}

func TestARA_PreReconcile_RoleRef_NotFound(t *testing.T) {
	t.Parallel()
	obj := newTestARA("my-ara", "default")
	obj.Spec.RoleName = nil
	obj.Spec.RoleRef = &snowplanev1alpha1.LocalObjectReference{Name: "nonexistent"}
	scheme := testutil.TestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(obj).WithStatusSubresource(
		&snowplanev1alpha1.AccountRoleAssignment{},
	).Build()
	a := &accountRoleAssignmentAdapter{client: c, recorder: record.NewFakeRecorder(10)}
	err := a.PreReconcile(context.Background(), obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving AccountRole ref")
}

// --- DatabaseRoleAssignment tests ---

func TestDRA_CRNotFound(t *testing.T) {
	t.Parallel()
	r := draReconciler(&mockService{})
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("gone", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestDRA_Grant_ToRole(t *testing.T) {
	t.Parallel()
	obj := newTestDRA("my-dra", "default")
	obj.Finalizers = []string{databaseRoleAssignmentFinalizer}
	var captured snowflake.GrantRoleOptions
	first := true
	mock := &mockService{
		grantFn: func(_ context.Context, opts snowflake.GrantRoleOptions) error {
			captured = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
			if first {
				first = false
				return &snowflake.RoleAssignmentObservation{Exists: false}, nil
			}
			return okDatabaseObs(), nil
		},
	}
	r := draReconciler(mock, obj, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("my-dra", "default"))
	require.NoError(t, err)
	assert.Equal(t, "MY_DB.READER", captured.RoleName)
	assert.True(t, captured.IsDatabaseRole)
	assert.Equal(t, "SYSADMIN", captured.ToRole)
	got := &snowplanev1alpha1.DatabaseRoleAssignment{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "my-dra", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestDRA_BuildIdentifier(t *testing.T) {
	t.Parallel()
	a := &databaseRoleAssignmentAdapter{}
	obj := newTestDRA("t", "default")
	id, err := a.BuildIdentifier(obj)
	require.NoError(t, err)
	raID := id.(snowflake.RoleAssignmentIdentifier)
	assert.Equal(t, "MY_DB.READER", raID.RoleName)
	assert.True(t, raID.IsDatabaseRole)
	assert.Equal(t, "ROLE", raID.GrantedTo)
	assert.Equal(t, "SYSADMIN", raID.GranteeName)
}

func TestResolveDatabaseTarget(t *testing.T) {
	t.Parallel()
	t.Run("ToRole", func(t *testing.T) {
		t.Parallel()
		obj := newTestDRA("t", "d")
		gt, gn := resolveDatabaseTarget(obj)
		assert.Equal(t, "ROLE", gt)
		assert.Equal(t, "SYSADMIN", gn)
	})
	t.Run("ToDatabaseRole", func(t *testing.T) {
		t.Parallel()
		obj := newTestDRA("t", "d")
		obj.Spec.ToRole = nil
		obj.Spec.ToDatabaseRole = testutil.Ptr("MY_DB.WRITER")
		gt, gn := resolveDatabaseTarget(obj)
		assert.Equal(t, "DATABASE_ROLE", gt)
		assert.Equal(t, "MY_DB.WRITER", gn)
	})
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		obj := newTestDRA("t", "d")
		obj.Spec.ToRole = nil
		gt, gn := resolveDatabaseTarget(obj)
		assert.Empty(t, gt)
		assert.Empty(t, gn)
	})
}

// --- Shared helper tests ---

func TestAlterOptions_HasChanges(t *testing.T) {
	t.Parallel()
	assert.False(t, roleAssignmentAlterOptions{}.HasChanges())
}

func TestApplyShowOutput(t *testing.T) {
	t.Parallel()
	obs := okAccountObs()
	result := applyRoleAssignmentShowOutput(obs)
	require.NotNil(t, result)
	assert.Equal(t, "ANALYST", result.Role)
	assert.Equal(t, "ROLE", result.GrantedTo)
	assert.Equal(t, "SYSADMIN", result.GranteeName)
}

func TestDetectDrift(t *testing.T) {
	t.Parallel()
	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()
		dr := detectRoleAssignmentDrift("ROLE", "SYSADMIN", okAccountObs())
		assert.False(t, dr.HasDrift)
	})
	t.Run("Drift", func(t *testing.T) {
		t.Parallel()
		dr := detectRoleAssignmentDrift("USER", "john", okAccountObs())
		assert.True(t, dr.HasImmutableViolation)
	})
}

func TestDrop(t *testing.T) {
	t.Parallel()
	t.Run("FromRole", func(t *testing.T) {
		t.Parallel()
		var c snowflake.RevokeRoleOptions
		mock := &mockService{
			revokeFn: func(_ context.Context, opts snowflake.RevokeRoleOptions) error {
				c = opts
				return nil
			},
		}
		id := snowflake.RoleAssignmentIdentifier{RoleName: "ANALYST", GrantedTo: "ROLE", GranteeName: "SYSADMIN"}
		require.NoError(t, roleAssignmentDrop(context.Background(), mock, id))
		assert.Equal(t, "SYSADMIN", c.FromRole)
	})
	t.Run("FromUser", func(t *testing.T) {
		t.Parallel()
		var c snowflake.RevokeRoleOptions
		mock := &mockService{
			revokeFn: func(_ context.Context, opts snowflake.RevokeRoleOptions) error {
				c = opts
				return nil
			},
		}
		id := snowflake.RoleAssignmentIdentifier{RoleName: "ANALYST", GrantedTo: "USER", GranteeName: "john"}
		require.NoError(t, roleAssignmentDrop(context.Background(), mock, id))
		assert.Equal(t, "john", c.FromUser)
	})
	t.Run("UnknownGrantedTo", func(t *testing.T) {
		t.Parallel()
		id := snowflake.RoleAssignmentIdentifier{RoleName: "X", GrantedTo: "UNKNOWN", GranteeName: "Y"}
		err := roleAssignmentDrop(context.Background(), &mockService{}, id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported grantedTo type")
	})
	t.Run("EmptyGrantedTo", func(t *testing.T) {
		t.Parallel()
		id := snowflake.RoleAssignmentIdentifier{RoleName: "X", GrantedTo: "", GranteeName: "Y"}
		err := roleAssignmentDrop(context.Background(), &mockService{}, id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "grantedTo is empty")
	})
	t.Run("FromDatabaseRole", func(t *testing.T) {
		t.Parallel()
		var c snowflake.RevokeRoleOptions
		mock := &mockService{
			revokeFn: func(_ context.Context, opts snowflake.RevokeRoleOptions) error {
				c = opts
				return nil
			},
		}
		id := snowflake.RoleAssignmentIdentifier{RoleName: "MY_DB.READER", IsDatabaseRole: true, GrantedTo: "DATABASE_ROLE", GranteeName: "MY_DB.WRITER"}
		require.NoError(t, roleAssignmentDrop(context.Background(), mock, id))
		assert.Equal(t, "MY_DB.WRITER", c.FromDatabaseRole)
	})
}

func TestARA_BuildIdentifier_EmptyTarget(t *testing.T) {
	t.Parallel()
	a := &accountRoleAssignmentAdapter{}
	obj := newTestARA("t", "default")
	obj.Spec.ToRole = nil
	obj.Spec.ToUser = nil
	_, err := a.BuildIdentifier(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

func TestDRA_BuildIdentifier_EmptyTarget(t *testing.T) {
	t.Parallel()
	a := &databaseRoleAssignmentAdapter{}
	obj := newTestDRA("t", "default")
	obj.Spec.ToRole = nil
	_, err := a.BuildIdentifier(obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of toRole")
}

func TestARA_ApplyObservation_QuotedFQN(t *testing.T) {
	t.Parallel()
	a := &accountRoleAssignmentAdapter{}
	obj := newTestARA("t", "default")
	obs := &reconciler.Observation[*snowflake.RoleAssignmentObservation]{
		Exists: true,
		Detail: okAccountObs(),
	}
	a.ApplyObservation(obj, obs)
	// FQN must use quoted identifiers from RoleAssignmentIdentifier.FullyQualifiedName().
	assert.Equal(t, `GRANT ROLE "ANALYST" TO ROLE "SYSADMIN"`, obj.Status.FullyQualifiedName)
	require.NotNil(t, obj.Status.ShowOutput)
	assert.Equal(t, "ANALYST", obj.Status.ShowOutput.Role)
}

func TestDRA_ApplyObservation_QuotedFQN(t *testing.T) {
	t.Parallel()
	a := &databaseRoleAssignmentAdapter{}
	obj := newTestDRA("t", "default")
	obs := &reconciler.Observation[*snowflake.RoleAssignmentObservation]{
		Exists: true,
		Detail: okDatabaseObs(),
	}
	a.ApplyObservation(obj, obs)
	assert.Equal(t, `GRANT DATABASE ROLE "MY_DB"."READER" TO ROLE "SYSADMIN"`, obj.Status.FullyQualifiedName)
}

func readyAccountRole(name, ns, roleName string) *snowplanev1alpha1.AccountRole {
	role := &snowplanev1alpha1.AccountRole{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.AccountRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"}},
			Name:       roleName,
		},
		Status: snowplanev1alpha1.AccountRoleStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"` + roleName + `"`,
			},
		},
	}
	conditions.SetReady(role, "available")

	return role
}
