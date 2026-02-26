//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCrossResource_DatabaseSchemaRole tests the dependency chain:
// Database -> Schema -> DatabaseRole
func TestCrossResource_DatabaseSchemaRole(t *testing.T) {
	dbSFName := uniqueName("XREF_DB")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "cross-resource test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)
	require.True(t, sfExists(t, "DATABASES", dbSFName))

	schemaSFName := uniqueName("XREF_SCHEMA")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "cross-ref schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)
	require.True(t, sfExistsInDB(t, "SCHEMAS", dbSFName, schemaSFName))

	schemaFQN := getStatusField(t, gvrSchema, schemaName, "fullyQualifiedName")
	require.Contains(t, schemaFQN, dbSFName)
	require.Contains(t, schemaFQN, schemaSFName)

	roleSFName := uniqueName("XREF_DROLE")
	roleName := k8sName(roleSFName)
	roleCleanup := createCR(t, gvrDatabaseRole, newDatabaseRoleCR(roleName, roleSFName, dbName, "cross-ref role"))
	defer roleCleanup()
	waitForReady(t, gvrDatabaseRole, roleName)
	require.True(t, sfExistsInDB(t, "DATABASE ROLES", dbSFName, roleSFName))

	// Delete in reverse order
	deleteCR(t, gvrDatabaseRole, roleName)
	waitForCRDeleted(t, gvrDatabaseRole, roleName)

	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)

	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)

	require.False(t, sfExists(t, "DATABASES", dbSFName), "database should be dropped")
}

// TestCrossResource_FullStack tests a deep dependency chain:
// Database -> Schema -> Table -> Stream + Task + Grant
func TestCrossResource_FullStack(t *testing.T) {
	// 1. Warehouse (needed for task)
	whSFName := uniqueName("XREF_FULL_WH")
	whName := k8sName(whSFName)
	whCleanup := createCR(t, gvrWarehouse, newWarehouseCR(whName, whSFName, "full stack wh"))
	defer whCleanup()
	waitForReady(t, gvrWarehouse, whName)

	// 2. Database
	dbSFName := uniqueName("XREF_FULL_DB")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "full stack test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	// 2. Schema
	schemaSFName := uniqueName("XREF_FULL_SCH")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "full stack schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	// 3. Table
	tableSFName := uniqueName("XREF_FULL_TBL")
	tableName := k8sName(tableSFName)
	tableCleanup := createCR(t, gvrTable, newTableCR(tableName, tableSFName, dbName, schemaName))
	defer tableCleanup()
	waitForReady(t, gvrTable, tableName)

	// 4. Stream on the table
	streamSFName := uniqueName("XREF_FULL_STR")
	streamName := k8sName(streamSFName)
	fqTable := dbSFName + "." + schemaSFName + "." + tableSFName
	streamCleanup := createCR(t, gvrStream, newStreamCR(streamName, streamSFName, dbName, schemaName, "TABLE", fqTable))
	defer streamCleanup()
	waitForReady(t, gvrStream, streamName)

	// 5. Task in the same schema (warehouse-backed)
	taskSFName := uniqueName("XREF_FULL_TSK")
	taskName := k8sName(taskSFName)
	taskCleanup := createCR(t, gvrTask, newTaskCR(taskName, taskSFName, dbName, schemaName, whSFName, "SELECT 1"))
	defer taskCleanup()
	waitForReady(t, gvrTask, taskName)

	// 6. Account role + grant on the database
	roleSFName := uniqueName("XREF_FULL_ROLE")
	roleName := k8sName(roleSFName)
	roleCleanup := createCR(t, gvrAccountRole, newAccountRoleCR(roleName, roleSFName, "full stack role"))
	defer roleCleanup()
	waitForReady(t, gvrAccountRole, roleName)

	grantName := roleName + "-usage"
	grantCR := newAccountRoleGrantCR(grantName, "USAGE", roleSFName, map[string]interface{}{
		"accountObject": map[string]interface{}{
			"objectType": "DATABASE",
			"objectName": dbSFName,
		},
	})
	grantCleanup := createCR(t, gvrAccountRoleGrant, grantCR)
	defer grantCleanup()
	waitForReady(t, gvrAccountRoleGrant, grantName)

	// Verify all exist
	require.True(t, sfExists(t, "DATABASES", dbSFName))
	require.True(t, sfExistsInDB(t, "SCHEMAS", dbSFName, schemaSFName))
	require.True(t, sfExistsInSchema(t, "TABLES", dbSFName, schemaSFName, tableSFName))
	require.True(t, sfExistsInSchema(t, "STREAMS", dbSFName, schemaSFName, streamSFName))
	require.True(t, sfExistsInSchema(t, "TASKS", dbSFName, schemaSFName, taskSFName))
	require.True(t, sfGrantExists(t, roleSFName, "USAGE", "DATABASE", dbSFName))

	// Delete in reverse dependency order
	deleteCR(t, gvrAccountRoleGrant, grantName)
	waitForCRDeleted(t, gvrAccountRoleGrant, grantName)
	deleteCR(t, gvrAccountRole, roleName)
	waitForCRDeleted(t, gvrAccountRole, roleName)
	deleteCR(t, gvrTask, taskName)
	waitForCRDeleted(t, gvrTask, taskName)
	deleteCR(t, gvrStream, streamName)
	waitForCRDeleted(t, gvrStream, streamName)
	deleteCR(t, gvrTable, tableName)
	waitForCRDeleted(t, gvrTable, tableName)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
	deleteCR(t, gvrWarehouse, whName)
	waitForCRDeleted(t, gvrWarehouse, whName)

	require.False(t, sfExists(t, "DATABASES", dbSFName), "database should be dropped after full chain delete")
}
