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

func TestAlert_CreateLifecycle(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"alert-db", "ALERT_DB", "alert-schema", "ALERT_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	alertMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
		if id.Name() == "MY_ALERT" && created.Load() {
			return alertObservation("MY_ALERT", "ALERT_DB", "ALERT_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.AlertObservation{Exists: false}, nil
	})

	alertMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAlertOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestAlert("test-alert", "MY_ALERT", "alert-db", "alert-schema")
	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-alert", Namespace: testNamespace}

	t.Cleanup(func() {
		alertMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error { return nil })

		var obj snowplanev1alpha1.Alert
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)

			require.Eventually(t, func() bool {
				return k8sClient.Get(ctx, key, &snowplanev1alpha1.Alert{}) != nil
			}, defaultTimeout, defaultInterval)
		}
	})

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Alert
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "alert should become Ready")

	var obj snowplanev1alpha1.Alert
	require.NoError(t, k8sClient.Get(ctx, key, &obj))
	require.NotNil(t, obj.Status.ShowOutput)
	require.Equal(t, "MY_ALERT", obj.Status.ShowOutput.Name)
}

func TestAlert_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	_, _, cleanupParents := setupReadyDatabaseAndSchema(t,
		"alert-orphan-db", "ALERT_ORPHAN_DB", "alert-orphan-schema", "ALERT_ORPHAN_SCHEMA")
	defer cleanupParents()

	var created atomic.Bool

	alertMockSvc.SetObserve(func(_ context.Context, id snowflake.SchemaObjectIdentifier) (*snowflake.AlertObservation, error) {
		if id.Name() == "MY_ALERT_ORPHAN" && created.Load() {
			return alertObservation("MY_ALERT_ORPHAN", "ALERT_ORPHAN_DB", "ALERT_ORPHAN_SCHEMA", "SYSADMIN"), nil
		}

		return &snowflake.AlertObservation{Exists: false}, nil
	})

	alertMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateAlertOptions) error {
		created.Store(true)

		return nil
	})

	cr := newTestAlert("test-alert-orphan", "MY_ALERT_ORPHAN", "alert-orphan-db", "alert-orphan-schema")
	cr.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan

	require.NoError(t, k8sClient.Create(ctx, cr))

	key := types.NamespacedName{Name: "test-alert-orphan", Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Alert
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var dropCalled atomic.Bool

	alertMockSvc.SetDrop(func(_ context.Context, _ snowflake.SchemaObjectIdentifier) error {
		dropCalled.Store(true)

		return nil
	})

	require.NoError(t, k8sClient.Delete(ctx, cr))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Alert{}) != nil
	}, defaultTimeout, defaultInterval)

	require.False(t, dropCalled.Load())
}
