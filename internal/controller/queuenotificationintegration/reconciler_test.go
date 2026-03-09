package queuenotificationintegration

import (
	"context"
	"testing"

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
