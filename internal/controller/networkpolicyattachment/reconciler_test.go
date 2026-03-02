package networkpolicyattachment

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

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}

	return &snowflake.NetworkPolicyAttachmentObservation{Exists: false}, nil
}

func (m *mockService) SetNetworkPolicy(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error {
	if m.setFn != nil {
		return m.setFn(ctx, opts)
	}

	return nil
}

func (m *mockService) UnsetNetworkPolicy(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error {
	if m.unsetFn != nil {
		return m.unsetFn(ctx, opts)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func ptrStr(s string) *string { return &s }

func newTestNetworkPolicyAttachment(name, namespace string) *snowplanev1alpha1.NetworkPolicyAttachment {
	return &snowplanev1alpha1.NetworkPolicyAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.NetworkPolicyAttachmentSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			TargetType: "ACCOUNT",
			PolicyName: ptrStr("MY_NETWORK_POLICY"),
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.NetworkPolicyAttachment{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("NetworkPolicyAttachment"),
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
			return newTestNetworkPolicyAttachment(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.NetworkPolicyAttachment{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}
