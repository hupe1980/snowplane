//go:build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

// TestConcurrency_ParallelDatabaseCreation validates that the controller manager
// and rate limiter handle many resources being created simultaneously without
// deadlocking, losing updates, or tripping the circuit breaker.
func TestConcurrency_ParallelDatabaseCreation(t *testing.T) {
	resetMocks()

	const numResources = 20

	var createdDBs sync.Map

	dbMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.DatabaseObservation, error) {
		if _, ok := createdDBs.Load(id.Name()); ok {
			return databaseObservation(id.Name(), "", "SYSADMIN"), nil
		}

		return &snowflake.DatabaseObservation{Exists: false}, nil
	})

	dbMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateDatabaseOptions) error {
		createdDBs.Store(opts.Name.Name(), true)
		return nil
	})

	dbMockSvc.SetDrop(func(_ context.Context, id snowflake.AccountObjectIdentifier) error {
		createdDBs.Delete(id.Name())
		return nil
	})

	keys := make([]types.NamespacedName, numResources)

	var wg sync.WaitGroup

	for i := range numResources {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			k8sName := fmt.Sprintf("conc-db-%03d", idx)
			sfName := fmt.Sprintf("CONC_DB_%03d", idx)

			db := newTestDatabase(k8sName, sfName)
			if err := k8sClient.Create(ctx, db); err != nil {
				t.Errorf("failed to create database %d: %v", idx, err)
				return
			}

			keys[idx] = types.NamespacedName{Name: k8sName, Namespace: testNamespace}
		}(i)
	}

	wg.Wait()

	for i, key := range keys {
		if key.Name == "" {
			continue
		}

		require.Eventuallyf(t, func() bool {
			var obj snowplanev1alpha1.Database
			if err := k8sClient.Get(ctx, key, &obj); err != nil {
				return false
			}

			return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
		}, defaultTimeout, defaultInterval, "database %d (%s) should become Ready", i, key.Name)
	}

	var readyCount int

	for _, key := range keys {
		if key.Name == "" {
			continue
		}

		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err == nil && conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) {
			readyCount++
		}
	}

	assert.Equal(t, numResources, readyCount, "all %d databases should be Ready", numResources)

	for _, key := range keys {
		if key.Name == "" {
			continue
		}

		var obj snowplanev1alpha1.Database
		if err := k8sClient.Get(ctx, key, &obj); err == nil {
			_ = k8sClient.Delete(ctx, &obj)
		}
	}

	for _, key := range keys {
		if key.Name == "" {
			continue
		}

		require.Eventually(t, func() bool {
			var db snowplanev1alpha1.Database
			return k8sClient.Get(ctx, key, &db) != nil
		}, defaultTimeout, defaultInterval)
	}
}
