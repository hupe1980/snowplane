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

func TestGrant_CreateLifecycle(t *testing.T) {
	resetMocks()

	grantK8s := "grant-create-test"
	privilege := "USAGE"
	objectType := "DATABASE"
	objectName := "MY_DB"
	toRole := "DATA_READER"

	var granted atomic.Bool

	grantMockSvc.SetObserve(func(_ context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
		if granted.Load() {
			return grantObservation(privilege, objectType, objectName, "ROLE", toRole, false), nil
		}

		return &snowflake.GrantObservation{Exists: false}, nil
	})

	grantMockSvc.SetGrant(func(_ context.Context, opts snowflake.CreateGrantOptions) error {
		assert.Equal(t, privilege, opts.Privilege)
		assert.Contains(t, opts.OnClause, objectName)
		assert.Contains(t, opts.ToClause, toRole)
		granted.Store(true)

		return nil
	})

	grant := newTestGrant(grantK8s, privilege, objectType, objectName, toRole)
	require.NoError(t, k8sClient.Create(ctx, grant))

	key := types.NamespacedName{Name: grantK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "grant should become Ready")

	var result snowplanev1alpha1.GrantPrivilegesToAccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, granted.Load(), "Snowflake GRANT should have been called")
	assert.Equal(t, privilege, result.Status.ShowOutput.Privilege)
	assert.Equal(t, objectName, result.Status.ShowOutput.Name)
	assert.Equal(t, toRole, result.Status.ShowOutput.GranteeName)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/grantprivilegestoaccountrole")

	// Cleanup — revoke on delete.
	grantMockSvc.SetRevoke(func(_ context.Context, opts snowflake.RevokeGrantOptions) error {
		assert.Equal(t, privilege, opts.Privilege)
		assert.Contains(t, opts.FromClause, toRole)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "grant should be cleaned up")
}

func TestGrant_WithGrantOption(t *testing.T) {
	resetMocks()

	grantK8s := "grant-option-test"
	privilege := "USAGE"
	objectType := "DATABASE"
	objectName := "ANALYTICS_DB"
	toRole := "ANALYST_ROLE"

	var granted atomic.Bool

	grantMockSvc.SetObserve(func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
		if granted.Load() {
			return grantObservation(privilege, objectType, objectName, "ROLE", toRole, true), nil
		}

		return &snowflake.GrantObservation{Exists: false}, nil
	})

	grantMockSvc.SetGrant(func(_ context.Context, opts snowflake.CreateGrantOptions) error {
		assert.True(t, opts.WithGrantOption, "WithGrantOption should be true")
		granted.Store(true)

		return nil
	})

	grant := newTestGrant(grantK8s, privilege, objectType, objectName, toRole)
	grant.Spec.WithGrantOption = true
	require.NoError(t, k8sClient.Create(ctx, grant))

	key := types.NamespacedName{Name: grantK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "grant should become Ready")

	var result snowplanev1alpha1.GrantPrivilegesToAccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, result.Status.ShowOutput.GrantOption, "status should reflect GrantOption=true")

	// Cleanup.
	grantMockSvc.SetRevoke(func(_ context.Context, _ snowflake.RevokeGrantOptions) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestGrant_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	grantK8s := "grant-orphan-test"
	privilege := "USAGE"
	objectType := "WAREHOUSE"
	objectName := "COMPUTE_WH"
	toRole := "ETL_ROLE"

	var (
		granted atomic.Bool
		revoked atomic.Bool
	)

	grantMockSvc.SetObserve(func(_ context.Context, _ snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
		if granted.Load() {
			return grantObservation(privilege, objectType, objectName, "ROLE", toRole, false), nil
		}

		return &snowflake.GrantObservation{Exists: false}, nil
	})

	grantMockSvc.SetGrant(func(_ context.Context, _ snowflake.CreateGrantOptions) error {
		granted.Store(true)
		return nil
	})

	grantMockSvc.SetRevoke(func(_ context.Context, _ snowflake.RevokeGrantOptions) error {
		revoked.Store(true)
		return nil
	})

	grant := newTestGrant(grantK8s, privilege, objectType, objectName, toRole)
	grant.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, grant))

	key := types.NamespacedName{Name: grantK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.GrantPrivilegesToAccountRole
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.GrantPrivilegesToAccountRole
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, revoked.Load(), "Snowflake REVOKE should not be called with Orphan policy")
}
