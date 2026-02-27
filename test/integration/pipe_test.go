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

func TestPipe_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pipe-db", "PIPE_DB", "pipe-schema", "PIPE_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	pipeMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error) {
		if id.Name() == "MY_PIPE" && created.Load() {
			return pipeObservation("MY_PIPE", "PIPE_DB", "PIPE_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.PipeObservation{Exists: false}, nil
	})

	pipeMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreatePipeOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestPipe("test-pipe", "MY_PIPE", "pipe-db", "pipe-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-pipe", Namespace: testNamespace}

	t.Cleanup(func() {
		pipeMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.Pipe
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.Pipe{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Pipe
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "pipe should become Ready")

	var obj snowplanev1alpha1.Pipe
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_PIPE", obj.Status.ShowOutput.Name)
}

func TestPipe_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pipe-orphan-db", "PIPE_ORPHAN_DB", "pipe-orphan-schema", "PIPE_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	pipeMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.PipeObservation, error) {
		if id.Name() == "MY_PIPE_ORPHAN" && created.Load() {
			return pipeObservation("MY_PIPE_ORPHAN", "PIPE_ORPHAN_DB", "PIPE_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.PipeObservation{Exists: false}, nil
	})

	pipeMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreatePipeOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestPipe("test-pipe-orphan", "MY_PIPE_ORPHAN", "pipe-orphan-db", "pipe-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-pipe-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Pipe
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	pipeMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Pipe{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
