//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)


// ---------------------------------------------------------------------------
// Secret tests
// ---------------------------------------------------------------------------

func TestSecretWithAuthorizationCodeGrant_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sacg-db", "SACG_DB", "sacg-schema", "SACG_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	secretAuthCodeGrantMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
		if id.Name() == "MY_SECRET_ACG" && created.Load() {
			return secretObservation("MY_SECRET_ACG", "SACG_DB", "SACG_SCHEMA"), nil
		}
		return &snowflake.SecretObservation{Exists: false}, nil
	})
	secretAuthCodeGrantMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSecretOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSecretWithAuthCodeGrant("test-secret-acg", "MY_SECRET_ACG", "sacg-db", "sacg-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		secretAuthCodeGrantMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.SecretWithAuthorizationCodeGrant
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecretWithAuthorizationCodeGrant{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecretWithAuthorizationCodeGrant
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SecretWithAuthorizationCodeGrant should become Ready")
}

func TestSecretWithBasicAuthentication_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sba-db", "SBA_DB", "sba-schema", "SBA_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	secretBasicAuthMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
		if id.Name() == "MY_SECRET_BA" && created.Load() {
			return secretObservation("MY_SECRET_BA", "SBA_DB", "SBA_SCHEMA"), nil
		}
		return &snowflake.SecretObservation{Exists: false}, nil
	})
	secretBasicAuthMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSecretOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSecretWithBasicAuth("test-secret-ba", "MY_SECRET_BA", "sba-db", "sba-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		secretBasicAuthMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.SecretWithBasicAuthentication
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecretWithBasicAuthentication{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecretWithBasicAuthentication
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SecretWithBasicAuthentication should become Ready")
}

func TestSecretWithClientCredentials_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"scc-db", "SCC_DB", "scc-schema", "SCC_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	secretClientCredsMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
		if id.Name() == "MY_SECRET_CC" && created.Load() {
			return secretObservation("MY_SECRET_CC", "SCC_DB", "SCC_SCHEMA"), nil
		}
		return &snowflake.SecretObservation{Exists: false}, nil
	})
	secretClientCredsMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSecretOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSecretWithClientCreds("test-secret-cc", "MY_SECRET_CC", "scc-db", "scc-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		secretClientCredsMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.SecretWithClientCredentials
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecretWithClientCredentials{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecretWithClientCredentials
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SecretWithClientCredentials should become Ready")
}

func TestSecretWithGenericString_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sgs-db", "SGS_DB", "sgs-schema", "SGS_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	secretGenericStringMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.SecretObservation, error) {
		if id.Name() == "MY_SECRET_GS" && created.Load() {
			return secretObservation("MY_SECRET_GS", "SGS_DB", "SGS_SCHEMA"), nil
		}
		return &snowflake.SecretObservation{Exists: false}, nil
	})
	secretGenericStringMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSecretOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSecretWithGenericString("test-secret-gs", "MY_SECRET_GS", "sgs-db", "sgs-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		secretGenericStringMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.SecretWithGenericString
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.SecretWithGenericString{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.SecretWithGenericString
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "SecretWithGenericString should become Ready")
}


// ---------------------------------------------------------------------------
// Stream tests
// ---------------------------------------------------------------------------

func TestStreamOnDirectoryTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sdt-db", "SDT_DB", "sdt-schema", "SDT_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	streamOnDirectoryTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
		if id.Name() == "MY_STREAM_DT" && created.Load() {
			return streamObservation("MY_STREAM_DT", "SDT_DB", "SDT_SCHEMA"), nil
		}
		return &snowflake.StreamObservation{Exists: false}, nil
	})
	streamOnDirectoryTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestStreamOnDirectoryTable("test-stream-dt", "MY_STREAM_DT", "sdt-db", "sdt-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		streamOnDirectoryTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.StreamOnDirectoryTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StreamOnDirectoryTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StreamOnDirectoryTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "StreamOnDirectoryTable should become Ready")
}

func TestStreamOnDynamicTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sdyn-db", "SDYN_DB", "sdyn-schema", "SDYN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	streamOnDynamicTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
		if id.Name() == "MY_STREAM_DYN" && created.Load() {
			return streamObservation("MY_STREAM_DYN", "SDYN_DB", "SDYN_SCHEMA"), nil
		}
		return &snowflake.StreamObservation{Exists: false}, nil
	})
	streamOnDynamicTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestStreamOnDynamicTable("test-stream-dyn", "MY_STREAM_DYN", "sdyn-db", "sdyn-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		streamOnDynamicTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.StreamOnDynamicTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StreamOnDynamicTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StreamOnDynamicTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "StreamOnDynamicTable should become Ready")
}

func TestStreamOnExternalTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"set-db", "SET_DB", "set-schema", "SET_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	streamOnExternalTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
		if id.Name() == "MY_STREAM_ET" && created.Load() {
			return streamObservation("MY_STREAM_ET", "SET_DB", "SET_SCHEMA"), nil
		}
		return &snowflake.StreamObservation{Exists: false}, nil
	})
	streamOnExternalTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestStreamOnExternalTable("test-stream-et", "MY_STREAM_ET", "set-db", "set-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		streamOnExternalTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.StreamOnExternalTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StreamOnExternalTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StreamOnExternalTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "StreamOnExternalTable should become Ready")
}

func TestStreamOnTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"st-db", "ST_DB", "st-schema", "ST_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	streamOnTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
		if id.Name() == "MY_STREAM_T" && created.Load() {
			return streamObservation("MY_STREAM_T", "ST_DB", "ST_SCHEMA"), nil
		}
		return &snowflake.StreamObservation{Exists: false}, nil
	})
	streamOnTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestStreamOnTable("test-stream-t", "MY_STREAM_T", "st-db", "st-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		streamOnTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.StreamOnTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StreamOnTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StreamOnTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "StreamOnTable should become Ready")
}

func TestStreamOnView_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"sv-db", "SV_DB", "sv-schema", "SV_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	streamOnViewMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
		if id.Name() == "MY_STREAM_V" && created.Load() {
			return streamObservation("MY_STREAM_V", "SV_DB", "SV_SCHEMA"), nil
		}
		return &snowflake.StreamObservation{Exists: false}, nil
	})
	streamOnViewMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestStreamOnView("test-stream-v", "MY_STREAM_V", "sv-db", "sv-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		streamOnViewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.StreamOnView
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.StreamOnView{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.StreamOnView
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "StreamOnView should become Ready")
}


// ---------------------------------------------------------------------------
// Other schema-scoped resources
// ---------------------------------------------------------------------------

func TestExternalTable_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ext-db", "EXT_DB", "ext-schema", "EXT_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	externalTableMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error) {
		if id.Name() == "MY_EXT_TBL" && created.Load() {
			return externalTableObservation("MY_EXT_TBL", "EXT_DB", "EXT_SCHEMA"), nil
		}
		return &snowflake.ExternalTableObservation{Exists: false}, nil
	})
	externalTableMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateExternalTableOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestExternalTable("test-ext-tbl", "MY_EXT_TBL", "ext-db", "ext-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		externalTableMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.ExternalTable
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ExternalTable{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalTable
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "ExternalTable should become Ready")
}

func TestMaterializedView_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"mv-db", "MV_DB", "mv-schema", "MV_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	materializedViewMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.MaterializedViewObservation, error) {
		if id.Name() == "MY_MV" && created.Load() {
			return materializedViewObservation("MY_MV", "MV_DB", "MV_SCHEMA"), nil
		}
		return &snowflake.MaterializedViewObservation{Exists: false}, nil
	})
	materializedViewMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateMaterializedViewOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestMaterializedView("test-mv", "MY_MV", "mv-db", "mv-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		materializedViewMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.MaterializedView
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.MaterializedView{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.MaterializedView
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "MaterializedView should become Ready")
}

func TestNetworkRule_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"nr-db", "NR_DB", "nr-schema", "NR_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	networkRuleMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.NetworkRuleObservation, error) {
		if id.Name() == "MY_NR" && created.Load() {
			return networkRuleObservation("MY_NR", "NR_DB", "NR_SCHEMA"), nil
		}
		return &snowflake.NetworkRuleObservation{Exists: false}, nil
	})
	networkRuleMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateNetworkRuleOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestNetworkRule("test-nr", "MY_NR", "nr-db", "nr-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		networkRuleMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.NetworkRule
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.NetworkRule{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.NetworkRule
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "NetworkRule should become Ready")
}

func TestSequence_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"seq-db", "SEQ_DB", "seq-schema", "SEQ_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	sequenceMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.SequenceObservation, error) {
		if id.Name() == "MY_SEQ" && created.Load() {
			return sequenceObservation("MY_SEQ", "SEQ_DB", "SEQ_SCHEMA"), nil
		}
		return &snowflake.SequenceObservation{Exists: false}, nil
	})
	sequenceMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateSequenceOptions) error {
		created.Store(true)
		return nil
	})

	cr := newTestSequence("test-seq", "MY_SEQ", "seq-db", "seq-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
	t.Cleanup(func() {
		sequenceMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
		var obj snowplanev1alpha1.Sequence
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.Sequence{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Sequence
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}
		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "Sequence should become Ready")
}
