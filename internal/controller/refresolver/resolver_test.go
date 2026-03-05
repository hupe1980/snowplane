package refresolver

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(snowplanev1alpha1.AddToScheme(s))

	return s
}

func readyDB(name, namespace, fqn string) *snowplanev1alpha1.Database {
	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       snowplanev1alpha1.DatabaseSpec{Name: "ANALYTICS"},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{FullyQualifiedName: fqn},
		},
	}
	conditions.SetReady(db, "ok")

	return db
}

// --------------------------------------------------------------------------
// Tests: ResolveDatabaseRef
// --------------------------------------------------------------------------

func TestResolveDatabaseRef_Happy(t *testing.T) {
	t.Parallel()

	db := readyDB("my-db", "default", `"ANALYTICS"`)
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()

	fqn, err := ResolveDatabaseRef(context.Background(), c, "default",
		snowplanev1alpha1.ObjectReference{Name: "my-db"})
	require.NoError(t, err)
	assert.Equal(t, `"ANALYTICS"`, fqn)
}

func TestResolveDatabaseRef_NotFound(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveDatabaseRef(context.Background(), c, "default",
		snowplanev1alpha1.ObjectReference{Name: "missing"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotFound)
}

func TestResolveDatabaseRef_NotReady(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
		Spec:       snowplanev1alpha1.DatabaseSpec{Name: "ANALYTICS"},
		Status: snowplanev1alpha1.DatabaseStatus{
			CommonStatus: snowplanev1alpha1.CommonStatus{FullyQualifiedName: `"ANALYTICS"`},
		},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()

	_, err := ResolveDatabaseRef(context.Background(), c, "default",
		snowplanev1alpha1.ObjectReference{Name: "my-db"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotReady)
}

func TestResolveDatabaseRef_EmptyFQN(t *testing.T) {
	t.Parallel()

	db := &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
		Spec:       snowplanev1alpha1.DatabaseSpec{Name: "ANALYTICS"},
	}
	conditions.SetReady(db, "ok")

	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()

	_, err := ResolveDatabaseRef(context.Background(), c, "default",
		snowplanev1alpha1.ObjectReference{Name: "my-db"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotReady)
}

// --------------------------------------------------------------------------
// Tests: ResolveSecretKeyRef
// --------------------------------------------------------------------------

func TestResolveSecretKeyRef_Happy(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("abc123")},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(secret).Build()

	val, err := ResolveSecretKeyRef(context.Background(), c, "default",
		snowplanev1alpha1.SecretKeyReference{Name: "my-secret", Key: "token"})
	require.NoError(t, err)
	assert.Equal(t, "abc123", val)
}

func TestResolveSecretKeyRef_NotFound(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveSecretKeyRef(context.Background(), c, "default",
		snowplanev1alpha1.SecretKeyReference{Name: "missing", Key: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotFound)
}

func TestResolveSecretKeyRef_MissingKey(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"other": []byte("val")},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(secret).Build()

	_, err := ResolveSecretKeyRef(context.Background(), c, "default",
		snowplanev1alpha1.SecretKeyReference{Name: "my-secret", Key: "token"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotFound)
}

func TestResolveSecretKeyRef_CrossNamespace(t *testing.T) {
	t.Parallel()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "secrets-ns"},
		Data:       map[string][]byte{"token": []byte("cross-ns")},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(secret).Build()

	val, err := ResolveSecretKeyRef(context.Background(), c, "default",
		snowplanev1alpha1.SecretKeyReference{Name: "my-secret", Namespace: "secrets-ns", Key: "token"})
	require.NoError(t, err)
	assert.Equal(t, "cross-ns", val)
}

// --------------------------------------------------------------------------
// Tests: ResolveLocalRef non-404 error
// --------------------------------------------------------------------------

func TestResolveLocalRef_Non404Error(t *testing.T) {
	t.Parallel()

	// Use an interceptor to inject a non-NotFound API error.
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
				return fmt.Errorf("internal server error")
			},
		}).
		Build()

	_, err := ResolveDatabaseRef(context.Background(), c, "default",
		snowplanev1alpha1.ObjectReference{Name: "my-db"})
	require.Error(t, err)
	// The error should NOT be ErrReferenceNotFound — it wraps the API error.
	assert.NotErrorIs(t, err, ErrReferenceNotFound)
	assert.Contains(t, err.Error(), "fetching reference")
	assert.Contains(t, err.Error(), "internal server error")
}

// --------------------------------------------------------------------------
// Tests: ResolveDatabaseSource – ErrNeitherRefNorNameSet
// --------------------------------------------------------------------------

func TestResolveDatabaseSource_NeitherRefNorName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveDatabaseSource(context.Background(), c, "default", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNeitherRefNorNameSet)
}

func TestResolveDatabaseSource_EmptyName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	empty := ""

	_, err := ResolveDatabaseSource(context.Background(), c, "default", nil, &empty)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNeitherRefNorNameSet)
}

func TestResolveDatabaseSource_RawName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	name := "ANALYTICS"

	fqn, err := ResolveDatabaseSource(context.Background(), c, "default", nil, &name)
	require.NoError(t, err)
	assert.Equal(t, "ANALYTICS", fqn)
}

// --------------------------------------------------------------------------
// Tests: ResolveSchemaSource – ErrNeitherRefNorNameSet
// --------------------------------------------------------------------------

func TestResolveSchemaSource_NeitherRefNorName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	_, err := ResolveSchemaSource(context.Background(), c, "default", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNeitherRefNorNameSet)
}

func TestResolveSchemaSource_EmptyName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	empty := ""

	_, err := ResolveSchemaSource(context.Background(), c, "default", nil, &empty)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNeitherRefNorNameSet)
}

func TestResolveSchemaSource_RawName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	name := `"ANALYTICS"."PUBLIC"`

	fqn, err := ResolveSchemaSource(context.Background(), c, "default", nil, &name)
	require.NoError(t, err)
	assert.Equal(t, `"ANALYTICS"."PUBLIC"`, fqn)
}

// --------------------------------------------------------------------------
// Tests: Cross-namespace ObjectReference (T-1)
// --------------------------------------------------------------------------

func TestResolveDatabaseRef_CrossNamespace(t *testing.T) {
	t.Parallel()

	// Database lives in "infra" namespace, resource in "team-a"
	db := readyDB("shared-db", "infra", `"SHARED_DB"`)
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()

	// Cross-namespace ref: resource in team-a references DB in infra
	fqn, err := ResolveDatabaseRef(context.Background(), c, "team-a",
		snowplanev1alpha1.ObjectReference{Name: "shared-db", Namespace: "infra"})
	require.NoError(t, err)
	assert.Equal(t, `"SHARED_DB"`, fqn)
}

func TestResolveDatabaseRef_CrossNamespace_NotFound(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithScheme(testScheme()).Build()

	// Cross-namespace ref to non-existent DB
	_, err := ResolveDatabaseRef(context.Background(), c, "team-a",
		snowplanev1alpha1.ObjectReference{Name: "missing-db", Namespace: "infra"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReferenceNotFound)
	// Error message should reference the infra namespace, not team-a
	assert.Contains(t, err.Error(), "infra")
}

func TestResolveDatabaseRef_SameNamespace_FallsBack(t *testing.T) {
	t.Parallel()

	// When Namespace is empty, falls back to the resource's own namespace
	db := readyDB("local-db", "team-a", `"LOCAL_DB"`)
	c := fake.NewClientBuilder().WithScheme(testScheme()).
		WithRuntimeObjects(db).
		WithStatusSubresource(&snowplanev1alpha1.Database{}).
		Build()

	fqn, err := ResolveDatabaseRef(context.Background(), c, "team-a",
		snowplanev1alpha1.ObjectReference{Name: "local-db"}) // No Namespace set
	require.NoError(t, err)
	assert.Equal(t, `"LOCAL_DB"`, fqn)
}
