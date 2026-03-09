package gitrepository

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

type mockService struct {
	observeFn func(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateGitRepositoryOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterGitRepositoryOptions) error
	dropFn    func(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.GitRepositoryObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.GitRepositoryObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateGitRepositoryOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterGitRepositoryOptions) error {
	if m.alterFn != nil {
		return m.alterFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error {
	if m.dropFn != nil {
		return m.dropFn(ctx, name)
	}
	return nil
}

func newTestGitRepository(name, ns string) *snowplanev1alpha1.GitRepository {
	return &snowplanev1alpha1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.GitRepositorySpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:           "MY_GIT_REPO",
			DatabaseRef:    &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
			SchemaRef:      &snowplanev1alpha1.ObjectReference{Name: "public-schema"},
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "my_api_integration",
		},
	}
}

func newTestDB(name, ns string) *snowplanev1alpha1.Database {
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
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

func newTestSchema(name, ns string) *snowplanev1alpha1.Schema {
	s := &snowplanev1alpha1.Schema{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.SchemaSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:        "PUBLIC",
			DatabaseRef: &snowplanev1alpha1.ObjectReference{Name: "analytics-db"},
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

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.GitRepository, Service, *snowflake.GitRepositoryObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.GitRepository{},
			&snowplanev1alpha1.Database{},
			&snowplanev1alpha1.Schema{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("GitRepository")

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
			return newTestGitRepository(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.GitRepository{}
		},
		FinalizerName: finalizerName,
		PrereqObjects: func() []runtime.Object {
			return []runtime.Object{
				newTestDB("analytics-db", "default"),
				newTestSchema("public-schema", "default"),
			}
		},
	}.Run(t)
}
