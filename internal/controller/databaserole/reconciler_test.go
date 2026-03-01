package databaserole

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
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/tracked"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error
	dropFn    func(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.DatabaseRoleObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error {
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

func newTestDatabaseRole(name, namespace string) *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseRoleSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "DATA_ANALYST",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "my-db"},
		},
	}
}

func newTestDatabase(name, namespace string) *snowplanev1alpha1.Database {
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
			Name: "MY_DB",
		},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				ObservedGeneration: 1,
				FullyQualifiedName: `"MY_DB"`,
				Conditions: []metav1.Condition{
					{
						Type:               snowplanev1alpha1.TypeReady,
						Status:             metav1.ConditionTrue,
						Reason:             "Available",
						Message:            "Database is ready",
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		},
	}

	return db
}

func newSuccessfulObservation() *snowflake.DatabaseRoleObservation {
	return &snowflake.DatabaseRoleObservation{
		Exists: true,
		ShowOutput: &snowflake.DatabaseRoleShowOutput{
			CreatedOn:      "2024-01-01",
			Name:           "DATA_ANALYST",
			DatabaseName:   "MY_DB",
			Comment:        "",
			Owner:          "SYSADMIN",
			GrantedToRoles: 0,
			GrantedRoles:   0,
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.DatabaseRole{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.ProviderConfig{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	recorder := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: recorder,
		Adapter: &adapter{
			client:   c,
			recorder: recorder,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("DatabaseRole"),
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
			return newTestDatabaseRole(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.DatabaseRole{}
		},
		FinalizerName: finalizerName,
		PrereqObjects: func() []runtime.Object {
			db := newTestDatabase("my-db", "default")
			return []runtime.Object{db}
		},
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_CreateRole(t *testing.T) {
	t.Parallel()

	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateDatabaseRoleOptions

	firstCall := true

	mock := &mockService{
		createFn: func(_ context.Context, opts snowflake.CreateDatabaseRoleOptions) error {
			capturedOpts = opts
			return nil
		},
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
			if firstCall {
				firstCall = false
				return &snowflake.DatabaseRoleObservation{Exists: false}, nil
			}

			return newSuccessfulObservation(), nil
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	// Verify create was called with correct identifier.
	assert.Equal(t, "DATA_ANALYST", capturedOpts.Name.Name())
	assert.Equal(t, "MY_DB", capturedOpts.Name.DatabaseName())

	// Verify status was updated.
	got := &snowplanev1alpha1.DatabaseRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.NotEmpty(t, got.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", got.Status.DatabaseName)
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_AlterRole(t *testing.T) {
	t.Parallel()

	comment := "updated comment"
	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Spec.Comment = &comment
	role.Status.ObservedGeneration = 1
	role.Status.LastAppliedSpecHash = "old-hash"

	var capturedOpts snowflake.AlterDatabaseRoleOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
			return newSuccessfulObservation(), nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterDatabaseRoleOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated comment", *capturedOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_DeleteRole(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.DeletionTimestamp = &now
	role.Status.DatabaseName = "MY_DB"

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.DatabaseObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "DATA_ANALYST", name.Name())
			assert.Equal(t, "MY_DB", name.DatabaseName())
			return nil
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.True(t, dropCalled)
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.DeletionTimestamp = &now
	role.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	role.Status.DatabaseName = "MY_DB"

	dropCalled := false
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)
	assert.False(t, dropCalled, "orphan policy should skip Snowflake DROP")
}

// --------------------------------------------------------------------------
// Tests: Error handling
// --------------------------------------------------------------------------

func TestReconcile_CreateError(t *testing.T) {
	t.Parallel()

	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
			return &snowflake.DatabaseRoleObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateDatabaseRoleOptions) error {
			return fmt.Errorf("snowflake unavailable")
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snowflake unavailable")
}

// --------------------------------------------------------------------------
// Tests: TrackedParameters tracking
// --------------------------------------------------------------------------

func TestReconcile_TrackedParametersTracking(t *testing.T) {
	t.Parallel()

	comment := "test"
	role := newTestDatabaseRole("myrole", "default")
	role.Finalizers = []string{finalizerName}
	role.Status.ObservedGeneration = 1
	role.Spec.Comment = &comment

	obs := newSuccessfulObservation()
	obs.ShowOutput.Comment = "test"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
			return obs, nil
		},
	}

	db := newTestDatabase("my-db", "default")
	r := newTestReconciler(mock, role, db, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myrole", "default"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.DatabaseRole{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myrole", Namespace: "default"}, got))
	assert.Contains(t, got.Status.TrackedParameters, "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: Helper functions
// --------------------------------------------------------------------------

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	t.Run("NoTrackedParameters", func(t *testing.T) {
		t.Parallel()
		role := &snowplanev1alpha1.DatabaseRole{}
		assert.Nil(t, tracked.ComputeUnset(&role.Spec, role.Status.TrackedParameters))
	})

	t.Run("CommentUnset", func(t *testing.T) {
		t.Parallel()
		role := &snowplanev1alpha1.DatabaseRole{}
		role.Status.TrackedParameters = []string{"COMMENT"}
		assert.Equal(t, []string{"COMMENT"}, tracked.ComputeUnset(&role.Spec, role.Status.TrackedParameters))
	})

	t.Run("CommentStillSet", func(t *testing.T) {
		t.Parallel()
		comment := "keep"
		role := &snowplanev1alpha1.DatabaseRole{}
		role.Spec.Comment = &comment
		role.Status.TrackedParameters = []string{"COMMENT"}
		assert.Nil(t, tracked.ComputeUnset(&role.Spec, role.Status.TrackedParameters))
	})
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("NoFields", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.DatabaseRoleSpec{}
		assert.Nil(t, tracked.ComputeTracked(spec))
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "test"
		spec := &snowplanev1alpha1.DatabaseRoleSpec{Comment: &comment}
		assert.Equal(t, []string{"COMMENT"}, tracked.ComputeTracked(spec))
	})
}

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()
		comment := "hello"
		role := &snowplanev1alpha1.DatabaseRole{}
		role.Spec.Comment = &comment
		obs := &snowflake.DatabaseRoleObservation{
			ShowOutput: &snowflake.DatabaseRoleShowOutput{Comment: "hello"},
		}
		result := detectDrift(role, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("CommentDrift", func(t *testing.T) {
		t.Parallel()
		comment := "new"
		role := &snowplanev1alpha1.DatabaseRole{}
		role.Spec.Comment = &comment
		obs := &snowflake.DatabaseRoleObservation{
			ShowOutput: &snowflake.DatabaseRoleShowOutput{Comment: "old"},
		}
		result := detectDrift(role, obs)
		assert.True(t, result.HasDrift)
	})
}
