package alert

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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateAlertOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterAlertOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.AlertObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateAlertOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterAlertOptions) error {
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

func newTestAlert(name, namespace string) *snowplanev1alpha1.Alert {
	return &snowplanev1alpha1.Alert{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.AlertSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "MY_ALERT",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
			Condition:    "SELECT 1 FROM my_table WHERE status = 'ERROR'",
			Action:       "CALL my_procedure()",
		},
	}
}

func successfulObservation() *snowflake.AlertObservation {
	return &snowflake.AlertObservation{
		Exists: true,
		ShowOutput: &snowflake.AlertShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "MY_ALERT",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
			Warehouse:    "",
			Schedule:     "",
			State:        "suspended",
			Condition:    "SELECT 1 FROM my_table WHERE status = 'ERROR'",
			Action:       "CALL my_procedure()",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Alert{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Alert, Service, *snowflake.AlertObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Alert"),
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
			return newTestAlert(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.Alert{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateAlertOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AlertObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAlertOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_ALERT", capturedOpts.Name.Name())
	assert.Equal(t, "SELECT 1 FROM my_table WHERE status = 'ERROR'", capturedOpts.Condition)
	assert.Equal(t, "CALL my_procedure()", capturedOpts.Action)

	got := &snowplanev1alpha1.Alert{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myalert", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateWithAllFields(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	alert.Status.WarehouseName = "COMPUTE_WH"
	alert.Spec.Schedule = testutil.PtrString("10 MINUTE")
	alert.Spec.WarehouseName = testutil.PtrString("COMPUTE_WH")
	alert.Spec.Comment = testutil.PtrString("my alert comment")

	var capturedOpts snowflake.CreateAlertOptions
	obs := successfulObservation()
	obs.ShowOutput.Schedule = "10 MINUTE"
	obs.ShowOutput.Warehouse = "COMPUTE_WH"
	obs.ShowOutput.Comment = "my alert comment"

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AlertObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateAlertOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "COMPUTE_WH", *capturedOpts.Warehouse)
	assert.Equal(t, "10 MINUTE", *capturedOpts.Schedule)
	assert.Equal(t, "my alert comment", *capturedOpts.Comment)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		createFn: func(_ context.Context, _ snowflake.CreateAlertOptions) error {
			return fmt.Errorf("snowflake error: insufficient privileges")
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient privileges")
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.ObservedGeneration = 1
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateCommentChanged(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.ObservedGeneration = 1
	alert.Generation = 2
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	alert.Spec.Comment = testutil.PtrString("updated comment")

	obs := successfulObservation()

	var capturedOpts snowflake.AlterAlertOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAlertOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.NotNil(t, capturedOpts.Comment)
	assert.Equal(t, "updated comment", *capturedOpts.Comment)
}

func TestReconcile_UpdateConditionChanged(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.ObservedGeneration = 1
	alert.Generation = 2
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	alert.Spec.Condition = "SELECT 1 FROM new_table WHERE status = 'CRITICAL'"

	obs := successfulObservation()

	var capturedOpts snowflake.AlterAlertOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAlertOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	require.NotNil(t, capturedOpts.Condition)
	assert.Equal(t, "SELECT 1 FROM new_table WHERE status = 'CRITICAL'", *capturedOpts.Condition)
}

func TestReconcile_UpdateActionChanged(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.ObservedGeneration = 1
	alert.Generation = 2
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	alert.Spec.Action = "CALL new_procedure()"

	obs := successfulObservation()

	var capturedOpts snowflake.AlterAlertOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterAlertOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	require.NotNil(t, capturedOpts.Action)
	assert.Equal(t, "CALL new_procedure()", *capturedOpts.Action)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	alert.DeletionTimestamp = &now

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.Alert{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myalert", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	alert.DeletionTimestamp = &now

	dropCalled := false

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestReconcile_DeleteDropFails(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	alert.DeletionTimestamp = &now

	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			return fmt.Errorf("drop failed")
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop failed")
}

// --------------------------------------------------------------------------
// Tests: Immutable fields
// --------------------------------------------------------------------------

func TestReconcile_ImmutableName(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.ObservedGeneration = 1
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"
	alert.Spec.Name = "RENAMED_ALERT"
	alert.Status.ShowOutput = &snowplanev1alpha1.AlertShowOutput{
		Name:         "MY_ALERT",
		DatabaseName: "MY_DB",
		SchemaName:   "MY_SCHEMA",
	}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	// Immutable field violations are terminal — reconciler returns nil error
	// but sets NotReady condition.
	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	got := &snowplanev1alpha1.Alert{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myalert", Namespace: "default"}, got))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: Helper unit tests
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Spec.WarehouseName = testutil.PtrString("COMPUTE_WH")
	alert.Status.WarehouseName = "COMPUTE_WH"
	alert.Spec.Schedule = testutil.PtrString("USING CRON 0 9 * * * America/New_York")
	alert.Spec.Comment = testutil.PtrString("my comment")

	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ALERT")
	opts := buildCreateOptions(alert, id)

	assert.Equal(t, "MY_ALERT", opts.Name.Name())
	assert.Equal(t, "COMPUTE_WH", *opts.Warehouse)
	assert.Equal(t, "USING CRON 0 9 * * * America/New_York", *opts.Schedule)
	assert.Equal(t, "my comment", *opts.Comment)
	assert.Equal(t, "SELECT 1 FROM my_table WHERE status = 'ERROR'", opts.Condition)
	assert.Equal(t, "CALL my_procedure()", opts.Action)
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ALERT")

	t.Run("NoChanges", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentChanged", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Spec.Comment = testutil.PtrString("new comment")
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.True(t, opts.HasChanges())
		require.NotNil(t, opts.Comment)
		assert.Equal(t, "new comment", *opts.Comment)
	})

	t.Run("ScheduleChanged", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Spec.Schedule = testutil.PtrString("5 MINUTE")
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.True(t, opts.HasChanges())
		require.NotNil(t, opts.Schedule)
		assert.Equal(t, "5 MINUTE", *opts.Schedule)
	})

	t.Run("ConditionChanged", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Spec.Condition = "SELECT 1 FROM other_table"
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.True(t, opts.HasChanges())
		require.NotNil(t, opts.Condition)
		assert.Equal(t, "SELECT 1 FROM other_table", *opts.Condition)
	})

	t.Run("ActionChanged", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Spec.Action = "CALL new_procedure()"
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.True(t, opts.HasChanges())
		require.NotNil(t, opts.Action)
		assert.Equal(t, "CALL new_procedure()", *opts.Action)
	})

	t.Run("SuspendChanged", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Spec.Suspend = testutil.PtrBool(false)
		obs := successfulObservation()
		opts := buildAlterOptions(alert, id, obs)
		assert.True(t, opts.HasChanges())
		require.NotNil(t, opts.Suspend)
		assert.False(t, *opts.Suspend)
	})
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	t.Run("NoTracked", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		result := tracked.ComputeUnset(&alert.Spec, alert.Status.TrackedParameters)
		assert.Nil(t, result)
	})

	t.Run("CommentTracked_NowNil", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.TrackedParameters = []string{"COMMENT"}
		result := tracked.ComputeUnset(&alert.Spec, alert.Status.TrackedParameters)
		assert.Contains(t, result, "COMMENT")
	})

	t.Run("CommentTracked_StillSet", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.TrackedParameters = []string{"COMMENT"}
		alert.Spec.Comment = testutil.PtrString("still here")
		result := tracked.ComputeUnset(&alert.Spec, alert.Status.TrackedParameters)
		assert.Empty(t, result)
	})
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("NoneSet", func(t *testing.T) {
		spec := &snowplanev1alpha1.AlertSpec{}
		result := tracked.ComputeTracked(spec)
		assert.Empty(t, result)
	})

	t.Run("AllSet", func(t *testing.T) {
		spec := &snowplanev1alpha1.AlertSpec{
			Comment:       testutil.PtrString("c"),
			Schedule:      testutil.PtrString("s"),
			WarehouseName: testutil.PtrString("w"),
		}
		result := tracked.ComputeTracked(spec)
		assert.Contains(t, result, "COMMENT")
		assert.Contains(t, result, "SCHEDULE")
		assert.Contains(t, result, "WAREHOUSE")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	obs := successfulObservation()

	applyObservation(alert, obs)

	require.NotNil(t, alert.Status.ShowOutput)
	assert.Equal(t, "MY_ALERT", alert.Status.ShowOutput.Name)
	assert.Equal(t, "MY_DB", alert.Status.ShowOutput.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", alert.Status.ShowOutput.SchemaName)
	assert.Equal(t, "SYSADMIN", alert.Status.ShowOutput.Owner)
	assert.Equal(t, "suspended", alert.Status.ShowOutput.State)
	assert.Equal(t, "SELECT 1 FROM my_table WHERE status = 'ERROR'", alert.Status.ShowOutput.Condition)
	assert.Equal(t, "CALL my_procedure()", alert.Status.ShowOutput.Action)
	assert.Equal(t, "MY_DB", alert.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", alert.Status.SchemaName)
}

// --------------------------------------------------------------------------
// Tests: Drift detection
// --------------------------------------------------------------------------

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.DatabaseName = "MY_DB"
		alert.Status.SchemaName = "MY_SCHEMA"

		obs := successfulObservation()
		result := detectDrift(alert, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("WithDrift", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.DatabaseName = "MY_DB"
		alert.Status.SchemaName = "MY_SCHEMA"
		alert.Spec.Comment = testutil.PtrString("desired-comment")

		obs := successfulObservation()
		obs.ShowOutput.Comment = "actual-comment"
		result := detectDrift(alert, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("ConditionDrift", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.DatabaseName = "MY_DB"
		alert.Status.SchemaName = "MY_SCHEMA"

		obs := successfulObservation()
		obs.ShowOutput.Condition = "SELECT 1 FROM different_table"
		result := detectDrift(alert, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("ActionDrift", func(t *testing.T) {
		alert := newTestAlert("myalert", "default")
		alert.Status.DatabaseName = "MY_DB"
		alert.Status.SchemaName = "MY_SCHEMA"

		obs := successfulObservation()
		obs.ShowOutput.Action = "CALL different_procedure()"
		result := detectDrift(alert, obs)
		assert.True(t, result.HasDrift)
	})
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission(t *testing.T) {
	t.Parallel()

	alert := newTestAlert("myalert", "default")
	alert.Finalizers = []string{finalizerName}
	alert.Status.DatabaseName = "MY_DB"
	alert.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
				call++
				if call == 1 {
					return &snowflake.AlertObservation{Exists: false}, nil
				}
				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateAlertOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, alert, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myalert", "default"))
	require.NoError(t, err)

	rec := r.Recorder.(*record.FakeRecorder)

	select {
	case event := <-rec.Events:
		assert.Contains(t, event, "Creating")
	default:
		t.Error("expected a Creating event")
	}
}
