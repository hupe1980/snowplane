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

func TestFileFormat_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ff-db", "FF_DB", "ff-schema", "FF_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	fileFormatMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
		if id.Name() == "MY_FF" && created.Load() {
			return fileFormatObservation("MY_FF", "FF_DB", "FF_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.FileFormatObservation{Exists: false}, nil
	})

	fileFormatMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFileFormatOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestFileFormat("test-ff", "MY_FF", "ff-db", "ff-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ff", Namespace: testNamespace}

	t.Cleanup(func() {
		fileFormatMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.FileFormat
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.FileFormat{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FileFormat
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "file format should become Ready")

	var obj snowplanev1alpha1.FileFormat
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_FF", obj.Status.ShowOutput.Name)
}

func TestFileFormat_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ff-orphan-db", "FF_ORPHAN_DB", "ff-orphan-schema", "FF_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	fileFormatMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.FileFormatObservation, error) {
		if id.Name() == "MY_FF_ORPHAN" && created.Load() {
			return fileFormatObservation("MY_FF_ORPHAN", "FF_ORPHAN_DB", "FF_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.FileFormatObservation{Exists: false}, nil
	})

	fileFormatMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateFileFormatOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestFileFormat("test-ff-orphan", "MY_FF_ORPHAN", "ff-orphan-db", "ff-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ff-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.FileFormat
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	fileFormatMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.FileFormat{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
