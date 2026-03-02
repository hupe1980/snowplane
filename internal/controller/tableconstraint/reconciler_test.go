package tableconstraint

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
	observeFn func(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error)
	addFn     func(ctx context.Context, opts snowflake.AddConstraintOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterConstraintOptions) error
	dropFn    func(ctx context.Context, id snowflake.TableConstraintIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, id, constraintType)
	}

	return &snowflake.TableConstraintObservation{Exists: false}, nil
}

func (m *mockService) AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error {
	if m.addFn != nil {
		return m.addFn(ctx, opts)
	}

	return nil
}

func (m *mockService) AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}

	return nil
}

func (m *mockService) DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, id)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestTableConstraint(name, namespace string) *snowplanev1alpha1.TableConstraint {
	return &snowplanev1alpha1.TableConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.TableConstraintSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:      "pk_orders",
			Type:      snowplanev1alpha1.ConstraintTypePrimaryKey,
			TableName: `"DB"."SCHEMA"."ORDERS"`,
			Columns:   []string{"ORDER_ID"},
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	scheme := testutil.TestScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.TableConstraint{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation]{
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
		GVK: snowplanev1alpha1.GroupVersion.WithKind("TableConstraint"),
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
			return newTestTableConstraint(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.TableConstraint{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}
