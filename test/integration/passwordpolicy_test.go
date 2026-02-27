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

func TestPasswordPolicy_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pp-db", "PP_DB", "pp-schema", "PP_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	passwordPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
		if id.Name() == "MY_PP" && created.Load() {
			return passwordPolicyObservation("MY_PP", "PP_DB", "PP_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.PasswordPolicyObservation{Exists: false}, nil
	})

	passwordPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreatePasswordPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestPasswordPolicy("test-pp", "MY_PP", "pp-db", "pp-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-pp", Namespace: testNamespace}

	t.Cleanup(func() {
		passwordPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.PasswordPolicy
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.PasswordPolicy{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.PasswordPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "password policy should become Ready")

	var obj snowplanev1alpha1.PasswordPolicy
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_PP", obj.Status.ShowOutput.Name)
}

func TestPasswordPolicy_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"pp-orphan-db", "PP_ORPHAN_DB", "pp-orphan-schema", "PP_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	passwordPolicyMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.PasswordPolicyObservation, error) {
		if id.Name() == "MY_PP_ORPHAN" && created.Load() {
			return passwordPolicyObservation("MY_PP_ORPHAN", "PP_ORPHAN_DB", "PP_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.PasswordPolicyObservation{Exists: false}, nil
	})

	passwordPolicyMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreatePasswordPolicyOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestPasswordPolicy("test-pp-orphan", "MY_PP_ORPHAN", "pp-orphan-db", "pp-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-pp-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.PasswordPolicy
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	passwordPolicyMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.PasswordPolicy{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
