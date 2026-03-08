//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestExternalVolume_CreateLifecycle(t *testing.T) {
	resetMocks()

	evK8s := "extvolume-create-test"
	sfEV := "EXTVOLUME_CREATE_TEST"

	var created atomic.Bool

	externalVolumeMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error) {
		if id.Name() == sfEV && created.Load() {
			return externalVolumeObservation(sfEV, "", false), nil
		}

		return &snowflake.ExternalVolumeObservation{Exists: false}, nil
	})

	externalVolumeMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateExternalVolumeOptions) error {
		assert.Equal(t, sfEV, opts.Name.Name())
		assert.Len(t, opts.StorageLocations, 1)
		assert.Equal(t, "loc1", opts.StorageLocations[0].Name)
		assert.Equal(t, "S3", opts.StorageLocations[0].StorageProvider)
		assert.Equal(t, "s3://my-bucket/path/", opts.StorageLocations[0].StorageBaseURL)
		created.Store(true)

		return nil
	})

	ev := newTestExternalVolume(evK8s, sfEV)
	require.NoError(t, k8sClient.Create(ctx, ev))

	key := types.NamespacedName{Name: evK8s, Namespace: testNamespace}

	t.Cleanup(func() {
		externalVolumeMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ExternalVolume{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "external volume should become Ready")

	var result snowplanev1alpha1.ExternalVolume
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load())
	assert.NotNil(t, result.Status.ShowOutput)
	assert.Equal(t, sfEV, result.Status.ShowOutput.Name)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/externalvolume")
}

func TestExternalVolume_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	evK8s := "extvolume-orphan-test"
	sfEV := "EXTVOLUME_ORPHAN_TEST"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	externalVolumeMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error) {
		if id.Name() == sfEV && created.Load() {
			return externalVolumeObservation(sfEV, "", false), nil
		}

		return &snowflake.ExternalVolumeObservation{Exists: false}, nil
	})

	externalVolumeMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateExternalVolumeOptions) error {
		created.Store(true)
		return nil
	})

	externalVolumeMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	ev := newTestExternalVolume(evK8s, sfEV)
	ev.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, ev))

	key := types.NamespacedName{Name: evK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.ExternalVolume
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalVolume
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestExternalVolume_UpdateComment(t *testing.T) {
	resetMocks()

	evK8s := "extvolume-alter-test"
	sfEV := "EXTVOLUME_ALTER_TEST"

	var (
		created    atomic.Bool
		altered    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	externalVolumeMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.ExternalVolumeObservation, error) {
		if id.Name() == sfEV && created.Load() {
			return externalVolumeObservation(sfEV, curComment.Load().(string), false), nil
		}

		return &snowflake.ExternalVolumeObservation{Exists: false}, nil
	})

	externalVolumeMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateExternalVolumeOptions) error {
		created.Store(true)
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	externalVolumeMockSvc.SetAlter(func(_ context.Context, opts snowflake.AlterExternalVolumeOptions) error {
		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
			altered.Store(true)
		}

		return nil
	})

	ev := newTestExternalVolume(evK8s, sfEV)
	initComment := "initial comment"
	ev.Spec.Comment = &initComment
	f := false
	ev.Spec.ManagementPolicies.CreateOrAlter = &f

	require.NoError(t, k8sClient.Create(ctx, ev))

	key := types.NamespacedName{Name: evK8s, Namespace: testNamespace}

	t.Cleanup(func() {
		externalVolumeMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.ExternalVolume{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment
	var current snowplanev1alpha1.ExternalVolume
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	require.Eventually(t, func() bool {
		return altered.Load()
	}, defaultTimeout, defaultInterval, "ALTER should have been called")

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.ExternalVolume
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")
}
