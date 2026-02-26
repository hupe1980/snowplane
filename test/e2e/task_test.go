//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestTask_FullLifecycle creates a warehouse-backed suspended task, verifies it in
// Snowflake, updates the comment, and deletes it.
func TestTask_FullLifecycle(t *testing.T) {
	// Setup: warehouse + database + schema
	whSFName := uniqueName("WH_FOR_TASK")
	whName := k8sName(whSFName)
	whCleanup := createCR(t, gvrWarehouse, newWarehouseCR(whName, whSFName, "task test wh"))
	defer whCleanup()
	waitForReady(t, gvrWarehouse, whName)

	dbSFName := uniqueName("DB_FOR_TASK")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "task test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCH_FOR_TASK")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "task test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	// Create warehouse-backed suspended task
	sfName := uniqueName("TASK")
	name := k8sName(sfName)
	cr := newTaskCR(name, sfName, dbName, schemaName, whSFName, "SELECT 1")
	cleanup := createCR(t, gvrTask, cr)
	defer cleanup()

	waitForReady(t, gvrTask, name)
	require.True(t, sfExistsInSchema(t, "TASKS", dbSFName, schemaSFName, sfName), "task should exist in Snowflake")

	fqn := getStatusField(t, gvrTask, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	// Verify suspended state in status
	obj := getCR(t, gvrTask, name)
	state, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "state")
	require.Equal(t, "suspended", state, "task should be in suspended state")

	// Update comment
	updatedComment := "e2e updated task comment"
	updateCR(t, gvrTask, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrTask, name)
		c, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "comment")
		return c == updatedComment
	}, defaultTimeout, defaultInterval, "task comment should be updated")

	// Delete
	deleteCR(t, gvrTask, name)
	waitForCRDeleted(t, gvrTask, name)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
	deleteCR(t, gvrWarehouse, whName)
	waitForCRDeleted(t, gvrWarehouse, whName)
}
