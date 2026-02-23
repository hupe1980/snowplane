package provider

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = snowplanev1alpha1.AddToScheme(s)

	return s
}

// testConditionedObject implements conditions.ConditionedObject for tests.
type testConditionedObject struct {
	conditions []metav1.Condition
}

func (o *testConditionedObject) GetConditions() []metav1.Condition  { return o.conditions }
func (o *testConditionedObject) SetConditions(c []metav1.Condition) { o.conditions = c }

func newReadyPC(namespace string) *snowplanev1alpha1.ProviderConfig {
	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-pc",
			Namespace: namespace,
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "acct",
			User:               "user",
			Region:             "us-east-1",
			Role:               "SYSADMIN",
			Warehouse:          "WH",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name:      "snowflake-creds",
					Namespace: namespace,
					Key:       "password",
				},
			},
		},
	}
	conditions.SetReady(pc, "ok")

	return pc
}

func newTestSecret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snowflake-creds",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte("s3cret"),
		},
	}
}

func TestResolveClient_Success(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")
	secret := newTestSecret("default")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	sfClient, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, sfClient)
}

func TestResolveClient_CrossNamespace(t *testing.T) {
	t.Parallel()

	// ProviderConfig lives in "system" namespace, resource lives in "team-a" namespace.
	pc := newReadyPC("system")
	secret := newTestSecret("system")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	// Resource namespace is "team-a" but providerRef.namespace overrides to "system".
	sfClient, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, sfClient)
}

func TestResolveClient_AllowedNamespaces_Allowed(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaces = []string{"team-a", "team-b"}
	secret := newTestSecret("system")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	sfClient, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, sfClient)
}

func TestResolveClient_AllowedNamespaces_Denied(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaces = []string{"team-a", "team-b"}
	secret := newTestSecret("system")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-c", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace not allowed")

	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonNamespaceNotAllowed, cond.Reason)
	assert.Contains(t, cond.Message, "team-c")
}

func TestResolveClient_AllowedNamespaces_Wildcard(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaces = []string{"*"}
	secret := newTestSecret("system")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	sfClient, err := ResolveClient(context.Background(), c, factory, obj, ref, "any-namespace", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, sfClient)
}

func TestResolveClient_AllowedNamespaces_EmptyListAllowsAll(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	// AllowedNamespaces is nil (empty) — should allow all namespaces.
	secret := newTestSecret("system")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	sfClient, err := ResolveClient(context.Background(), c, factory, obj, ref, "any-ns", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, sfClient)
}

func TestResolveClient_ProviderConfigNotFound(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	factory := clientfactory.NewClientFactory()
	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "missing-pc"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching ProviderConfig")

	// Should set NotReady condition
	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyWait, cond.Reason)
}

func TestResolveClient_ProviderConfigNotReady(t *testing.T) {
	t.Parallel()

	pc := &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-pc",
			Namespace: "default",
		},
	}
	// Not setting Ready condition — it's not ready.

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc).
		Build()

	factory := clientfactory.NewClientFactory()
	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProviderConfig not ready")

	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonDependencyWait, cond.Reason)
}

func TestResolveClient_SecretNotFound(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewClientFactory()
	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching secret")

	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonCredentialsError, cond.Reason)
}

func TestResolveClient_SecretMissingKey(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snowflake-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"wrong-key": []byte("value"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewClientFactory()
	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "password")
}

func TestResolveClient_FactoryGetOrCreateFailure(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")
	secret := newTestSecret("default")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return nil, fmt.Errorf("driver init failed")
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver init failed")

	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonReconcileError, cond.Reason)
}

// fakeSnowflakeClient implements clientfactory.SnowflakeClient for tests.
type fakeSnowflakeClient struct{}

func (f *fakeSnowflakeClient) Ping(_ context.Context) error { return nil }
func (f *fakeSnowflakeClient) Close() error                 { return nil }
func (f *fakeSnowflakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (f *fakeSnowflakeClient) QueryRow(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}
func (f *fakeSnowflakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeSnowflakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, nil, nil
}
