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
