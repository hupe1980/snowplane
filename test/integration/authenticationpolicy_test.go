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

func TestAuthenticationPolicy_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ap-db", "AP_DB", "ap-schema", "AP_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	authenticationPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
		if id.Name() == "MY_AP" && created.Load() {
			return authenticationPolicyObservation("MY_AP", "AP_DB", "AP_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
	})

	authenticationPolicyMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateAuthenticationPolicyOptions) error {
		require.Equal(t, "MY_AP", opts.Name.Name())
		require.Equal(t, []string{"PASSWORD", "SAML"}, opts.AuthenticationMethods)
		require.Equal(t, []string{"SNOWFLAKE_UI", "DRIVERS"}, opts.ClientTypes)
		created.Store(true)

		return nil
	})

	cr := newTestAuthenticationPolicy("test-ap", "MY_AP", "ap-db", "ap-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ap", Namespace: testNamespace}

	t.Cleanup(func() {
		authenticationPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.AuthenticationPolicy
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.AuthenticationPolicy{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AuthenticationPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "authentication policy should become Ready")

	var obj snowplanev1alpha1.AuthenticationPolicy
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_AP", obj.Status.ShowOutput.Name)
	require.Equal(t, "AP_DB", obj.Status.ShowOutput.DatabaseName)
	require.Equal(t, "AP_SCHEMA", obj.Status.ShowOutput.SchemaName)
}

func TestAuthenticationPolicy_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"ap-orphan-db", "AP_ORPHAN_DB", "ap-orphan-schema", "AP_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	authenticationPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.AuthenticationPolicyObservation, error) {
		if id.Name() == "MY_AP_ORPHAN" && created.Load() {
			return authenticationPolicyObservation("MY_AP_ORPHAN", "AP_ORPHAN_DB", "AP_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.AuthenticationPolicyObservation{Exists: false}, nil
	})

	authenticationPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAuthenticationPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestAuthenticationPolicy("test-ap-orphan", "MY_AP_ORPHAN", "ap-orphan-db", "ap-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-ap-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.AuthenticationPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	authenticationPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.AuthenticationPolicy{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
