package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/testutil"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// newTestDBWithUID creates a test Database with the given name, namespace, and a fixed UID.
func newTestDBWithUID(name, namespace, uid string) *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			UID:        types.UID(uid),
			Generation: 1,
		},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default-pc"},
			},
			Name: "SHARED_DB",
		},
	}
}

func newOwnershipReconciler(adapter *mockAdapter, objs ...runtime.Object) *reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any] {
	scheme := testutil.TestScheme()
	cb := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&snowplanev1alpha1.Database{}, &snowplanev1alpha1.ProviderConfig{})
	for _, obj := range objs {
		cb = cb.WithRuntimeObjects(obj)
	}
	c := cb.Build()
	factory := clientfactory.NewTestClientFactoryWithFn(func(_ snowflake.Config) (clientfactory.SnowflakeClient, error) {
		return &mockSnowflakeClient{}, nil
	})

	return &reconciler.GenericReconciler[*snowplanev1alpha1.Database, any, any]{
		Client:   c,
		Factory:  factory,
		Recorder: record.NewFakeRecorder(100),
		Adapter:  adapter,
		GVK:      snowplanev1alpha1.GroupVersion.WithKind("Database"),
	}
}

// ---------------------------------------------------------------------------
// Tests: Ownership conflict detection (H-3)
// ---------------------------------------------------------------------------

func TestReconcile_Adoption_OwnershipConflict_Rejected(t *testing.T) {
	t.Parallel()

	// CR-1 already adopted → has the external-name-hash label.
	existingDB := newTestDBWithUID("owner-db", "default", "uid-owner")
	existingDB.Finalizers = []string{"snowplane.test/database"}
	existingDB.Labels = map[string]string{
		snowplanev1alpha1.LabelExternalNameHash: reconciler.ComputeExternalNameHash("test-id"),
	}
	// Mark as already reconciled so it doesn't enter adoption path itself.
	existingDB.Status.ObservedGeneration = 1

	// CR-2 tries to adopt the same Snowflake resource.
	newDB := newTestDBWithUID("duplicate-db", "default", "uid-duplicate")
	newDB.Finalizers = []string{"snowplane.test/database"}
	newDB.Spec.ManagementPolicies.AdoptionPolicy = snowplanev1alpha1.AdoptionPolicyTypeAdopt

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}

	r := newOwnershipReconciler(adapter, existingDB, newDB,
		testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("duplicate-db", "default"))
	require.NoError(t, err, "Conflict detection should not return an error")
	assert.Zero(t, result.RequeueAfter, "Should not requeue — terminal ConflictDetected")

	// CR-2 should have ConflictDetected condition.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(),
		testutil.ReconcileReq("duplicate-db", "default").NamespacedName, &fetched))
	tc := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonConflictDetected, tc.Reason)
	assert.Contains(t, tc.Message, "ownership conflict")
	assert.Contains(t, tc.Message, "owner-db")

	// ApplyObservation should NOT have been called (adoption was blocked).
	assert.Zero(t, adapter.applyObservationCalled, "Adoption should be blocked before ApplyObservation")
}

func TestReconcile_Adoption_NoConflict_Succeeds(t *testing.T) {
	t.Parallel()

	// Only one CR exists — no conflict.
	db := newTestDBWithUID("solo-db", "default", "uid-solo")
	db.Finalizers = []string{"snowplane.test/database"}
	db.Spec.ManagementPolicies.AdoptionPolicy = snowplanev1alpha1.AdoptionPolicyTypeAdopt

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}

	r := newOwnershipReconciler(adapter, db,
		testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("solo-db", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter, "Should requeue normally after adoption")

	// Verify adoption succeeded — ApplyObservation was called.
	assert.GreaterOrEqual(t, adapter.applyObservationCalled, 1, "ApplyObservation should be called")
	assert.Equal(t, 1, adapter.postCreateCalled)

	// Verify the external-name-hash label was stamped.
	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(),
		testutil.ReconcileReq("solo-db", "default").NamespacedName, &fetched))
	assert.NotEmpty(t, fetched.Labels[snowplanev1alpha1.LabelExternalNameHash],
		"External name hash label should be set after adoption")
}

func TestReconcile_Adoption_SameUIDRetry_Succeeds(t *testing.T) {
	t.Parallel()

	// Simulate re-adoption of the same CR (e.g., controller restart).
	// The CR already has the label from a previous partial adoption attempt.
	db := newTestDBWithUID("retry-db", "default", "uid-retry")
	db.Finalizers = []string{"snowplane.test/database"}
	db.Spec.ManagementPolicies.AdoptionPolicy = snowplanev1alpha1.AdoptionPolicyTypeAdopt
	db.Labels = map[string]string{
		snowplanev1alpha1.LabelExternalNameHash: reconciler.ComputeExternalNameHash("test-id"),
	}

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}

	r := newOwnershipReconciler(adapter, db,
		testutil.NewTestPC("default"), testutil.NewTestSecret("default"))

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("retry-db", "default"))
	require.NoError(t, err)
	assert.Equal(t, reconciler.DefaultRequeueInterval, result.RequeueAfter, "retry of same CR should succeed")
	assert.GreaterOrEqual(t, adapter.applyObservationCalled, 1)
}

func TestReconcile_Adoption_CrossNamespaceConflict_Rejected(t *testing.T) {
	t.Parallel()

	// CR-1 in namespace "default" already owns the resource.
	existingDB := newTestDBWithUID("owner-ns-db", "default", "uid-ns-owner")
	existingDB.Finalizers = []string{"snowplane.test/database"}
	existingDB.Labels = map[string]string{
		snowplanev1alpha1.LabelExternalNameHash: reconciler.ComputeExternalNameHash("test-id"),
	}
	existingDB.Status.ObservedGeneration = 1

	// CR-2 in the same namespace tries to adopt the same Snowflake resource
	// (simulates cross-namespace — label-based conflict detection is namespace-agnostic).
	newDB := newTestDBWithUID("conflict-ns-db", "default", "uid-ns-conflict")
	newDB.Finalizers = []string{"snowplane.test/database"}
	newDB.Spec.ManagementPolicies.AdoptionPolicy = snowplanev1alpha1.AdoptionPolicyTypeAdopt

	adapter := &mockAdapter{
		observeFn: func(_ context.Context, _ any, _ reconciler.Identifier) (*reconciler.Observation[any], error) {
			return &reconciler.Observation[any]{Exists: true, Detail: "observed"}, nil
		},
	}

	r := newOwnershipReconciler(adapter, existingDB, newDB,
		testutil.NewTestPC("default"), testutil.NewTestSecret("default"),
	)

	result, err := r.Reconcile(context.Background(), testutil.ReconcileReq("conflict-ns-db", "default"))
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter, "Conflict should be terminal")

	var fetched snowplanev1alpha1.Database
	require.NoError(t, r.Client.Get(context.Background(),
		testutil.ReconcileReq("conflict-ns-db", "default").NamespacedName, &fetched))
	tc := conditions.Get(&fetched, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, snowplanev1alpha1.ReasonConflictDetected, tc.Reason)
}

func TestComputeExternalNameHash_Deterministic(t *testing.T) {
	t.Parallel()

	hash1 := reconciler.ComputeExternalNameHash("MY_DB.MY_SCHEMA.MY_TABLE")
	hash2 := reconciler.ComputeExternalNameHash("MY_DB.MY_SCHEMA.MY_TABLE")
	assert.Equal(t, hash1, hash2, "Same FQN should produce same hash")
	assert.Len(t, hash1, 16, "Hash should be 16 hex chars")

	hash3 := reconciler.ComputeExternalNameHash("OTHER_DB.OTHER_SCHEMA.OTHER_TABLE")
	assert.NotEqual(t, hash1, hash3, "Different FQNs should produce different hashes")
}
