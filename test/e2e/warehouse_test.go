//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWarehouse_FullLifecycle(t *testing.T) {
	sfName := uniqueName("WH")
	name := k8sName(sfName)
	comment := "e2e warehouse comment"

	cr := newWarehouseCR(name, sfName, comment)
	cleanup := createCR(t, gvrWarehouse, cr)
	defer cleanup()

	waitForReady(t, gvrWarehouse, name)

	require.True(t, sfExists(t, "WAREHOUSES", sfName), "warehouse should exist in Snowflake")

	fqn := getStatusField(t, gvrWarehouse, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated warehouse comment"
	updateCR(t, gvrWarehouse, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		return sfGetComment(t, "WAREHOUSES", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "warehouse comment was not updated")

	// Drift detection
	sfExec(t, `ALTER WAREHOUSE "`+sfName+`" SET COMMENT = 'external wh drift'`)

	require.Eventually(t, func() bool {
		return sfGetComment(t, "WAREHOUSES", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "warehouse drift was not corrected")

	// Delete
	deleteCR(t, gvrWarehouse, name)
	waitForCRDeleted(t, gvrWarehouse, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "WAREHOUSES", sfName)
	}, defaultTimeout, defaultInterval, "warehouse should be dropped")
}

func TestWarehouse_DeleteWithOrphanPolicy(t *testing.T) {
	sfName := uniqueName("WH_ORPHAN")
	name := k8sName(sfName)

	cr := withDeletionPolicy(newWarehouseCR(name, sfName, "orphan wh"), "Orphan")
	cleanup := createCR(t, gvrWarehouse, cr)
	defer cleanup()
	defer sfDrop(t, "WAREHOUSE", sfName)

	waitForReady(t, gvrWarehouse, name)
	require.True(t, sfExists(t, "WAREHOUSES", sfName))

	deleteCR(t, gvrWarehouse, name)
	waitForCRDeleted(t, gvrWarehouse, name)

	require.True(t, sfExists(t, "WAREHOUSES", sfName), "orphaned warehouse should still exist")
	sfDrop(t, "WAREHOUSE", sfName)
}
