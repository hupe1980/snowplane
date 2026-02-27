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

func TestTag_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"tag-db", "TAG_DB", "tag-schema", "TAG_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	tagMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
		if id.Name() == "MY_TAG" && created.Load() {
			return tagObservation("MY_TAG", "TAG_DB", "TAG_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.TagObservation{Exists: false}, nil
	})

	tagMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateTagOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestTag("test-tag", "MY_TAG", "tag-db", "tag-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-tag", Namespace: testNamespace}

	t.Cleanup(func() {
		tagMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.Tag
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.Tag{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Tag
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "tag should become Ready")

	var obj snowplanev1alpha1.Tag
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_TAG", obj.Status.ShowOutput.Name)
}

func TestTag_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"tag-orphan-db", "TAG_ORPHAN_DB", "tag-orphan-schema", "TAG_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	tagMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.TagObservation, error) {
		if id.Name() == "MY_TAG_ORPHAN" && created.Load() {
			return tagObservation("MY_TAG_ORPHAN", "TAG_ORPHAN_DB", "TAG_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.TagObservation{Exists: false}, nil
	})

	tagMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateTagOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestTag("test-tag-orphan", "MY_TAG_ORPHAN", "tag-orphan-db", "tag-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-tag-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Tag
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	tagMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Tag{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
