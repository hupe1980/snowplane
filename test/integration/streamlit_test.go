//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestStreamlit_CreateLifecycle(t *testing.T) {
	resetMocks()

	dbK8s := "streamlit-create-db"
	sfDB := "STREAMLIT_CREATE_DB"
	schemaK8s := "streamlit-create-schema"
	sfSchema := "STREAMLIT_CREATE_SCHEMA"
	stK8s := "streamlit-create-test"
	sfSt := "STREAMLIT_CREATE_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var created atomic.Bool

	streamlitMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error) {
		if created.Load() {
			return streamlitObservation(sfSt, sfDB, sfSchema, "SYSADMIN", "main.py"), nil
		}

		return &snowflake.StreamlitObservation{Exists: false}, nil
	})

	streamlitMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStreamlitOptions) error {
		assert.Equal(t, sfSt, opts.Name.Name())
		assert.Equal(t, sfDB, opts.Name.DatabaseName())
		assert.Equal(t, sfSchema, opts.Name.SchemaName())
		assert.NotNil(t, opts.MainFile)
		assert.Equal(t, "main.py", *opts.MainFile)
		created.Store(true)

		return nil
	})

	st := newTestStreamlit(stK8s, sfSt, dbK8s, schemaK8s)
	require.NoError(t, k8sClient.Create(ctx, st))

	key := types.NamespacedName{Name: stK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "streamlit should become Ready")

	var result snowplanev1alpha1.Streamlit
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load())
	assert.Equal(t, sfSt, result.Status.ShowOutput.Name)
	assert.Equal(t, sfDB, result.Status.DatabaseName)
	assert.Equal(t, sfSchema, result.Status.SchemaName)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/streamlit")

	refCond := conditions.Get(&result, snowplanev1alpha1.TypeReferencesResolved)
	assert.NotNil(t, refCond)
	assert.Equal(t, metav1.ConditionTrue, refCond.Status)

	// Cleanup
	streamlitMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "streamlit should be cleaned up")
}

func TestStreamlit_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	dbK8s := "streamlit-orphan-db"
	sfDB := "STREAMLIT_ORPHAN_DB"
	schemaK8s := "streamlit-orphan-schema"
	sfSchema := "STREAMLIT_ORPHAN_SCHEMA"
	stK8s := "streamlit-orphan-test"
	sfSt := "STREAMLIT_ORPHAN_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	streamlitMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error) {
		if created.Load() {
			return streamlitObservation(sfSt, sfDB, sfSchema, "SYSADMIN", "main.py"), nil
		}

		return &snowflake.StreamlitObservation{Exists: false}, nil
	})

	streamlitMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateStreamlitOptions) error {
		created.Store(true)
		return nil
	})

	streamlitMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	st := newTestStreamlit(stK8s, sfSt, dbK8s, schemaK8s)
	st.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, st))

	key := types.NamespacedName{Name: stK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Streamlit
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestStreamlit_UpdateComment(t *testing.T) {
	resetMocks()

	dbK8s := "streamlit-alter-db"
	sfDB := "STREAMLIT_ALTER_DB"
	schemaK8s := "streamlit-alter-schema"
	sfSchema := "STREAMLIT_ALTER_SCHEMA"
	stK8s := "streamlit-alter-test"
	sfSt := "STREAMLIT_ALTER_TEST"

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t, dbK8s, sfDB, schemaK8s, sfSchema)
	defer cleanupParents()

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	streamlitMockSvc.SetObserve(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error) {
		if created.Load() {
			obs := streamlitObservation(sfSt, sfDB, sfSchema, "SYSADMIN", "main.py")
			obs.ShowOutput.Comment = curComment.Load().(string)

			return obs, nil
		}

		return &snowflake.StreamlitObservation{Exists: false}, nil
	})

	streamlitMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateStreamlitOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	streamlitMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterStreamlitOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	st := newTestStreamlit(stK8s, sfSt, dbK8s, schemaK8s)
	initComment := "initial streamlit comment"
	st.Spec.Comment = &initComment
	f := false
	st.Spec.ManagementPolicies.CreateOrAlter = &f

	require.NoError(t, k8sClient.Create(ctx, st))

	key := types.NamespacedName{Name: stK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment
	var current snowplanev1alpha1.Streamlit
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated streamlit comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated streamlit comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup
	streamlitMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Streamlit
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
