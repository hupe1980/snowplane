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
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(snowplanev1alpha1.AddToScheme(s))

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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
	assert.Equal(t, "default-pc", resolved.Name)
	assert.Equal(t, "default/default-pc", resolved.CacheKey)
	assert.Equal(t, "acct", resolved.Account)
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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	// Resource namespace is "team-a" but providerRef.namespace overrides to "system".
	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
	assert.Equal(t, "default-pc", resolved.Name)
	assert.Equal(t, "system/default-pc", resolved.CacheKey)
	assert.Equal(t, "acct", resolved.Account)
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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "any-namespace", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "any-ns", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
}

func TestResolveClient_AllowedNamespaceSelector_MatchLabels(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"team": "analytics"},
	}
	secret := newTestSecret("system")

	// Namespace with matching label.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "team-analytics",
			Labels: map[string]string{"team": "analytics"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret, ns).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-analytics", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolved.Client)
}

func TestResolveClient_AllowedNamespaceSelector_NoMatch(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"team": "analytics"},
	}
	secret := newTestSecret("system")

	// Namespace without matching label.
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "team-marketing",
			Labels: map[string]string{"team": "marketing"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret, ns).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}

	_, err := ResolveClient(context.Background(), c, factory, obj, ref, "team-marketing", nil, nil, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace not allowed")

	cond := conditions.Get(obj, snowplanev1alpha1.TypeReady)
	require.NotNil(t, cond)
	assert.Equal(t, snowplanev1alpha1.ReasonNamespaceNotAllowed, cond.Reason)
}

func TestResolveClient_AllowedNamespaceSelector_ORWithStaticList(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("system")
	pc.Spec.AllowedNamespaces = []string{"team-a"}
	pc.Spec.AllowedNamespaceSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"env": "prod"},
	}
	secret := newTestSecret("system")

	// team-b: not in static list, but has matching label → allowed via selector.
	nsB := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "team-b",
			Labels: map[string]string{"env": "prod"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret, nsB).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	// team-a: allowed via static list (no namespace object needed).
	objA := &testConditionedObject{}
	refA := snowplanev1alpha1.ProviderReference{Name: "default-pc", Namespace: "system"}
	resolvedA, err := ResolveClient(context.Background(), c, factory, objA, refA, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolvedA.Client)

	// team-b: allowed via selector.
	objB := &testConditionedObject{}
	resolvedB, err := ResolveClient(context.Background(), c, factory, objB, refA, "team-b", nil, nil, "test")
	require.NoError(t, err)
	assert.NotNil(t, resolvedB.Client)
}

func TestResolveClient_AllowedDatabases_Propagated(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")
	pc.Spec.AllowedDatabases = []string{"ANALYTICS", "RAW"}
	pc.Spec.AllowedSchemas = []string{"ANALYTICS.PUBLIC"}
	secret := newTestSecret("default")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, []string{"ANALYTICS", "RAW"}, resolved.AllowedDatabases)
	assert.Equal(t, []string{"ANALYTICS.PUBLIC"}, resolved.AllowedSchemas)
}

func TestIsDatabaseAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved *ResolvedProvider
		database string
		want     bool
	}{
		{"empty_allows_all", &ResolvedProvider{}, "ANY", true},
		{"wildcard", &ResolvedProvider{AllowedDatabases: []string{"*"}}, "ANY", true},
		{"match", &ResolvedProvider{AllowedDatabases: []string{"ANALYTICS"}}, "ANALYTICS", true},
		{"case_insensitive", &ResolvedProvider{AllowedDatabases: []string{"Analytics"}}, "ANALYTICS", true},
		{"denied", &ResolvedProvider{AllowedDatabases: []string{"ANALYTICS"}}, "RAW", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsDatabaseAllowed(tt.resolved, tt.database), tt.name)
	}
}

func TestIsSchemaAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved *ResolvedProvider
		database string
		schema   string
		want     bool
	}{
		{"empty_allows_all", &ResolvedProvider{}, "DB", "S", true},
		{"wildcard", &ResolvedProvider{AllowedSchemas: []string{"*"}}, "DB", "S", true},
		{"fqn_match", &ResolvedProvider{AllowedSchemas: []string{"DB.PUBLIC"}}, "DB", "PUBLIC", true},
		{"fqn_case_insensitive", &ResolvedProvider{AllowedSchemas: []string{"db.public"}}, "DB", "PUBLIC", true},
		{"fqn_wrong_db", &ResolvedProvider{AllowedSchemas: []string{"DB.PUBLIC"}}, "OTHER", "PUBLIC", false},
		{"name_only", &ResolvedProvider{AllowedSchemas: []string{"PUBLIC"}}, "ANY", "PUBLIC", true},
		{"name_only_denied", &ResolvedProvider{AllowedSchemas: []string{"PUBLIC"}}, "DB", "STAGING", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsSchemaAllowed(tt.resolved, tt.database, tt.schema), tt.name)
	}
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
	assert.ErrorIs(t, err, ErrProviderConfigNotReady)

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

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
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

func TestProviderCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		namespace string
		name      string
		want      string
	}{
		{"default", "default", "default/default"},
		{"team-a", "my-pc", "team-a/my-pc"},
		{"system", "default", "system/default"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, ProviderCacheKey(tt.namespace, tt.name))
	}
}

func TestResolveClient_NamespaceIsolation(t *testing.T) {
	t.Parallel()

	// Two ProviderConfigs with the same name in different namespaces
	// must produce different cache keys (C-3 fix).
	pcA := newReadyPC("team-a")
	pcA.Name = "default"
	pcA.Spec.Account = "acct-a"

	pcB := newReadyPC("team-b")
	pcB.Name = "default"
	pcB.Spec.Account = "acct-b"

	secretA := newTestSecret("team-a")
	secretB := newTestSecret("team-b")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pcA, pcB, secretA, secretB).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	// Resolve for team-a
	objA := &testConditionedObject{}
	refA := snowplanev1alpha1.ProviderReference{Name: "default", Namespace: "team-a"}

	resolvedA, err := ResolveClient(context.Background(), c, factory, objA, refA, "team-a", nil, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "team-a/default", resolvedA.CacheKey)
	assert.Equal(t, "acct-a", resolvedA.Account)

	// Resolve for team-b
	objB := &testConditionedObject{}
	refB := snowplanev1alpha1.ProviderReference{Name: "default", Namespace: "team-b"}

	resolvedB, err := ResolveClient(context.Background(), c, factory, objB, refB, "team-b", nil, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "team-b/default", resolvedB.CacheKey)
	assert.Equal(t, "acct-b", resolvedB.Account)

	// Cache keys must differ
	assert.NotEqual(t, resolvedA.CacheKey, resolvedB.CacheKey)
}

func TestIsRefNamespaceAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resolved *ResolvedProvider
		sourceNS string
		targetNS string
		want     bool
	}{
		{"empty_allows_all", &ResolvedProvider{}, "team-a", "infra", true},
		{"same_ns_always_allowed", &ResolvedProvider{AllowedRefNamespaces: []string{"SAME"}}, "team-a", "team-a", true},
		{"empty_target_always_allowed", &ResolvedProvider{AllowedRefNamespaces: []string{"SAME"}}, "team-a", "", true},
		{"wildcard", &ResolvedProvider{AllowedRefNamespaces: []string{"*"}}, "team-a", "infra", true},
		{"SAME_denies_cross_ns", &ResolvedProvider{AllowedRefNamespaces: []string{"SAME"}}, "team-a", "infra", false},
		{"explicit_match", &ResolvedProvider{AllowedRefNamespaces: []string{"infra", "shared"}}, "team-a", "infra", true},
		{"explicit_denied", &ResolvedProvider{AllowedRefNamespaces: []string{"infra", "shared"}}, "team-a", "forbidden", false},
		{"mixed_SAME_explicit", &ResolvedProvider{AllowedRefNamespaces: []string{"SAME", "infra"}}, "team-a", "infra", true},
		{"mixed_SAME_denied", &ResolvedProvider{AllowedRefNamespaces: []string{"SAME", "infra"}}, "team-a", "other", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsRefNamespaceAllowed(tt.resolved, tt.sourceNS, tt.targetNS), tt.name)
	}
}

func TestResolveClient_AllowedRefNamespaces_Propagated(t *testing.T) {
	t.Parallel()

	pc := newReadyPC("default")
	pc.Spec.AllowedRefNamespaces = []string{"infra", "shared"}
	secret := newTestSecret("default")

	c := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRuntimeObjects(pc, secret).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		Build()

	factory := clientfactory.NewTestClientFactoryWithFn(func(_ context.Context, _ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &fakeSnowflakeClient{}, nil
	})

	obj := &testConditionedObject{}
	ref := snowplanev1alpha1.ProviderReference{Name: "default-pc"}

	resolved, err := ResolveClient(context.Background(), c, factory, obj, ref, "default", nil, nil, "test")
	require.NoError(t, err)
	assert.Equal(t, []string{"infra", "shared"}, resolved.AllowedRefNamespaces)
}

// fakeSnowflakeClient implements clientfactory.SnowflakeClient for tests.
type fakeSnowflakeClient struct{}

func (f *fakeSnowflakeClient) Ping(_ context.Context) error { return nil }
func (f *fakeSnowflakeClient) Close() error                 { return nil }
func (f *fakeSnowflakeClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (f *fakeSnowflakeClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return nil
}
func (f *fakeSnowflakeClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeSnowflakeClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, nil, nil
}
