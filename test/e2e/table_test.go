//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTable_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_TABLE")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "table test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCHEMA_FOR_TABLE")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "table test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	sfName := uniqueName("TBL")
	name := k8sName(sfName)

	cr := newTableCR(name, sfName, dbName, schemaName)
	cleanup := createCR(t, gvrTable, cr)
	defer cleanup()

	waitForReady(t, gvrTable, name)

	require.True(t, sfExistsInSchema(t, "TABLES", dbSFName, schemaSFName, sfName),
		"table should exist in Snowflake")

	fqn := getStatusField(t, gvrTable, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated table comment"
	updateCR(t, gvrTable, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrTable, name)
		c, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "comment")
		return c == updatedComment
	}, defaultTimeout, defaultInterval, "table comment was not updated in status")

	// Delete
	deleteCR(t, gvrTable, name)
	waitForCRDeleted(t, gvrTable, name)

	require.Eventually(t, func() bool {
		return !sfExistsInSchema(t, "TABLES", dbSFName, schemaSFName, sfName)
	}, defaultTimeout, defaultInterval, "table should be dropped")

	deleteCR(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
}
