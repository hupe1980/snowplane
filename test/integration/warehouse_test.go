//go:build integration

package integration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/utils/conditions"
)

func TestWarehouse_CreateLifecycle(t *testing.T) {
	resetMocks()

	whK8s := "wh-create-test"
	sfWH := "WH_CREATE_TEST"

	var created atomic.Bool

	warehouseMockSvc.SetObserve(func(_ context.Context, id snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		if created.Load() {
			return warehouseObservation(sfWH, "", "SYSADMIN"), nil
		}

		return &snowflake.WarehouseObservation{Exists: false}, nil
	})

	warehouseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
		assert.Equal(t, sfWH, opts.Name.Name())
		created.Store(true)

		return nil
	})

	wh := newTestWarehouse(whK8s, sfWH)
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced)
	}, defaultTimeout, defaultInterval, "warehouse should become Ready")

	var result snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	assert.True(t, created.Load(), "Snowflake CREATE should have been called")
	assert.Equal(t, sfWH, result.Status.ShowOutput.Name)
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.Equal(t, snowplanev1alpha1.WarehouseState("STARTED"), result.Status.State)
	assert.NotEmpty(t, result.Status.FullyQualifiedName)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.Contains(t, result.Finalizers, "snowplane.hupe1980.github.io/warehouse")

	// Cleanup.
	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval, "warehouse should be cleaned up")
}

func TestWarehouse_UpdateTriggersAlter(t *testing.T) {
	resetMocks()

	whK8s := "wh-alter-test"
	sfWH := "WH_ALTER_TEST"

	var (
		created    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		if created.Load() {
			obs := warehouseObservation(sfWH, curComment.Load().(string), "SYSADMIN")
			return obs, nil
		}

		return &snowflake.WarehouseObservation{Exists: false}, nil
	})

	// Warehouse supports CREATE OR ALTER, so updates go through the Create
	// mock (CREATE OR ALTER) rather than the Alter mock.
	warehouseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	wh := newTestWarehouse(whK8s, sfWH)
	initComment := "initial warehouse comment"
	wh.Spec.Comment = &initComment
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval, "warehouse should become Ready initially")

	// Update the comment.
	var current snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &current))

	newComment := "updated warehouse comment"
	current.Spec.Comment = &newComment
	require.NoError(t, k8sClient.Update(ctx, &current))

	// CREATE OR ALTER is the default for Warehouse, so the Create mock
	// handles the update. Verify the status reflects the new comment.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Comment == "updated warehouse comment"
	}, defaultTimeout, defaultInterval, "status should reflect updated comment")

	// Cleanup.
	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

func TestWarehouse_DeleteWithOrphanPolicy(t *testing.T) {
	resetMocks()

	whK8s := "wh-orphan-test"
	sfWH := "WH_ORPHAN_TEST"

	var (
		created atomic.Bool
		dropped atomic.Bool
	)

	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		if created.Load() {
			return warehouseObservation(sfWH, "", "SYSADMIN"), nil
		}

		return &snowflake.WarehouseObservation{Exists: false}, nil
	})

	warehouseMockSvc.SetCreate(func(_ context.Context, _ snowflake.CreateWarehouseOptions) error {
		created.Store(true)
		return nil
	})

	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error {
		dropped.Store(true)
		return nil
	})

	wh := newTestWarehouse(whK8s, sfWH)
	wh.Spec.DeletionPolicy = snowplanev1alpha1.DeletionPolicyOrphan
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady)
	}, defaultTimeout, defaultInterval)

	var current snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)

	assert.False(t, dropped.Load(), "Snowflake DROP should not be called with Orphan policy")
}

func TestWarehouse_DriftDetection(t *testing.T) {
	resetMocks()

	whK8s := "wh-drift-test"
	sfWH := "WH_DRIFT_TEST"

	var (
		created    atomic.Bool
		curComment atomic.Value
	)

	curComment.Store("")

	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		if created.Load() {
			return warehouseObservation(sfWH, curComment.Load().(string), "SYSADMIN"), nil
		}

		return &snowflake.WarehouseObservation{Exists: false}, nil
	})

	// Warehouse supports CREATE OR ALTER, so drift correction goes through
	// the Create mock rather than the Alter mock.
	warehouseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
		created.Store(true)

		if opts.Comment != nil {
			curComment.Store(*opts.Comment)
		}

		return nil
	})

	myComment := "drift warehouse comment"
	wh := newTestWarehouse(whK8s, sfWH)
	wh.Spec.Comment = &myComment
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) && obj.Status.LastAppliedSpecHash != ""
	}, defaultTimeout, defaultInterval)

	// Simulate external drift.
	curComment.Store("externally changed")

	// Warehouse supports CREATE OR ALTER, so drift correction goes through
	// the Create mock. Verify the comment is restored to the desired value.
	require.Eventually(t, func() bool {
		return curComment.Load().(string) == "drift warehouse comment"
	}, defaultTimeout, defaultInterval, "drift should be detected and corrected")

	// Cleanup.
	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}

// ---------------------------------------------------------------------------
// Warehouse Adoption Tests
// ---------------------------------------------------------------------------

func TestWarehouse_Adoption_FailIfExists(t *testing.T) {
	resetMocks()

	whK8s := "wh-adopt-fail"
	sfWH := "WH_ADOPT_FAIL"

	// Observe always returns Exists: true — pre-existing Snowflake warehouse.
	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		return warehouseObservation(sfWH, "", "SYSADMIN"), nil
	})

	// No adoption annotation → should become Terminal.
	wh := newTestWarehouse(whK8s, sfWH)
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTerminal(&obj)
	}, defaultTimeout, defaultInterval, "should become Terminal when warehouse exists without adoption annotation")

	var result snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	tc := conditions.Get(&result, snowplanev1alpha1.TypeReady)
	require.NotNil(t, tc)
	assert.Equal(t, metav1.ConditionFalse, tc.Status)
	assert.Equal(t, snowplanev1alpha1.ReasonResourceExists, tc.Reason)
	assert.Contains(t, tc.Message, "already exists")
	assert.False(t, conditions.IsTrue(&result, snowplanev1alpha1.TypeReady))

	// Cleanup: remove finalizer and delete.
	var cleanup snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &cleanup))

	cleanup.Finalizers = nil
	require.NoError(t, k8sClient.Update(ctx, &cleanup))
	require.NoError(t, k8sClient.Delete(ctx, &cleanup))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Warehouse{}) != nil
	}, defaultTimeout, defaultInterval)
}

func TestWarehouse_Adoption_AdoptSuccess(t *testing.T) {
	resetMocks()

	whK8s := "wh-adopt-success"
	sfWH := "WH_ADOPT_SUCCESS"

	// Observe always returns Exists: true — pre-existing Snowflake warehouse.
	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		return warehouseObservation(sfWH, "pre-existing-wh", "SYSADMIN"), nil
	})

	// With adoption annotation → should adopt and become Ready.
	wh := newTestWarehouse(whK8s, sfWH)
	wh.Spec.ManagementPolicies.AdoptionPolicy = snowplanev1alpha1.AdoptionPolicyTypeAdopt
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) &&
			conditions.IsTrue(&obj, snowplanev1alpha1.TypeSynced) &&
			obj.Annotations[snowplanev1alpha1.AnnotationLateInitialized] == "true"
	}, defaultTimeout, defaultInterval, "adopted warehouse should become Ready with LateInitialized")

	var result snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &result))

	// Verify adoption results.
	assert.Equal(t, sfWH, result.Status.ShowOutput.Name, "ShowOutput should be populated from existing resource")
	assert.Equal(t, "SYSADMIN", result.Status.ShowOutput.Owner)
	assert.Equal(t, "pre-existing-wh", result.Status.ShowOutput.Comment)
	assert.NotEmpty(t, result.Status.LastAppliedSpecHash)
	assert.Equal(t, result.Generation, result.Status.ObservedGeneration)
	assert.False(t, conditions.IsTerminal(&result))

	// Verify LateInitialized annotation.
	assert.Equal(t, "true", result.Annotations[snowplanev1alpha1.AnnotationLateInitialized])

	// Cleanup.
	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	require.NoError(t, k8sClient.Delete(ctx, &result))

	require.Eventually(t, func() bool {
		return k8sClient.Get(ctx, key, &snowplanev1alpha1.Warehouse{}) != nil
	}, defaultTimeout, defaultInterval)
}

// ---------------------------------------------------------------------------
// Warehouse Structural Drift Detection Test
// ---------------------------------------------------------------------------
// This test verifies that drift detection works for structural fields
// (warehouse size), not just simple string fields like comments.

func TestWarehouse_StructuralDriftDetection(t *testing.T) {
	resetMocks()

	whK8s := "wh-struct-drift-test"
	sfWH := "WH_STRUCT_DRIFT_TEST"

	var (
		created atomic.Bool
		curSize atomic.Value
	)

	desiredSize := snowplanev1alpha1.WarehouseSizeSmall
	curSize.Store("SMALL")

	warehouseMockSvc.SetObserve(func(_ context.Context, _ snowflake.AccountObjectIdentifier) (*snowflake.WarehouseObservation, error) {
		if created.Load() {
			return &snowflake.WarehouseObservation{
				Exists: true,
				ShowOutput: &snowplanev1alpha1.WarehouseShowOutput{
					CreatedOn:       "2024-01-01",
					Name:            sfWH,
					State:           "STARTED",
					Type:            "STANDARD",
					Size:            curSize.Load().(string),
					Comment:         "",
					Owner:           "SYSADMIN",
					AutoSuspend:     600,
					AutoResume:      true,
					MinClusterCount: 1,
					MaxClusterCount: 1,
					ScalingPolicy:   "STANDARD",
				},
				Parameters: &snowflake.WarehouseParameters{
					MaxConcurrencyLevel:             ptr(int32(8)),
					StatementQueuedTimeoutInSeconds: ptr(int32(0)),
					StatementTimeoutInSeconds:       ptr(int32(172800)),
					EnableQueryAcceleration:         ptr(false),
					QueryAccelerationMaxScaleFactor: ptr(int32(8)),
				},
			}, nil
		}

		return &snowflake.WarehouseObservation{Exists: false}, nil
	})

	// Warehouse uses CREATE OR ALTER, so drift correction goes through Create.
	warehouseMockSvc.SetCreate(func(_ context.Context, opts snowflake.CreateWarehouseOptions) error {
		created.Store(true)

		if opts.WarehouseSize != nil {
			curSize.Store(*opts.WarehouseSize)
		}

		return nil
	})

	wh := newTestWarehouse(whK8s, sfWH)
	wh.Spec.WarehouseSize = &desiredSize
	require.NoError(t, k8sClient.Create(ctx, wh))

	key := types.NamespacedName{Name: whK8s, Namespace: testNamespace}

	// Wait for initial Ready.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return conditions.IsTrue(&obj, snowplanev1alpha1.TypeReady) && obj.Status.LastAppliedSpecHash != ""
	}, defaultTimeout, defaultInterval)

	// Verify initial size.
	assert.Equal(t, "SMALL", curSize.Load().(string))

	// Simulate external structural drift: warehouse size changed in Snowflake.
	curSize.Store("XLARGE")

	// Wait for reconciler to detect drift and correct it via CREATE OR ALTER.
	require.Eventually(t, func() bool {
		return curSize.Load().(string) == "SMALL"
	}, defaultTimeout, defaultInterval, "structural drift on warehouse size should be detected and corrected")

	// Verify the K8s status object also reflects the corrected size.
	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		if err := k8sClient.Get(ctx, key, &obj); err != nil {
			return false
		}

		return obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Size == "SMALL"
	}, defaultTimeout, defaultInterval, "status.showOutput.size should reflect corrected value")

	// Cleanup.
	warehouseMockSvc.SetDrop(func(_ context.Context, _ snowflake.AccountObjectIdentifier) error { return nil })

	var current snowplanev1alpha1.Warehouse
	require.NoError(t, k8sClient.Get(ctx, key, &current))
	require.NoError(t, k8sClient.Delete(ctx, &current))

	require.Eventually(t, func() bool {
		var obj snowplanev1alpha1.Warehouse
		return k8sClient.Get(ctx, key, &obj) != nil
	}, defaultTimeout, defaultInterval)
}
