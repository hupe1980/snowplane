package notificationintegration

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}

	return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error {
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

func newTestNotificationIntegration(name, namespace string) *snowplanev1alpha1.NotificationIntegration {
	return &snowplanev1alpha1.NotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.NotificationIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "MY_EMAIL_NI",
			Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
			Email: &snowplanev1alpha1.EmailNotificationConfig{
				AllowedRecipients: []string{"admin@example.com"},
			},
		},
	}
}

func successfulObservation() *snowflake.NotificationIntegrationObservation {
	return &snowflake.NotificationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
			CreatedOn: "2024-01-01",
			Name:      "MY_EMAIL_NI",
			Type:      "EMAIL",
			Category:  "NOTIFICATION",
			Enabled:   true,
		},
		DescribeOutput: map[string]string{
			"ALLOWED_RECIPIENTS": "admin@example.com",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.NotificationIntegration{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	r := NewReconcilerWithServiceFactory(c, factory, rec, nil,
		func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		},
	)
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("NotificationIntegration")

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
			return newTestNotificationIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.NotificationIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

// --------------------------------------------------------------------------
// Tests: Create flow
// --------------------------------------------------------------------------

func TestReconcile_Create(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	ni.Finalizers = []string{finalizerName}

	var capturedOpts snowflake.CreateNotificationIntegrationOptions
	obs := successfulObservation()

	mock := &mockService{
		observeFn: func() func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
			call := 0
			return func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
				call++
				if call == 1 {
					return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
				}

				return obs, nil
			}
		}(),
		createFn: func(_ context.Context, opts snowflake.CreateNotificationIntegrationOptions) error {
			capturedOpts = opts
			return nil
		},
	}

	r := newTestReconciler(mock, ni, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myni", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter)

	assert.Equal(t, "MY_EMAIL_NI", capturedOpts.Name.Name())
	assert.Equal(t, "EMAIL", capturedOpts.Type)
	assert.Equal(t, []string{"admin@example.com"}, capturedOpts.AllowedRecipients)

	got := &snowplanev1alpha1.NotificationIntegration{}
	require.NoError(t, r.Client.Get(context.Background(), types.NamespacedName{Name: "myni", Namespace: "default"}, got))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_CreateFails(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	ni.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
			return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNotificationIntegrationOptions) error {
			return fmt.Errorf("permission denied")
		},
	}

	r := newTestReconciler(mock, ni, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myni", "default"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestReconcile_CreateTerminalError(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	ni.Finalizers = []string{finalizerName}

	mock := &mockService{
		observeFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
			return &snowflake.NotificationIntegrationObservation{Exists: false}, nil
		},
		createFn: func(_ context.Context, _ snowflake.CreateNotificationIntegrationOptions) error {
			return snowflake.NewTerminalError(fmt.Errorf("invalid"))
		},
	}

	r := newTestReconciler(mock, ni, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	_, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myni", "default"))
	require.NoError(t, err)
}

// --------------------------------------------------------------------------
// Tests: Delete flow
// --------------------------------------------------------------------------

func TestReconcile_Delete(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	ni.Finalizers = []string{finalizerName}
	now := metav1.Now()
	ni.DeletionTimestamp = &now

	var dropCalled bool

	mock := &mockService{
		dropFn: func(_ context.Context, name snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			assert.Equal(t, "MY_EMAIL_NI", name.Name())
			return nil
		},
	}

	r := newTestReconciler(mock, ni, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myni", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, dropCalled)

	got := &snowplanev1alpha1.NotificationIntegration{}
	err = r.Client.Get(context.Background(), types.NamespacedName{Name: "myni", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcile_DeleteOrphanPolicy(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	ni.Finalizers = []string{finalizerName}
	ni.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	now := metav1.Now()
	ni.DeletionTimestamp = &now

	var dropCalled bool
	mock := &mockService{
		dropFn: func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
			dropCalled = true
			return nil
		},
	}

	r := newTestReconciler(mock, ni, testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("myni", "default"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, dropCalled)
}

// --------------------------------------------------------------------------
// Tests: Unit tests for helpers
// --------------------------------------------------------------------------

func TestBuildCreateOptions(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

	opts := buildCreateOptions(ni, id)
	assert.Equal(t, "MY_EMAIL_NI", opts.Name.Name())
	assert.Equal(t, "EMAIL", opts.Type)
	assert.Equal(t, []string{"admin@example.com"}, opts.AllowedRecipients)
}

func TestComputeTrackedParameters(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
			Email: &snowplanev1alpha1.EmailNotificationConfig{
				AllowedRecipients: []string{"a@b.com"},
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "ALLOWED_RECIPIENTS")
	})

	t.Run("EmailWithDefaults", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
			Email: &snowplanev1alpha1.EmailNotificationConfig{
				AllowedRecipients: []string{"a@b.com"},
				DefaultRecipients: []string{"c@d.com"},
				DefaultSubject:    testutil.Ptr("Alert"),
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "ALLOWED_RECIPIENTS")
		assert.Contains(t, fields, "DEFAULT_RECIPIENTS")
		assert.Contains(t, fields, "DEFAULT_SUBJECT")
	})

	t.Run("QueueAWSSNS", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type: snowplanev1alpha1.NotificationIntegrationTypeQueue,
			Queue: &snowplanev1alpha1.QueueNotificationConfig{
				NotificationProvider: "AWS_SNS",
				Direction:            "OUTBOUND",
				AWSSNSTopicARN:       testutil.Ptr("arn:aws:sns:us-east-1:123:topic"),
				AWSSNSRoleARN:        testutil.Ptr("arn:aws:iam::123:role/myrole"),
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "NOTIFICATION_PROVIDER")
		assert.Contains(t, fields, "AWS_SNS_TOPIC_ARN")
		assert.Contains(t, fields, "AWS_SNS_ROLE_ARN")
	})

	t.Run("WebhookBasic", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type: snowplanev1alpha1.NotificationIntegrationTypeWebhook,
			Webhook: &snowplanev1alpha1.WebhookNotificationConfig{
				WebhookURL: "https://hooks.example.com/notify",
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "WEBHOOK_URL")
	})

	t.Run("WebhookWithHeaders", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type: snowplanev1alpha1.NotificationIntegrationTypeWebhook,
			Webhook: &snowplanev1alpha1.WebhookNotificationConfig{
				WebhookURL: "https://hooks.example.com/notify",
				WebhookHeaders: map[string]string{
					"X-Zebra": "z",
					"X-Alpha": "a",
				},
			},
		}
		fields := tracked.ComputeTracked(spec)
		assert.Contains(t, fields, "WEBHOOK_HEADER_X-Alpha")
		assert.Contains(t, fields, "WEBHOOK_HEADER_X-Zebra")
		// Verify sorted order: X-Alpha before X-Zebra.
		alphaIdx := -1
		zebraIdx := -1
		for i, f := range fields {
			if f == "WEBHOOK_HEADER_X-Alpha" {
				alphaIdx = i
			}
			if f == "WEBHOOK_HEADER_X-Zebra" {
				zebraIdx = i
			}
		}
		assert.Greater(t, zebraIdx, alphaIdx, "headers should be tracked in sorted order")
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		spec := &snowplanev1alpha1.NotificationIntegrationSpec{
			Type:    snowplanev1alpha1.NotificationIntegrationTypeEmail,
			Comment: testutil.Ptr("test"),
		}
		assert.Contains(t, tracked.ComputeTracked(spec), "COMMENT")
	})
}

func TestComputeUnsetFields(t *testing.T) {
	t.Parallel()

	t.Run("NoTrackedParams", func(t *testing.T) {
		t.Parallel()
		ni := &snowplanev1alpha1.NotificationIntegration{
			Spec: snowplanev1alpha1.NotificationIntegrationSpec{
				Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
				Email: &snowplanev1alpha1.EmailNotificationConfig{
					AllowedRecipients: []string{"a@b.com"},
				},
			},
		}
		assert.Nil(t, tracked.ComputeUnset(&ni.Spec, ni.Status.TrackedParameters))
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		ni := &snowplanev1alpha1.NotificationIntegration{
			Spec: snowplanev1alpha1.NotificationIntegrationSpec{
				Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
				Email: &snowplanev1alpha1.EmailNotificationConfig{
					AllowedRecipients: []string{"a@b.com"},
				},
			},
		}
		ni.Status.TrackedParameters = []string{"COMMENT"}
		unset := tracked.ComputeUnset(&ni.Spec, ni.Status.TrackedParameters)
		assert.Contains(t, unset, "COMMENT")
	})

	t.Run("UnsetWebhookHeader", func(t *testing.T) {
		t.Parallel()
		ni := &snowplanev1alpha1.NotificationIntegration{
			Spec: snowplanev1alpha1.NotificationIntegrationSpec{
				Type: snowplanev1alpha1.NotificationIntegrationTypeWebhook,
				Webhook: &snowplanev1alpha1.WebhookNotificationConfig{
					WebhookURL: "https://example.com",
					WebhookHeaders: map[string]string{
						"X-Keep": "val",
					},
				},
			},
		}
		ni.Status.TrackedParameters = []string{"WEBHOOK_HEADER_X-Keep", "WEBHOOK_HEADER_X-Remove"}
		unset := tracked.ComputeUnset(&ni.Spec, ni.Status.TrackedParameters)
		assert.Contains(t, unset, "WEBHOOK_HEADER_X-Remove")
		assert.NotContains(t, unset, "WEBHOOK_HEADER_X-Keep")
	})

	t.Run("UnsetQueueARN", func(t *testing.T) {
		t.Parallel()
		ni := &snowplanev1alpha1.NotificationIntegration{
			Spec: snowplanev1alpha1.NotificationIntegrationSpec{
				Type: snowplanev1alpha1.NotificationIntegrationTypeQueue,
				Queue: &snowplanev1alpha1.QueueNotificationConfig{
					NotificationProvider: "AWS_SNS",
					Direction:            "OUTBOUND",
				},
			},
		}
		ni.Status.TrackedParameters = []string{"NOTIFICATION_PROVIDER", "AWS_SNS_TOPIC_ARN", "AWS_SNS_ROLE_ARN"}
		unset := tracked.ComputeUnset(&ni.Spec, ni.Status.TrackedParameters)
		assert.Contains(t, unset, "AWS_SNS_TOPIC_ARN")
		assert.Contains(t, unset, "AWS_SNS_ROLE_ARN")
	})
}

func TestApplyObservation(t *testing.T) {
	t.Parallel()

	ni := newTestNotificationIntegration("myni", "default")
	obs := successfulObservation()

	applyObservation(ni, obs)

	assert.Equal(t, "MY_EMAIL_NI", ni.Status.FullyQualifiedName)
	assert.Equal(t, "MY_EMAIL_NI", ni.Status.ShowOutput.Name)
	assert.Equal(t, "EMAIL", ni.Status.ShowOutput.Type)
	assert.Contains(t, ni.Status.DescribeOutput, "ALLOWED_RECIPIENTS")
}

func TestDetectDrift_NoDrift(t *testing.T) {
	t.Parallel()

	ni := &snowplanev1alpha1.NotificationIntegration{
		Spec: snowplanev1alpha1.NotificationIntegrationSpec{
			Name: "MY_EMAIL_NI",
			Type: snowplanev1alpha1.NotificationIntegrationTypeEmail,
		},
	}

	obs := &snowflake.NotificationIntegrationObservation{
		ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
			Name: "MY_EMAIL_NI",
			Type: "EMAIL",
		},
	}

	result := detectDrift(ni, obs)
	assert.False(t, result.HasDrift)
}

func TestDetectDrift_WithDrift(t *testing.T) {
	t.Parallel()

	ni := &snowplanev1alpha1.NotificationIntegration{
		Spec: snowplanev1alpha1.NotificationIntegrationSpec{
			Name:    "MY_EMAIL_NI",
			Type:    snowplanev1alpha1.NotificationIntegrationTypeEmail,
			Comment: testutil.Ptr("desired"),
		},
	}

	obs := &snowflake.NotificationIntegrationObservation{
		ShowOutput: &snowplanev1alpha1.NotificationIntegrationShowOutput{
			Name:    "MY_EMAIL_NI",
			Type:    "EMAIL",
			Comment: "drifted",
		},
	}

	result := detectDrift(ni, obs)
	assert.True(t, result.HasDrift)
	assert.Contains(t, result.Summary(), "COMMENT")
}
