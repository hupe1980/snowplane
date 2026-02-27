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

func TestTask_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"task-db", "TASK_DB", "task-schema", "TASK_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	taskMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
		if id.Name() == "MY_TASK" && created.Load() {
			return taskObservation("MY_TASK", "TASK_DB", "TASK_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.TaskObservation{Exists: false}, nil
	})

	taskMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateTaskOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestTask("test-task", "MY_TASK", "task-db", "task-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-task", Namespace: testNamespace}

	t.Cleanup(func() {
		taskMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.Task
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.Task{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Task
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "task should become Ready")

	var obj snowplanev1alpha1.Task
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_TASK", obj.Status.ShowOutput.Name)
}

func TestTask_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"task-orphan-db", "TASK_ORPHAN_DB", "task-orphan-schema", "TASK_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	taskMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.TaskObservation, error) {
		if id.Name() == "MY_TASK_ORPHAN" && created.Load() {
			return taskObservation("MY_TASK_ORPHAN", "TASK_ORPHAN_DB", "TASK_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.TaskObservation{Exists: false}, nil
	})

	taskMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateTaskOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestTask("test-task-orphan", "MY_TASK_ORPHAN", "task-orphan-db", "task-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-task-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Task
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	taskMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Task{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
