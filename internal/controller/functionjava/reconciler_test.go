package functionjava

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
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// --------------------------------------------------------------------------
// Mock service
// --------------------------------------------------------------------------

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateFunctionOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterFunctionOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) (*snowflake.FunctionObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name, argTypes)
	}

	return &snowflake.FunctionObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateFunctionOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterFunctionOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}

	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier, argTypes []string) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name, argTypes)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newTestCR(name, namespace string) *snowplanev1alpha1.FunctionJava {
	return &snowplanev1alpha1.FunctionJava{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.FunctionJavaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:            "MY_FUNC",
			Returns:         "VARCHAR",
			Handler:         "com.example.Handler.run",
			RuntimeVersion:  "11",
			SnowparkPackage: "1.14.0",
			DatabaseRef:     &snowplanev1alpha1.LocalObjectReference{Name: "analytics-db"},
			SchemaRef:       &snowplanev1alpha1.LocalObjectReference{Name: "public-schema"},
		},
	}
}

func newTestDB(name, namespace string) *snowplanev1alpha1.Database {
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "ANALYTICS",
		},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"ANALYTICS"`,
				ObservedGeneration: 1,
			},
		},
	}
	conditions.SetReady(db, "ok")

	return db
}

func newTestSchema(name, namespace string) *snowplanev1alpha1.Schema {
	s := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "PUBLIC",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "analytics-db"},
		},
		Status: snowplanev1alpha1.SchemaStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{
				FullyQualifiedName: `"ANALYTICS"."PUBLIC"`,
				ObservedGeneration: 1,
			},
			DatabaseName: `"ANALYTICS"`,
		},
	}
	conditions.SetReady(s, "ok")

	return s
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.FunctionJava{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
			&snowplanev1alpha1.ProviderConfig{},
		)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	rec := record.NewFakeRecorder(100)

	return &reconciler.GenericReconciler[*snowplanev1alpha1.FunctionJava, Service, *snowflake.FunctionObservation]{
		Client:   c,
		Factory:  factory,
		Recorder: rec,
		Adapter: &adapter{
			client:   c,
			recorder: rec,
			newService: func(_ context.Context, _ clientfactory.SnowflakeClient, _ string) (Service, func(context.Context), error) {
				return mock, nil, nil
			},
		},
		GVK: snowplanev1alpha1.GroupVersion.WithKind("FunctionJava"),
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
			return newTestCR(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.FunctionJava{}
		},
		FinalizerName: finalizerName,
		PrereqObjects: func() []runtime.Object {
			db := newTestDB("analytics-db", "default")
			sch := newTestSchema("public-schema", "default")
			return []runtime.Object{db, sch}
		},
	}.Run(t)
}
