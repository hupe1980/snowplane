//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStream_FullLifecycle creates a database, schema, and table, then creates
// a stream on the table, verifies it exists, and deletes everything.
func TestStream_FullLifecycle(t *testing.T) {
	// Setup: database + schema + table
	dbSFName := uniqueName("DB_FOR_STREAM")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "stream test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCH_FOR_STREAM")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "stream test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	tableSFName := uniqueName("TBL_FOR_STREAM")
	tableName := k8sName(tableSFName)
	tableCleanup := createCR(t, gvrTable, newTableCR(tableName, tableSFName, dbName, schemaName))
	defer tableCleanup()
	waitForReady(t, gvrTable, tableName)

	// Create stream on the table
	sfName := uniqueName("STREAM")
	name := k8sName(sfName)
	fqTableName := dbSFName + "." + schemaSFName + "." + tableSFName
	cr := newStreamCR(name, sfName, dbName, schemaName, "TABLE", fqTableName)
	cleanup := createCR(t, gvrStream, cr)
	defer cleanup()

	waitForReady(t, gvrStream, name)
	require.True(t, sfExistsInSchema(t, "STREAMS", dbSFName, schemaSFName, sfName), "stream should exist")

	fqn := getStatusField(t, gvrStream, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	// Delete in dependency order
	deleteCR(t, gvrStream, name)
	waitForCRDeleted(t, gvrStream, name)
	deleteCR(t, gvrTable, tableName)
	waitForCRDeleted(t, gvrTable, tableName)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}
