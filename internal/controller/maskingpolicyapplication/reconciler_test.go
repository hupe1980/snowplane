package maskingpolicyapplication

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error)
	setFn     func(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error
	unsetFn   func(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id)
	}

	return &snowflake.MaskingPolicyApplicationObservation{Exists: false}, nil
}

func (m *mockService) SetMaskingPolicy(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error {
	if m.setFn != nil {
		return m.setFn(ctx, opts)
	}

	return nil
}

func (m *mockService) UnsetMaskingPolicy(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error {
	if m.unsetFn != nil {
		return m.unsetFn(ctx, opts)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestMaskingPolicyApplication(name, namespace string) *snowplanev1alpha1.MaskingPolicyApplication {
	return &snowplanev1alpha1.MaskingPolicyApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.MaskingPolicyApplicationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			TableName:  `"DB"."SCHEMA"."MY_TABLE"`,
			ColumnName: "EMAIL",
			PolicyName: testutil.Ptr(`"DB"."SCHEMA"."MASK_POLICY"`),
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.MaskingPolicyApplication{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := testutil.NewTestClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: newAdapter(c, rec, func(_ context.Context, _ SnowflakeClient, _ string) (Service, func(context.Context), error) {
			return mock, nil, nil
		}),
		GVK: snowplanev1alpha1.GroupVersion.WithKind("MaskingPolicyApplication"),
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
			return newTestMaskingPolicyApplication(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.MaskingPolicyApplication{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}
