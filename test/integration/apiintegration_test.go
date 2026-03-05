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

func TestAPIIntegration_CreateLifecycle(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	apiIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
		if id.Name() == "MY_APII" && created.Load() {
			return apiIntegrationObservation("MY_APII"), nil
		}

		return &snowflake.APIIntegrationObservation{Exists: false}, nil
	})

	apiIntegrationMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateAPIIntegrationOptions) error {
		assert.Equal(t, "MY_APII", opts.Name.Name())
		assert.Equal(t, "aws_api_gateway", opts.APIProvider)
		assert.Equal(t, []string{"https://example.com/"}, opts.APIAllowedPrefixes)
		created.Store(true)

		return nil
	})

	cr := newTestAPIIntegration("test-apii", "MY_APII")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-apii", Namespace: testNamespace}

	t.Cleanup(func() {
		apiIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.APIIntegration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIIntegration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "api integration should become Ready")

	var obj snowplanev1alpha1.APIIntegration
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_APII", obj.Status.ShowOutput.Name)
	require.Equal(t, "EXTERNAL_API", obj.Status.ShowOutput.Type)
	require.NotEmpty(t, obj.Status.FullyQualifiedName)
}

func TestAPIIntegration_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	var created atomic.Bool
	var alterCalled atomic.Bool

	apiIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
		if id.Name() == "MY_APII_ALTER" && created.Load() {
			return apiIntegrationObservation("MY_APII_ALTER"), nil
		}

		return &snowflake.APIIntegrationObservation{Exists: false}, nil
	})

	apiIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAPIIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	apiIntegrationMockSvc.SetAlter(func(_ context.Context, _ snowflake.AlterAPIIntegrationOptions) error {
		alterCalled.Store(true)

		return nil
	})

	cr := newTestAPIIntegration("test-apii-alter", "MY_APII_ALTER")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-apii-alter", Namespace: testNamespace}

	t.Cleanup(func() {
		apiIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.APIIntegration
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIIntegration{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	// Update comment to trigger alter.
	var obj snowplanev1alpha1.APIIntegration
	require.NoError(t, k8sClient.Get(ctx, key, &obj))

	newComment := "updated comment"
	obj.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &obj))

	require.Eventually(t, func() bool {
		return alterCalled.Load()
	}, defaultTimeout, defaultInterval, "alter should have been called")
}

func TestAPIIntegration_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	var created atomic.Bool

	apiIntegrationMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
		if id.Name() == "MY_APII_ORPHAN" && created.Load() {
			return apiIntegrationObservation("MY_APII_ORPHAN"), nil
		}

		return &snowflake.APIIntegrationObservation{Exists: false}, nil
	})

	apiIntegrationMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAPIIntegrationOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestAPIIntegration("test-apii-orphan", "MY_APII_ORPHAN")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-apii-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.APIIntegration
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	apiIntegrationMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.APIIntegration{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
