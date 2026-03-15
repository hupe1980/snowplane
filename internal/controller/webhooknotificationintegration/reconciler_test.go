package webhooknotificationintegration

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WebhookNotificationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateWebhookNotificationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterWebhookNotificationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WebhookNotificationIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.WebhookNotificationIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateWebhookNotificationIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterWebhookNotificationIntegrationOptions) error {
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

func newTestWebhookNotificationIntegration(name, ns string) *snowplanev1alpha1.WebhookNotificationIntegration {
	return &snowplanev1alpha1.WebhookNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.WebhookNotificationIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:       "MY_WEBHOOK",
			WebhookURL: "https://example.com/hook",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.WebhookNotificationIntegration, Service, *snowflake.WebhookNotificationIntegrationObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.WebhookNotificationIntegration{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("WebhookNotificationIntegration")

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
			return newTestWebhookNotificationIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.WebhookNotificationIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func successfulObservation() *snowflake.WebhookNotificationIntegrationObservation {
	return &snowflake.WebhookNotificationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.WebhookNotificationIntegrationShowOutput{
			Name:    "MY_WEBHOOK",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"WEBHOOK_URL":           "https://example.com/hook",
			"WEBHOOK_BODY_TEMPLATE": "",
			"WEBHOOK_SECRET":        "****",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("WebhookURLSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestWebhookNotificationIntegration("x", "default")
		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.WebhookURL, "URL should be skipped when unchanged")
	})

	t.Run("WebhookURLSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestWebhookNotificationIntegration("x", "default")
		obj.Spec.WebhookURL = "https://example.com/new-hook"
		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.WebhookURL)
		assert.Equal(t, "https://example.com/new-hook", *opts.WebhookURL)
	})

	t.Run("BodyTemplateSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		tmpl := "my-template"
		obj := newTestWebhookNotificationIntegration("x", "default")
		obj.Spec.WebhookBodyTemplate = &tmpl

		obs := successfulObservation()
		obs.DescribeOutput["WEBHOOK_BODY_TEMPLATE"] = "my-template"
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.WebhookBodyTemplate, "body template should be skipped when unchanged")
	})

	t.Run("BodyTemplateSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		tmpl := "new-template"
		obj := newTestWebhookNotificationIntegration("x", "default")
		obj.Spec.WebhookBodyTemplate = &tmpl

		obs := successfulObservation()
		obs.DescribeOutput["WEBHOOK_BODY_TEMPLATE"] = "old-template"
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.WebhookBodyTemplate)
		assert.Equal(t, "new-template", *opts.WebhookBodyTemplate)
	})

	t.Run("SecretAlwaysSent", func(t *testing.T) {
		t.Parallel()

		secret := "my-secret"
		obj := newTestWebhookNotificationIntegration("x", "default")
		obj.Spec.WebhookSecret = &secret
		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.WebhookSecret, "secret should always be sent (masked in DESCRIBE)")
		assert.Equal(t, "my-secret", *opts.WebhookSecret)
	})

	t.Run("AllFieldsSentWhenNoObservation", func(t *testing.T) {
		t.Parallel()

		obj := newTestWebhookNotificationIntegration("x", "default")
		id := snowflake.NewAccountObjectIdentifier("MY_WEBHOOK")

		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.WebhookURL, "URL should be sent when no observation")
	})
}

func TestDetectDrift(t *testing.T) {
	t.Parallel()

	t.Run("NoDrift", func(t *testing.T) {
		t.Parallel()

		obj := newTestWebhookNotificationIntegration("x", "default")
		obs := successfulObservation()

		result := detectDrift(obj, obs)
		assert.False(t, result.HasDrift)
	})

	t.Run("BodyTemplateDrift", func(t *testing.T) {
		t.Parallel()

		tmpl := "my-template"
		obj := newTestWebhookNotificationIntegration("x", "default")
		obj.Spec.WebhookBodyTemplate = &tmpl
		obs := successfulObservation()
		obs.DescribeOutput["WEBHOOK_BODY_TEMPLATE"] = "other-template"

		result := detectDrift(obj, obs)
		assert.True(t, result.HasDrift)
	})
}
