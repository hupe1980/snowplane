package scimintegration

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
	observeFn func(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SCIMIntegrationObservation, error)
	createFn  func(ctx context.Context, opts snowflake.CreateSCIMIntegrationOptions) error
	alterFn   func(ctx context.Context, opts snowflake.AlterSCIMIntegrationOptions) error
	dropFn    func(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

func (m *mockService) Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SCIMIntegrationObservation, error) {
	if m.observeFn != nil {
		return m.observeFn(ctx, name)
	}
	return &snowflake.SCIMIntegrationObservation{Exists: false}, nil
}

func (m *mockService) Create(ctx context.Context, opts snowflake.CreateSCIMIntegrationOptions) error {
	if m.createFn != nil {
		return m.createFn(ctx, opts)
	}
	return nil
}

func (m *mockService) Alter(ctx context.Context, opts snowflake.AlterSCIMIntegrationOptions) error {
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

func newTestSCIMIntegration(name, ns string) *snowplanev1alpha1.SCIMIntegration {
	return &snowplanev1alpha1.SCIMIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Generation: 1},
		Spec: snowplanev1alpha1.SCIMIntegrationSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name:       "MY_SCIM",
			SCIMClient: "GENERIC_SCIM_PROVISIONER",
			RunAsRole:  "GENERIC_SCIM_PROVISIONER",
		},
	}
}

func newTestReconciler(mock *mockService, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.SCIMIntegration, Service, *snowflake.SCIMIntegrationObservation] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(
			&snowplanev1alpha1.SCIMIntegration{},
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
	r.GVK = snowplanev1alpha1.GroupVersion.WithKind("SCIMIntegration")

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
			return newTestSCIMIntegration(name, ns)
		},
		NewBlankObject: func() client.Object {
			return &snowplanev1alpha1.SCIMIntegration{}
		},
		FinalizerName: finalizerName,
	}.Run(t)
}

func successfulObservation() *snowflake.SCIMIntegrationObservation {
	return &snowflake.SCIMIntegrationObservation{
		Exists: true,
		ShowOutput: &snowplanev1alpha1.SCIMIntegrationShowOutput{
			Name:    "MY_SCIM",
			Enabled: true,
		},
		DescribeOutput: map[string]string{
			"SCIM_CLIENT":    "GENERIC_SCIM_PROVISIONER",
			"RUN_AS_ROLE":    "GENERIC_SCIM_PROVISIONER",
			"NETWORK_POLICY": "",
			"SYNC_PASSWORD":  "true",
		},
	}
}

func TestBuildAlterOptions(t *testing.T) {
	t.Parallel()

	t.Run("NetworkPolicySkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.NetworkPolicy = testutil.Ptr("MY_POLICY")
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := successfulObservation()
		obs.DescribeOutput["NETWORK_POLICY"] = "MY_POLICY"

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.NetworkPolicy, "Should skip when value matches DESCRIBE output")
	})

	t.Run("NetworkPolicySentWhenChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.NetworkPolicy = testutil.Ptr("NEW_POLICY")
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := successfulObservation()
		obs.DescribeOutput["NETWORK_POLICY"] = "OLD_POLICY"

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.NetworkPolicy)
		assert.Equal(t, "NEW_POLICY", *opts.NetworkPolicy)
	})

	t.Run("NetworkPolicyCaseInsensitiveMatch", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.NetworkPolicy = testutil.Ptr("my_policy")
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := successfulObservation()
		obs.DescribeOutput["NETWORK_POLICY"] = "MY_POLICY"

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.NetworkPolicy, "Should skip when value matches case-insensitively")
	})

	t.Run("SyncPasswordSkippedWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.SyncPassword = testutil.Ptr(true)
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := successfulObservation()
		obs.DescribeOutput["SYNC_PASSWORD"] = "true"

		opts := buildAlterOptions(obj, id, obs)
		assert.Nil(t, opts.SyncPassword, "Should skip when value matches DESCRIBE output")
	})

	t.Run("SyncPasswordSentWhenChanged", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.SyncPassword = testutil.Ptr(false)
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := successfulObservation()
		obs.DescribeOutput["SYNC_PASSWORD"] = "true"

		opts := buildAlterOptions(obj, id, obs)
		require.NotNil(t, opts.SyncPassword)
		assert.Equal(t, false, *opts.SyncPassword)
	})

	t.Run("AllFieldsSentWhenNoDescribeOutput", func(t *testing.T) {
		t.Parallel()

		obj := newTestSCIMIntegration("myint", "default")
		obj.Spec.NetworkPolicy = testutil.Ptr("MY_POLICY")
		obj.Spec.SyncPassword = testutil.Ptr(true)
		id := snowflake.NewAccountObjectIdentifier("MY_SCIM")
		obs := &snowflake.SCIMIntegrationObservation{
			Exists:     true,
			ShowOutput: successfulObservation().ShowOutput,
		}

		opts := buildAlterOptions(obj, id, obs)
		assert.NotNil(t, opts.NetworkPolicy, "Should send when no DESCRIBE output")
		assert.NotNil(t, opts.SyncPassword, "Should send when no DESCRIBE output")
	})
}
