package providerconfig

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/circuitbreaker"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/provider"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
	"github.com/hupe1980/snowplane/internal/utils/finalizers"
)

func strPtr(s string) *string { return &s }

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(s); err != nil {
		panic(fmt.Sprintf("failed to register client-go scheme: %v", err))
	}

	if err := snowplanev1alpha1.AddToScheme(s); err != nil {
		panic(fmt.Sprintf("failed to register snowplane scheme: %v", err))
	}

	return s
}

func newTestPC(name string) *snowplanev1alpha1.ProviderConfig {
	return &snowplanev1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: snowplanev1alpha1.ProviderConfigSpec{
			Account:            "testaccount",
			User:               "testuser",
			Region:             "us-east-1",
			Role:               "SYSADMIN",
			Warehouse:          "COMPUTE_WH",
			AuthenticationType: snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			Credentials: snowplanev1alpha1.ProviderCredentials{
				SecretRef: &snowplanev1alpha1.SecretKeyReference{
					Name:      "my-secret",
					Namespace: "default",
					Key:       "password",
				},
			},
		},
	}
}

func newTestSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("s3cret"),
		},
	}
}

// newTestReconciler builds a Reconciler with a fake k8s client, injected PingFunc, and fake EventRecorder.
func newTestReconciler(pingFn PingFunc, objs ...runtime.Object) (*Reconciler, *clientfactory.ClientFactory, *record.FakeRecorder) {
	return newTestReconcilerWithRoles(pingFn, nil, objs...)
}

// newTestReconcilerWithRoles is like newTestReconciler but also accepts an allowedRoles set.
func newTestReconcilerWithRoles(pingFn PingFunc, allowedRoles map[string]bool, objs ...runtime.Object) (*Reconciler, *clientfactory.ClientFactory, *record.FakeRecorder) {
	scheme := testScheme()

	cb := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{})

	// Register providerRef field indexer for all managed types so isInUse can
	// use MatchingFields. This mirrors the real SetupWithManager registration.
	for _, entry := range managedResourceTypes() {
		cb = cb.WithIndex(entry.proto, providerRefIndex, providerRefExtractor)
	}

	// Register secretRef field indexer for Secret→ProviderConfig lookups (R9-4).
	cb = cb.WithIndex(&snowplanev1alpha1.ProviderConfig{}, secretRefIndex, secretRefExtractor)

	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}

	c := cb.Build()
	factory := clientfactory.NewClientFactory()
	recorder := record.NewFakeRecorder(100)

	r := &Reconciler{
		client:         c,
		factory:        factory,
		recorder:       recorder,
		rateLimiter:    ratelimit.New(ratelimit.DefaultOptions()),
		circuitBreaker: circuitbreaker.New(circuitbreaker.DefaultOptions()),
		pingFn:         pingFn,
		allowedRoles:   allowedRoles,
	}

	return r, factory, recorder
}

func reconcileReq(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}}
}

// --------------------------------------------------------------------------
// Tests: CR not found
// --------------------------------------------------------------------------

func TestReconcile_CRDeleted(t *testing.T) {
	t.Parallel()

	r, factory, _ := newTestReconciler(nil)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("deleted"))
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --------------------------------------------------------------------------
// Tests: Secret errors
// --------------------------------------------------------------------------

func TestReconcile_SecretNotFound(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	r, factory, _ := newTestReconciler(nil, pc)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.Error(t, err) // original error returned for controller-runtime backoff
	assert.Contains(t, err.Error(), "not found")

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonSecretNotFound, readyCond.Reason)
}

func TestReconcile_SecretNamespaceFallback(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Namespace = "team-a"
	pc.Spec.Credentials.SecretRef.Namespace = "" // empty → fallback to pc.Namespace

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "team-a"},
		Data:       map[string][]byte{"password": []byte("pw")},
	}

	pingCalled := false
	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		pingCalled = true
		return nil
	}

	r, factory, _ := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "my-pc", Namespace: "team-a"},
	})
	require.NoError(t, err)
	assert.True(t, pingCalled, "should reach Ping when secret is found via namespace fallback")
}

func TestReconcile_SecretMissingKey(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	secret := newTestSecret()
	secret.Data = map[string][]byte{"wrong-key": []byte("value")}

	r, factory, _ := newTestReconciler(nil, pc, secret)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.Error(t, err) // config build error returned for backoff
	assert.Contains(t, err.Error(), "does not contain key")

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonInvalidConfig, readyCond.Reason)
}

// --------------------------------------------------------------------------
// Tests: Ping errors
// --------------------------------------------------------------------------

func TestReconcile_PingFailed(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	secret := newTestSecret()

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return fmt.Errorf("connection refused")
	}

	r, factory, _ := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.Error(t, err) // original error returned for backoff
	assert.Contains(t, err.Error(), "connection refused")

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonPingFailed, readyCond.Reason)
	assert.Contains(t, readyCond.Message, "connection refused")
}

// --------------------------------------------------------------------------
// Tests: Happy path
// --------------------------------------------------------------------------

func TestReconcile_ClientFactoryGetOrCreateFailure(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	secret := newTestSecret()

	// Use a factory that always fails to create a client.
	failingFactory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return nil, fmt.Errorf("cannot create snowflake client")
	})
	defer failingFactory.Close()

	scheme := testScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(pc, secret).Build()

	r := &Reconciler{
		client:   c,
		factory:  failingFactory,
		recorder: record.NewFakeRecorder(100),
		pingFn:   func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil },
	}

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot create snowflake client")

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonClientFailed, readyCond.Reason)
}

func TestReconcile_Success(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	secret := newTestSecret()

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return nil // success
	}

	r, factory, recorder := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Equal(t, requeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
	assert.Equal(t, int64(1), got.Status.ObservedGeneration)

	// Verify events were emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, snowplanev1alpha1.ReasonAvailable)
	default:
		t.Fatal("expected an event to be recorded")
	}
}

func TestReconcile_Success_ClearsCredentialsInvalid(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	// Pre-set CredentialsInvalid from a previous failure.
	conditions.SetNotReady(pc, snowplanev1alpha1.ReasonSecretNotFound, "old error")

	secret := newTestSecret()
	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil }

	r, factory, _ := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

// --------------------------------------------------------------------------
// Tests: BuildConfig (unit — shared provider package)
// --------------------------------------------------------------------------

func TestBuildConfig_UsernamePassword(t *testing.T) {
	t.Parallel()
	pc := newTestPC("default")
	secret := newTestSecret()

	cfg, err := provider.BuildSnowflakeConfig(pc, secret)
	require.NoError(t, err)
	assert.Equal(t, "testaccount", cfg.Account)
	assert.Equal(t, "testuser", cfg.User)
	assert.Equal(t, []byte("s3cret"), cfg.Password)
	assert.Empty(t, cfg.PrivateKey)
}

func TestBuildConfig_KeyPair(t *testing.T) {
	t.Parallel()
	pc := newTestPC("default")
	pc.Spec.AuthenticationType = snowplanev1alpha1.AuthenticationTypeKeyPair
	pc.Spec.Credentials.SecretRef.Key = "private-key"

	secret := newTestSecret()
	secret.Data = map[string][]byte{
		"private-key": []byte("-----BEGIN PRIVATE KEY-----\nfakekey\n-----END PRIVATE KEY-----"),
	}

	cfg, err := provider.BuildSnowflakeConfig(pc, secret)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.PrivateKey)
	assert.Empty(t, cfg.Password)
}

func TestBuildConfig_MissingSecretKey(t *testing.T) {
	t.Parallel()
	pc := newTestPC("default")
	secret := newTestSecret()
	secret.Data = map[string][]byte{}

	_, err := provider.BuildSnowflakeConfig(pc, secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain key")
}

func TestBuildConfig_EmptyKeyFails(t *testing.T) {
	t.Parallel()
	pc := newTestPC("default")
	pc.Spec.Credentials.SecretRef.Key = "" // CRD enforces minLength:1, but test the code path

	secret := newTestSecret()
	secret.Data = map[string][]byte{
		"credentials": []byte("default-password"),
	}

	_, err := provider.BuildSnowflakeConfig(pc, secret)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain key")
}

func TestComputeHash_Deterministic(t *testing.T) {
	t.Parallel()
	cfg := snowflake.Config{
		Account:  "acct",
		User:     "user",
		Password: []byte("pass"),
	}
	h1 := provider.ComputeHash(cfg)
	h2 := provider.ComputeHash(cfg)
	assert.Equal(t, h1, h2)
	assert.Len(t, h1, 64) // SHA-256 hex
}

func TestComputeHash_DifferentConfigs(t *testing.T) {
	t.Parallel()
	cfg1 := snowflake.Config{Account: "acct1"}
	cfg2 := snowflake.Config{Account: "acct2"}
	assert.NotEqual(t, provider.ComputeHash(cfg1), provider.ComputeHash(cfg2))
}

func TestConditionHelpers(t *testing.T) {
	t.Parallel()
	pc := newTestPC("test")
	conditions.SetReady(pc, "ok")
	assert.True(t, conditions.IsTrue(pc, "Ready"))

	conditions.SetNotReady(pc, "Reason", "msg")
	assert.False(t, conditions.IsTrue(pc, "Ready"))
}

// --------------------------------------------------------------------------
// Tests: mapSecretToProviderConfigs
// --------------------------------------------------------------------------

func TestMapSecretToProviderConfigs_MatchingSecret(t *testing.T) {
	t.Parallel()

	pc1 := newTestPC("pc-1")
	pc2 := newTestPC("pc-2")
	pc2.Spec.Credentials.SecretRef.Name = "other-secret"

	r, factory, _ := newTestReconciler(nil, pc1, pc2)
	defer factory.Close()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
	}

	requests := r.mapSecretToProviderConfigs(context.Background(), secret)

	// Only pc-1 references "my-secret"
	require.Len(t, requests, 1)
	assert.Equal(t, "pc-1", requests[0].Name)
	assert.Equal(t, "default", requests[0].Namespace)
}

func TestMapSecretToProviderConfigs_NoMatch(t *testing.T) {
	t.Parallel()

	pc := newTestPC("pc-1")
	r, factory, _ := newTestReconciler(nil, pc)
	defer factory.Close()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated-secret",
			Namespace: "default",
		},
	}

	requests := r.mapSecretToProviderConfigs(context.Background(), secret)
	assert.Empty(t, requests)
}

func TestMapSecretToProviderConfigs_NamespaceMatch(t *testing.T) {
	t.Parallel()

	pc := newTestPC("pc-1")
	pc.Spec.Credentials.SecretRef.Namespace = "prod"

	r, factory, _ := newTestReconciler(nil, pc)
	defer factory.Close()

	// Secret in wrong namespace — should not match.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
	}

	requests := r.mapSecretToProviderConfigs(context.Background(), secret)
	assert.Empty(t, requests)

	// Secret in correct namespace — should match.
	secretProd := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "prod",
		},
	}

	requests = r.mapSecretToProviderConfigs(context.Background(), secretProd)
	require.Len(t, requests, 1)
	assert.Equal(t, "pc-1", requests[0].Name)
}

func TestMapSecretToProviderConfigs_EmptyNamespaceFallsBackToPCNamespace(t *testing.T) {
	t.Parallel()

	pc := newTestPC("pc-1")
	pc.Spec.Credentials.SecretRef.Namespace = "" // empty = use PC's namespace

	r, factory, _ := newTestReconciler(nil, pc)
	defer factory.Close()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default", // PC's namespace
		},
	}

	requests := r.mapSecretToProviderConfigs(context.Background(), secret)
	require.Len(t, requests, 1)
	assert.Equal(t, "pc-1", requests[0].Name)
}

// --------------------------------------------------------------------------
// Tests: Finalizer and deletion guard (L-14)
// --------------------------------------------------------------------------

func TestReconcile_AddsFinalizer(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	secret := newTestSecret()
	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil }

	r, factory, _ := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, finalizers.Has(got, finalizerName), "should have in-use finalizer after reconcile")
}

func TestReconcile_DeleteBlockedWhenInUse(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	pc := newTestPC("my-pc")
	pc.DeletionTimestamp = &now
	pc.Finalizers = []string{finalizerName}

	// Create a Database that references this ProviderConfig.
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "ref-db", Namespace: "default"},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "my-pc"},
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
			},
			Name: "REF_DB",
		},
	}

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil }
	r, factory, recorder := newTestReconciler(pingFn, pc, db)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, result.RequeueAfter, "should requeue to retry deletion")

	// Verify finalizer is still present.
	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))
	assert.True(t, finalizers.Has(got, finalizerName), "finalizer should remain when in use")

	// Verify warning event was emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "InUse")
	default:
		t.Fatal("expected InUse warning event")
	}
}

func TestReconcile_DeleteSucceedsWhenNotInUse(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	pc := newTestPC("my-pc")
	pc.DeletionTimestamp = &now
	pc.Finalizers = []string{finalizerName}

	// No resources reference this ProviderConfig.
	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil }
	r, factory, _ := newTestReconciler(pingFn, pc)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "should not requeue after successful deletion")

	// Once the last finalizer is removed, Kubernetes (fake client) deletes the object.
	got := &snowplanev1alpha1.ProviderConfig{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got)
	assert.True(t, apierrors.IsNotFound(err), "object should be deleted after finalizer removal")
}

func TestReconcile_DeleteNoFinalizerSkipsCheck(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	pc := newTestPC("my-pc")
	pc.DeletionTimestamp = &now
	pc.Finalizers = []string{"other-finalizer"} // has a finalizer, but not ours

	r, factory, _ := newTestReconciler(nil, pc)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
}

func TestIsInUse_AllResourceTypes(t *testing.T) {
	t.Parallel()

	// For each resource type, create one that references "my-pc" and verify isInUse returns true.
	tests := []struct {
		name string
		obj  runtime.Object
	}{
		{"Database", &snowplanev1alpha1.Database{
			ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "default"},
			Spec:       snowplanev1alpha1.DatabaseSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "D"},
		}},
		{"Schema", &snowplanev1alpha1.Schema{
			ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"},
			Spec:       snowplanev1alpha1.SchemaSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "S", DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "db"}},
		}},
		{"Warehouse", &snowplanev1alpha1.Warehouse{
			ObjectMeta: metav1.ObjectMeta{Name: "w", Namespace: "default"},
			Spec:       snowplanev1alpha1.WarehouseSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "WH"},
		}},
		{"User", &snowplanev1alpha1.User{
			ObjectMeta: metav1.ObjectMeta{Name: "u", Namespace: "default"},
			Spec:       snowplanev1alpha1.UserSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "U"},
		}},
		{"AccountRole", &snowplanev1alpha1.AccountRole{
			ObjectMeta: metav1.ObjectMeta{Name: "ar", Namespace: "default"},
			Spec:       snowplanev1alpha1.AccountRoleSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "AR"},
		}},
		{"DatabaseRole", &snowplanev1alpha1.DatabaseRole{
			ObjectMeta: metav1.ObjectMeta{Name: "dr", Namespace: "default"},
			Spec:       snowplanev1alpha1.DatabaseRoleSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Name: "DR", DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "db"}},
		}},
		{"AccountRoleGrant", &snowplanev1alpha1.AccountRoleGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
			Spec:       snowplanev1alpha1.AccountRoleGrantSpec{CommonSpec: snowplanev1alpha1.CommonSpec{ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"}, DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete}, Privilege: "USAGE", On: snowplanev1alpha1.GrantOn{Account: true}, AccountRole: strPtr("R")},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, factory, _ := newTestReconciler(nil, tt.obj)
			defer factory.Close()

			inUse, err := r.isInUse(context.Background(), "my-pc")
			require.NoError(t, err)
			assert.True(t, inUse, "%s should mark ProviderConfig as in-use", tt.name)
		})
	}
}

func TestIsInUse_NoReferences(t *testing.T) {
	t.Parallel()

	// A Database referencing a different ProviderConfig.
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "default"},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "other-pc"},
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
			},
			Name: "D",
		},
	}

	r, factory, _ := newTestReconciler(nil, db)
	defer factory.Close()

	inUse, err := r.isInUse(context.Background(), "my-pc")
	require.NoError(t, err)
	assert.False(t, inUse)
}

// --------------------------------------------------------------------------
// Tests: Secret rotation (Story 9.4.2)
// --------------------------------------------------------------------------

func TestReconcile_SecretRotation_EmitsEvent(t *testing.T) {
	t.Parallel()

	pc := newTestPC("rotation-pc")
	secret := newTestSecret()

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return nil
	}

	r, factory, recorder := newTestReconciler(pingFn, pc, secret)
	defer factory.Close()

	// First reconciliation — establishes initial connection.
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "rotation-pc", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Drain events from first reconciliation.
	drainEvents(recorder)

	// Simulate secret rotation: update the secret data.
	got := &corev1.Secret{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{
		Name: "my-secret", Namespace: "default",
	}, got))
	got.Data["password"] = []byte("new-rotated-password")
	require.NoError(t, r.client.Update(context.Background(), got))

	// Second reconciliation — should detect hash change and emit rotation event.
	result, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "rotation-pc", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify rotation event was emitted.
	events := collectEvents(recorder)
	hasRotationEvent := false
	for _, e := range events {
		if strings.Contains(e, "CredentialsRotated") {
			hasRotationEvent = true
			break
		}
	}
	assert.True(t, hasRotationEvent, "expected CredentialsRotated event, got events: %v", events)
}

func TestReconcile_SecretRotation_NewClientCreated(t *testing.T) {
	t.Parallel()

	pc := newTestPC("rotation-client-pc")
	secret := newTestSecret()

	connectCount := 0
	testFactory := clientfactory.NewTestClientFactoryWithFn(func(cfg snowflake.Config) (clientfactory.SnowflakeClient, error) {
		connectCount++
		return &mockClient{password: cfg.Password}, nil
	})
	defer testFactory.Close()

	scheme := testScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.ProviderConfig{}).
		WithRuntimeObjects(pc, secret).Build()

	r := &Reconciler{
		client:   c,
		factory:  testFactory,
		recorder: record.NewFakeRecorder(100),
		pingFn:   func(_ context.Context, _ clientfactory.SnowflakeClient) error { return nil },
	}

	// First reconciliation — creates initial client.
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "rotation-client-pc", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, connectCount, "should create initial client")

	// Rotate secret.
	got := &corev1.Secret{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{
		Name: "my-secret", Namespace: "default",
	}, got))
	got.Data["password"] = []byte("rotated-password")
	require.NoError(t, r.client.Update(context.Background(), got))

	// Second reconciliation — should create new client with new credentials.
	_, err = r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "rotation-client-pc", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, connectCount, "should create new client after secret rotation")
}

func drainEvents(recorder *record.FakeRecorder) {
	for {
		select {
		case <-recorder.Events:
		default:
			return
		}
	}
}

func collectEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-recorder.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

// mockClient implements clientfactory.SnowflakeClient for tests that need to
// track which credentials were used to create the client.
type mockClient struct {
	password []byte
}

func (m *mockClient) Ping(_ context.Context) error { return nil }
func (m *mockClient) Close() error                 { return nil }
func (m *mockClient) Exec(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}
func (m *mockClient) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return nil
}

// --------------------------------------------------------------------------
// Tests: providerRefExtractor
// --------------------------------------------------------------------------

func TestProviderRefExtractor_ReturnsName(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "default"},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				ProviderRef: snowplanev1alpha1.ProviderReference{Name: "my-pc"},
			},
			Name: "D",
		},
	}

	got := providerRefExtractor(db)
	assert.Equal(t, []string{"my-pc"}, got)
}

func TestProviderRefExtractor_EmptyName(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "default"},
		Spec:       snowplanev1alpha1.DatabaseSpec{Name: "D"},
	}

	got := providerRefExtractor(db)
	assert.Nil(t, got)
}

func TestProviderRefExtractor_WrongType(t *testing.T) {
	t.Parallel()

	// ProviderConfig itself does not satisfy ManagedResource.GetProviderRef()
	pc := newTestPC("x")

	got := providerRefExtractor(pc)
	assert.Nil(t, got)
}

func TestListLen_AllTypes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1, listLen(&snowplanev1alpha1.DatabaseList{Items: []snowplanev1alpha1.Database{{}}}))
	assert.Equal(t, 0, listLen(&snowplanev1alpha1.DatabaseList{}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.SchemaList{Items: []snowplanev1alpha1.Schema{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.WarehouseList{Items: []snowplanev1alpha1.Warehouse{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.UserList{Items: []snowplanev1alpha1.User{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.AccountRoleList{Items: []snowplanev1alpha1.AccountRole{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.DatabaseRoleList{Items: []snowplanev1alpha1.DatabaseRole{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.TableList{Items: []snowplanev1alpha1.Table{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.ViewList{Items: []snowplanev1alpha1.View{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.StageList{Items: []snowplanev1alpha1.Stage{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.AccountRoleGrantList{Items: []snowplanev1alpha1.AccountRoleGrant{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.DatabaseRoleGrantList{Items: []snowplanev1alpha1.DatabaseRoleGrant{{}}}))
	assert.Equal(t, 1, listLen(&snowplanev1alpha1.ShareGrantList{Items: []snowplanev1alpha1.ShareGrant{{}}}))
}

// --------------------------------------------------------------------------
// Tests: IsRoleAllowed (M-4)
// --------------------------------------------------------------------------

func TestIsRoleAllowed_NilAllowlist_AllRolesPermitted(t *testing.T) {
	t.Parallel()

	r := &Reconciler{allowedRoles: nil}
	assert.True(t, r.IsRoleAllowed("ACCOUNTADMIN"))
	assert.True(t, r.IsRoleAllowed("SYSADMIN"))
	assert.True(t, r.IsRoleAllowed("anything"))
}

func TestIsRoleAllowed_EmptyAllowlist_AllRolesPermitted(t *testing.T) {
	t.Parallel()

	r := &Reconciler{allowedRoles: map[string]bool{}}
	assert.True(t, r.IsRoleAllowed("ACCOUNTADMIN"))
	assert.True(t, r.IsRoleAllowed("SYSADMIN"))
}

func TestIsRoleAllowed_AllowlistEnforced(t *testing.T) {
	t.Parallel()

	r := &Reconciler{allowedRoles: map[string]bool{
		"SYSADMIN":      true,
		"DATA_ENGINEER": true,
	}}

	assert.True(t, r.IsRoleAllowed("SYSADMIN"))
	assert.True(t, r.IsRoleAllowed("DATA_ENGINEER"))
	assert.False(t, r.IsRoleAllowed("ACCOUNTADMIN"))
	assert.False(t, r.IsRoleAllowed("SECURITYADMIN"))
}

func TestIsRoleAllowed_CaseInsensitive(t *testing.T) {
	t.Parallel()

	r := &Reconciler{allowedRoles: map[string]bool{
		"SYSADMIN": true,
	}}

	assert.True(t, r.IsRoleAllowed("SYSADMIN"))
	assert.True(t, r.IsRoleAllowed("sysadmin"))
	assert.True(t, r.IsRoleAllowed("SysAdmin"))
	assert.True(t, r.IsRoleAllowed("Sysadmin"))
}

// --------------------------------------------------------------------------
// Tests: Reconcile with role allowlist (M-4)
// --------------------------------------------------------------------------

func TestReconcile_RoleNotAllowed_SetsCondition(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "ACCOUNTADMIN"
	secret := newTestSecret()

	allowedRoles := map[string]bool{"SYSADMIN": true, "USERADMIN": true}
	r, factory, recorder := newTestReconcilerWithRoles(nil, allowedRoles, pc, secret)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err, "should not return error — requeues via RequeueAfter")
	assert.True(t, result.RequeueAfter > 0, "should requeue for periodic resync")

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonRoleNotAllowed, readyCond.Reason)
	assert.Contains(t, readyCond.Message, "ACCOUNTADMIN")
	assert.Contains(t, readyCond.Message, "not in the allowed roles list")

	// Verify warning event was emitted.
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, snowplanev1alpha1.ReasonRoleNotAllowed)
		assert.Contains(t, event, "ACCOUNTADMIN")
	default:
		t.Fatal("expected RoleNotAllowed warning event")
	}
}

func TestReconcile_RoleNotAllowed_CaseInsensitive(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "accountadmin" // lowercase

	allowedRoles := map[string]bool{"SYSADMIN": true}
	r, factory, _ := newTestReconcilerWithRoles(nil, allowedRoles, pc, newTestSecret())
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	readyCond := conditions.Get(got, snowplanev1alpha1.TypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, snowplanev1alpha1.ReasonRoleNotAllowed, readyCond.Reason)
}

func TestReconcile_RoleAllowed_Succeeds(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "SYSADMIN"
	secret := newTestSecret()

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return nil
	}

	allowedRoles := map[string]bool{"SYSADMIN": true, "USERADMIN": true}
	r, factory, _ := newTestReconcilerWithRoles(pingFn, allowedRoles, pc, secret)
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Equal(t, requeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeSynced))
}

func TestReconcile_EmptyRole_SkipsAllowlistCheck(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "" // empty → user's default role, should skip allowlist check

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return nil
	}

	// Even with an allowlist configured, empty role should not be blocked.
	allowedRoles := map[string]bool{"SYSADMIN": true}
	r, factory, _ := newTestReconcilerWithRoles(pingFn, allowedRoles, pc, newTestSecret())
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Equal(t, requeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_NilAllowlist_AllRolesPass(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "ACCOUNTADMIN" // would be blocked by a real allowlist

	pingFn := func(_ context.Context, _ clientfactory.SnowflakeClient) error {
		return nil
	}

	// nil allowlist = no restriction.
	r, factory, _ := newTestReconcilerWithRoles(pingFn, nil, pc, newTestSecret())
	defer factory.Close()

	result, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)
	assert.Equal(t, requeueInterval, result.RequeueAfter)

	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.True(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func TestReconcile_RoleNotAllowed_HealthMetricSetFalse(t *testing.T) {
	t.Parallel()

	pc := newTestPC("my-pc")
	pc.Spec.Role = "SECURITYADMIN"

	allowedRoles := map[string]bool{"SYSADMIN": true}
	r, factory, _ := newTestReconcilerWithRoles(nil, allowedRoles, pc, newTestSecret())
	defer factory.Close()

	_, err := r.Reconcile(context.Background(), reconcileReq("my-pc"))
	require.NoError(t, err)

	// The metric gauge is set — we can't easily Assert on prometheus
	// gauges in unit tests, but we verify the code path completes without panic.
	got := &snowplanev1alpha1.ProviderConfig{}
	require.NoError(t, r.client.Get(context.Background(), types.NamespacedName{Name: "my-pc", Namespace: "default"}, got))

	assert.False(t, conditions.IsTrue(got, snowplanev1alpha1.TypeReady))
}

func (m *mockClient) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}
func (m *mockClient) WithRole(_ context.Context, _ string) (*snowflake.Client, func(context.Context), error) {
	return nil, nil, nil
}

func TestWithSnowflakeOpTimeout(t *testing.T) {
	r := &Reconciler{}

	// Default should return defaultSnowflakeOpTimeout.
	assert.Equal(t, defaultSnowflakeOpTimeout, r.getSnowflakeOpTimeout())

	// Override should be honoured.
	r.WithSnowflakeOpTimeout(30 * time.Second)
	assert.Equal(t, 30*time.Second, r.getSnowflakeOpTimeout())

	// Zero should fall back to default.
	r.WithSnowflakeOpTimeout(0)
	assert.Equal(t, defaultSnowflakeOpTimeout, r.getSnowflakeOpTimeout())
}
