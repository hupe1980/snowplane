package tag

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateTagOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterTagOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.TagObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateTagOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterTagOptions) error {
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

func newTestTag(name, namespace string) *snowplanev1alpha1.Tag {
	return &snowplanev1alpha1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.TagSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:         "COST_CENTER",
			DatabaseName: testutil.PtrString("MY_DB"),
			SchemaName:   testutil.PtrString("MY_SCHEMA"),
		},
	}
}

func successfulObservation() *snowflake.TagObservation {
	return &snowflake.TagObservation{
		Exists: true,
		ShowOutput: &snowflake.TagShowOutput{
			CreatedOn:    "2024-01-01",
			Name:         "COST_CENTER",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Owner:        "SYSADMIN",
			Comment:      "",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Tag{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Tag, Service, *snowflake.TagObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("Tag"),
	}
}

func TestReconcile_CRNotFound(t *testing.T) {
	t.Parallel()

	r := newTestReconciler(&mockService{})

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("gone", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	r := newTestReconciler(&mockService{}, tag)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	r := newTestReconciler(&mockService{}, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.Tag{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mytag", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"

	var capturedOpts snowflake.CreateTagOptions

	obs := successfulObservation()

	callCount := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			callCount++
			if callCount == 1 {
				return &snowflake.TagObservation{Exists: false}, nil
			}

			return obs, nil
		},
		createFn: func(_ context.Context, opts snowflake.CreateTagOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
	assert.Equal(t, "COST_CENTER", capturedOpts.Name.Name())

	got := &snowplanev1alpha1.Tag{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mytag", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateWithAllowedValues(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"
	tag.Spec.AllowedValues = []string{"engineering", "finance"}
	tag.Spec.Comment = testutil.PtrString("department tag")

	var capturedOpts snowflake.CreateTagOptions

	obs := successfulObservation()
	obs.ShowOutput.Comment = "department tag"
	obs.ShowOutput.AllowedValues = "engineering,finance"

	callCount := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			callCount++
			if callCount == 1 {
				return &snowflake.TagObservation{Exists: false}, nil
			}

			return obs, nil
		},
		createFn: func(_ context.Context, opts snowflake.CreateTagOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, []string{"engineering", "finance"}, capturedOpts.AllowedValues)
	assert.Equal(t, "department tag", *capturedOpts.Comment)
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			return &snowflake.TagObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateTagOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.ObservedGeneration = 1
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)
}

func TestReconcile_UpdateWithChanges(t *testing.T) {
	t.Parallel()

	// Tag supports CreateOrAlter (defaults to true), so updates use
	// the Create path with CREATE OR ALTER semantics, not Alter.
	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.ObservedGeneration = 1
	tag.Generation = 2
	tag.Spec.Comment = testutil.PtrString("updated")
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old"

	var createCalled bool
	var capturedCreateOpts snowflake.CreateTagOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			return obs, nil
		},
		createFn: func(_ context.Context, opts snowflake.CreateTagOptions) error {
			createCalled = true
			capturedCreateOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.True(t, createCalled, "expected CREATE OR ALTER path for tag updates")
	assert.Equal(t, "updated", *capturedCreateOpts.Comment)
}

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"
	now := metav1.Now()
	tag.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "COST_CENTER", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.Tag{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mytag", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"
	tag.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	tag.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Spec.AllowedValues = []string{"a", "b"}
	tag.Spec.Comment = testutil.PtrString("test")
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "COST_CENTER")

	opts := buildCreateOptions(tag, id)
	assert.Equal(t, "COST_CENTER", opts.Name.Name())
	assert.Equal(t, []string{"a", "b"}, opts.AllowedValues)
	assert.Equal(t, "test", *opts.Comment)
}

func TestBuildAlterOptions_AllowedValuesChanged(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Spec.AllowedValues = []string{"c", "a", "b"}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "COST_CENTER")
	obs := successfulObservation()
	obs.ShowOutput.AllowedValues = "d,e"

	opts := buildAlterOptions(tag, id, obs)
	assert.True(t, opts.HasChanges())
	assert.NotNil(t, opts.AllowedValues)
}

func TestBuildAlterOptions_AllowedValuesSame(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Spec.AllowedValues = []string{"b", "a"}
	id := snowflake.NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "COST_CENTER")
	obs := successfulObservation()
	obs.ShowOutput.AllowedValues = "a,b"

	opts := buildAlterOptions(tag, id, obs)
	assert.Nil(t, opts.AllowedValues, "should not alter when sorted values match")
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.TagSpec{
		Comment:       testutil.PtrString("x"),
		AllowedValues: []string{"a"},
	}

	fields := computeTrackedParameters(spec)
	assert.ElementsMatch(t, []string{"COMMENT", "ALLOWED_VALUES"}, fields)
}

func TestComputeTrackedParameters_Empty(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.TagSpec{}
	fields := computeTrackedParameters(spec)
	assert.Empty(t, fields)
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	obs := successfulObservation()

	applyObservation(tag, obs)

	assert.NotEmpty(t, tag.Status.FullyQualifiedName)
	assert.Equal(t, "MY_DB", tag.Status.DatabaseName)
	assert.Equal(t, "MY_SCHEMA", tag.Status.SchemaName)
	assert.Equal(t, "COST_CENTER", tag.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", tag.Status.ShowOutput.Owner)
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	tag := &snowplanev1alpha1.Tag{
		Spec: snowplanev1alpha1.TagSpec{
			Name:          "COST_CENTER",
			AllowedValues: []string{"a", "b"},
			Comment:       testutil.PtrString("test"),
		},
		Status: snowplanev1alpha1.TagStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.TagObservation{
		ShowOutput: &snowflake.TagShowOutput{
			Name:          "COST_CENTER",
			DatabaseName:  "MY_DB",
			SchemaName:    "MY_SCHEMA",
			Comment:       "test",
			AllowedValues: "a,b",
		},
	}

	result := detectDrift(tag, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	tag := &snowplanev1alpha1.Tag{
		Spec: snowplanev1alpha1.TagSpec{
			Name:    "COST_CENTER",
			Comment: testutil.PtrString("desired"),
		},
		Status: snowplanev1alpha1.TagStatus{
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
		},
	}

	obs := &snowflake.TagObservation{
		ShowOutput: &snowflake.TagShowOutput{
			Name:         "COST_CENTER",
			DatabaseName: "MY_DB",
			SchemaName:   "MY_SCHEMA",
			Comment:      "drifted",
		},
	}

	result := detectDrift(tag, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	tag := newTestTag("mytag", "default")
	tag.Finalizers = []string{finalizerName}
	tag.Status.DatabaseName = "MY_DB"
	tag.Status.SchemaName = "MY_SCHEMA"

	obs := successfulObservation()

	callCount := 0
	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
			callCount++
			if callCount == 1 {
				return &snowflake.TagObservation{Exists: false}, nil
			}

			return obs, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateTagOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, tag, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mytag", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
