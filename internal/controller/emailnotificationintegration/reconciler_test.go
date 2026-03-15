package emailnotificationintegration

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

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.EmailNotificationIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateEmailNotificationIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterEmailNotificationIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.EmailNotificationIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.EmailNotificationIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateEmailNotificationIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterEmailNotificationIntegrationOptions) error {
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

func newTestEmailNotificationIntegration(name, ns string) *snowplanev1alpha1.EmailNotificationIntegration {
	return &snowplanev1alpha1.EmailNotificationIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.EmailNotificationIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:              "MY_EMAIL_NI",
			AllowedRecipients: []string{"user@example.com"},
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.EmailNotificationIntegration{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("EmailNotificationIntegration")

	return r
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

func TestReconcile_StandardSuite(t *testing.T) {
	t.Parallel()

	testutil.ReconcileSuiteConfig{
		NewReconciler: func(objs ...runtime.Object) testutil.ReconcilerSetup {
			r := newTestReconciler(&mockService{}, objs...)
			return testutil.ReconcilerSetup{Reconciler: r, Client: r.Client}
		},
		NewFixture: func(name, ns string) client.Object {
			return newTestEmailNotificationIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.EmailNotificationIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func successfulObservation() *snowflake.EmailNotificationIntegrationObservation {
	return &snowflake.EmailNotificationIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.EmailNotificationIntegrationShowOutput{
			Name:    "MY_EMAIL_NI",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"ALLOWED_RECIPIENTS": "user@example.com",
			"DEFAULT_RECIPIENTS": "",
			"DEFAULT_SUBJECT":    "",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("AllowedRecipientsSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestEmailNotificationIntegration("x", "default")
		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.AllowedRecipients, "should be skipped when unchanged")
	})

	t.Run("AllowedRecipientsSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestEmailNotificationIntegration("x", "default")
		obj.Spec.AllowedRecipients = []string{"new@example.com", "other@example.com"}
		obs := successfulObservation()
		id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.AllowedRecipients)
		assert.ElementsMatch(t, []string{"new@example.com", "other@example.com"}, *opts.AllowedRecipients)
	})

	t.Run("DefaultSubjectSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		subj := "Alert"
		obj := newTestEmailNotificationIntegration("x", "default")
		obj.Spec.DefaultSubject = &subj

		obs := successfulObservation()
		obs.DescribeOutput["DEFAULT_SUBJECT"] = "Alert"
		id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

		opts := buildAlterOptions(obj, id, obs)

		assert.Nil(t, opts.DefaultSubject, "should be skipped when unchanged")
	})

	t.Run("DefaultSubjectSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		subj := "New Alert"
		obj := newTestEmailNotificationIntegration("x", "default")
		obj.Spec.DefaultSubject = &subj

		obs := successfulObservation()
		obs.DescribeOutput["DEFAULT_SUBJECT"] = "Old Alert"
		id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

		opts := buildAlterOptions(obj, id, obs)

		require.NotNil(t, opts.DefaultSubject)
		assert.Equal(t, "New Alert", *opts.DefaultSubject)
	})

	t.Run("AllFieldsSentWhenNoObservation", func(t *testing.T) {
		t.Parallel()

		subj := "Test"
		obj := newTestEmailNotificationIntegration("x", "default")
		obj.Spec.DefaultSubject = &subj
		id := snowflake.NewAccountObjectIdentifier("MY_EMAIL_NI")

		opts := buildAlterOptions(obj, id, nil)

		require.NotNil(t, opts.AllowedRecipients, "should be sent when no observation")
		require.NotNil(t, opts.DefaultSubject, "should be sent when no observation")
	})
}
