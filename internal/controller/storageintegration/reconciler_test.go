package storageintegration

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

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.StorageIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateStorageIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterStorageIntegrationOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestStorageIntegration(name, namespace string) *snowplanev1alpha1.StorageIntegration {
	roleARN := "arn:aws:iam::123456789012:role/myrole"

	return &snowplanev1alpha1.StorageIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.StorageIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                    "MY_INT",
			Type:                    snowplanev1alpha1.StorageIntegrationTypeExternalStage,
			StorageProvider:         "S3",
			StorageAWSRoleARN:       &roleARN,
			StorageAllowedLocations: []string{"s3://mybucket/"},
		},
	}
}

func successfulObservation() *snowflake.StorageIntegrationObservation {
	return &snowflake.StorageIntegrationObservation{
		Exists: true,
		ShowOutput: &snowflake.StorageIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      "MY_INT",
			Type:      "EXTERNAL_STAGE",
			Category:  "STORAGE",
			Enabled:   true,
			Comment:   "",
		},
		DescribeOutput: map[string]string{
			"STORAGE_AWS_IAM_USER_ARN":  "arn:aws:iam::000:user/abc",
			"STORAGE_AWS_EXTERNAL_ID":   "ext-id-123",
			"STORAGE_ALLOWED_LOCATIONS": "s3://mybucket/",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.StorageIntegration{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			newService: func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("StorageIntegration"),
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
// Tests: Finalizer management
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	r := newTestReconciler(&mockService{}, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, time.Second, result.RequeueAfter)

	got := &snowplanev1alpha1.StorageIntegration{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.Contains(t, got.Finalizers, finalizerName)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateStorageIntegrationOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StorageIntegrationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateStorageIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_INT", capturedOpts.Name.Name())
	assert.Equal(t, "EXTERNAL_STAGE", capturedOpts.Type)
	assert.Equal(t, "S3", capturedOpts.StorageProvider)

	got := &snowplanev1alpha1.StorageIntegration{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			return &snowflake.StorageIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateStorageIntegrationOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	id := snowflake.NewAccountObjectIdentifier("MY_INT")

	opts := buildCreateOptions(s, id)
	assert.Equal(t, "MY_INT", opts.Name.Name())
	assert.Equal(t, "EXTERNAL_STAGE", opts.Type)
	assert.Equal(t, "S3", opts.StorageProvider)
	assert.Equal(t, []string{"s3://mybucket/"}, opts.StorageAllowedLocations)
	assert.NotNil(t, opts.StorageAWSRoleARN)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.StorageIntegrationSpec{}
		assert.Empty(t, computeTrackedParameters(spec))
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "test"
		spec := &snowplanev1alpha1.StorageIntegrationSpec{Comment: &c}
		assert.Contains(t, computeTrackedParameters(spec), "COMMENT")
	})

	t.Run("EnabledSet", func(t *testing.T) {
		t.Parallel()
		e := true
		spec := &snowplanev1alpha1.StorageIntegrationSpec{Enabled: &e}
		assert.Contains(t, computeTrackedParameters(spec), "ENABLED")
	})
}

func TestSortedLocations(t *testing.T) {
	t.Parallel()

	t.Run("AlreadySorted", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "a,b,c", sortedLocations([]string{"a", "b", "c"}))
	})

	t.Run("Unsorted", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "a,b,c", sortedLocations([]string{"c", "a", "b"}))
	})

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", sortedLocations(nil))
	})
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.StorageIntegration{
		Spec: snowplanev1alpha1.StorageIntegrationSpec{
			Name:                    "MY_INT",
			StorageAllowedLocations: []string{"s3://mybucket/"},
		},
	}

	obs := &snowflake.StorageIntegrationObservation{
		ShowOutput: &snowflake.StorageIntegrationShowOutput{
			Name: "MY_INT",
		},
		DescribeOutput: map[string]string{
			"STORAGE_ALLOWED_LOCATIONS": "s3://mybucket/",
		},
	}

	result := detectDrift(s, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	s := &snowplanev1alpha1.StorageIntegration{
		Spec: snowplanev1alpha1.StorageIntegrationSpec{
			Name:    "MY_INT",
			Comment: testutil.PtrString("desired"),
		},
	}

	obs := &snowflake.StorageIntegrationObservation{
		ShowOutput: &snowflake.StorageIntegrationShowOutput{
			Name:    "MY_INT",
			Comment: "drifted",
		},
		DescribeOutput: map[string]string{},
	}

	result := detectDrift(s, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}

// --------------------------------------------------------------------------
// Tests: ProviderConfig resolution
// --------------------------------------------------------------------------

func TestReconcile_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	r := newTestReconciler(&mockService{}, s)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")
}

// --------------------------------------------------------------------------
// Tests: Create terminal error
// --------------------------------------------------------------------------

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			return &snowflake.StorageIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateStorageIntegrationOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Update flow
// --------------------------------------------------------------------------

func TestReconcile_UpdateNoChanges(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.ObservedGeneration = 1

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			return obs, nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.StorageIntegration{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_UpdateComment(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Status.ObservedGeneration = 1
	s.Generation = 2
	s.Spec.Comment = testutil.PtrString("new comment")

	obs := successfulObservation()
	obs.ShowOutput.Comment = "old comment"

	var capturedAlterOpts snowflake.AlterStorageIntegrationOptions

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			return obs, nil
		},
		alterFn: func(_ context.Context, opts snowflake.AlterStorageIntegrationOptions) error {
			capturedAlterOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)

	assert.NotNil(t, capturedAlterOpts.Comment)
	assert.Equal(t, "new comment", *capturedAlterOpts.Comment)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}
	now := metav1.Now()
	s.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_INT", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.StorageIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "mys", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}
	s.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	s.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: ApplyObservation
// --------------------------------------------------------------------------

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	obs := successfulObservation()

	applyObservation(s, obs)

	assert.Equal(t, "MY_INT", s.Status.FullyQualifiedName)
	assert.Equal(t, "MY_INT", s.Status.ShowOutput.Name)
	assert.Equal(t, "EXTERNAL_STAGE", s.Status.ShowOutput.Type)
	assert.Equal(t, "STORAGE", s.Status.ShowOutput.Category)
	assert.True(t, s.Status.ShowOutput.Enabled)
	assert.Equal(t, "arn:aws:iam::000:user/abc", s.Status.StorageAWSIAMUserARN)
	assert.Equal(t, "ext-id-123", s.Status.StorageAWSExternalID)
}

// --------------------------------------------------------------------------
// Tests: Event emission
// --------------------------------------------------------------------------

func TestReconcile_EventEmission_Create(t *testing.T) {
	t.Parallel()

	s := newTestStorageIntegration("mys", "default")
	s.Finalizers = []string{finalizerName}

	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.StorageIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.StorageIntegrationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, _ snowflake.CreateStorageIntegrationOptions) error {
			return nil
		},
	}

	r := newTestReconciler(mock, s, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))
	rec := r.Recorder.(*record.FakeRecorder)

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("mys", "default"))
	require.NoError(t, err)

	events := testutil.DrainEvents(rec)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], "Normal")
	assert.Contains(t, events[0], "Creating")
}
