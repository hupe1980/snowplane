package queuenotificationintegration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.QueueNotificationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateQueueNotificationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterQueueNotificationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.QueueNotificationIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.QueueNotificationIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateQueueNotificationIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterQueueNotificationIntegrationOptions) error {
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

func newTestQueueNotificationIntegration(name, ns string) *snowplanev1alpha1.QueueNotificationIntegration {
	return &snowplanev1alpha1.QueueNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.QueueNotificationIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:                 "MY_QNI",
			NotificationProvider: "AWS_SNS",
			Direction:            "OUTBOUND",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.QueueNotificationIntegration, Service, *snowflake.QueueNotificationIntegrationObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.QueueNotificationIntegration{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("QueueNotificationIntegration")

	return r
}

func TestReconcile_StandardSuite(t *testing.T) {
	t.Parallel()

	testutil.ReconcileSuiteConfig{
		NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
			r := newTestReconciler(&mockService{}, objs...)
			return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
		},
		NewFixture: func(name, ns string) client.Object {
			return newTestQueueNotificationIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.QueueNotificationIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func successfulObservation() *snowflake.QueueNotificationIntegrationObservation {
	return &snowflake.QueueNotificationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.QueueNotificationIntegrationShowOutput{
			Name:    "MY_QNI",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"NOTIFICATION_PROVIDER":           "AWS_SNS",
			"DIRECTION":                       "OUTBOUND",
			"AWS_SNS_TOPIC_ARN":               "",
			"AWS_SNS_ROLE_ARN":                "",
			"AWS_SQS_ARN":                     "",
			"AWS_SQS_ROLE_ARN":                "",
			"GCP_PUBSUB_TOPIC_NAME":           "",
			"GCP_PUBSUB_SUBSCRIPTION_NAME":    "",
			"AZURE_STORAGE_QUEUE_PRIMARY_URI": "",
			"AZURE_TENANT_ID":                 "",
			"AZURE_EVENT_GRID_TOPIC_ENDPOINT": "",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("CloudFieldsSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		topicARN := "arn:aws:sns:us-east-1:123456789012:my-topic"
		roleARN := "arn:aws:iam::123456789012:role/my-role"

		obj := newTestQueueNotificationIntegration("x", "default")
		obj.Spec.AWSSNSTopicARN = &topicARN
		obj.Spec.AWSSNSRoleARN = &roleARN

		obs := successfulObservation()
		obs.DescribeOutput["AWS_SNS_TOPIC_ARN"] = topicARN
		obs.DescribeOutput["AWS_SNS_ROLE_ARN"] = roleARN
		id := snowflake.NewAccountObjectIdentifier("MY_QNI")

		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.AWSSNSTopicARN, "topic ARN should be skipped when unchanged")
		assert.Nil(t, opts.AWSSNSRoleARN, "role ARN should be skipped when unchanged")
		assert.Nil(t, opts.Direction, "direction should be skipped when unchanged")
		assert.Nil(t, opts.NotificationProvider, "immutable provider should not be sent in ALTER")
	})

	t.Run("CloudFieldsSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		topicARN := "arn:aws:sns:us-east-1:123456789012:new-topic"
		roleARN := "arn:aws:iam::123456789012:role/new-role"

		obj := newTestQueueNotificationIntegration("x", "default")
		obj.Spec.AWSSNSTopicARN = &topicARN
		obj.Spec.AWSSNSRoleARN = &roleARN

		obs := successfulObservation()
		obs.DescribeOutput["AWS_SNS_TOPIC_ARN"] = "arn:aws:sns:us-east-1:123456789012:old-topic"
		obs.DescribeOutput["AWS_SNS_ROLE_ARN"] = "arn:aws:iam::123456789012:role/old-role"
		id := snowflake.NewAccountObjectIdentifier("MY_QNI")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.AWSSNSTopicARN)
		assert.Equal(t, topicARN, *opts.AWSSNSTopicARN)
		require.NotNil(t, opts.AWSSNSRoleARN)
		assert.Equal(t, roleARN, *opts.AWSSNSRoleARN)
	})

	t.Run("DirectionSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestQueueNotificationIntegration("x", "default")
		obj.Spec.Direction = "INBOUND"

		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_QNI")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.Direction)
		assert.Equal(t, "INBOUND", *opts.Direction)
	})

	t.Run("AllFieldsSentWhenNoObservation", func(t *testing.T) {
		t.Parallel()

		topicARN := "arn:aws:sns:us-east-1:123456789012:my-topic"
		obj := newTestQueueNotificationIntegration("x", "default")
		obj.Spec.AWSSNSTopicARN = &topicARN

		id := snowflake.NewAccountObjectIdentifier("MY_QNI")
		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.Direction, "direction should be sent when no observation")
		require.NotNil(t, opts.AWSSNSTopicARN, "cloud field should be sent when no observation")
	})
}

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestQueueNotificationIntegration("x", "default")
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("DirectionDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestQueueNotificationIntegration("x", "default")
		obs := successfulObservation()
		obs.DescribeOutput["DIRECTION"] = "INBOUND"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})

	t.Run("CloudFieldDrift", func(t *testing.T) {
		t.Parallel()

		topicARN := "arn:aws:sns:us-east-1:123456789012:my-topic"
		obj := newTestQueueNotificationIntegration("x", "default")
		obj.Spec.AWSSNSTopicARN = &topicARN

		obs := successfulObservation()
		obs.DescribeOutput["AWS_SNS_TOPIC_ARN"] = "arn:aws:sns:us-east-1:123456789012:other-topic"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})
}
