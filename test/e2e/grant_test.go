//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAccountRoleGrant_FullLifecycle creates a database and account role, grants
// USAGE on the database to the role, verifies the grant exists in Snowflake,
// then deletes the grant and confirms it was revoked.
func TestAccountRoleGrant_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_ARGRANT")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "grant test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	roleSFName := uniqueName("ROLE_FOR_ARGRANT")
	roleName := k8sName(roleSFName)
	roleCleanup := createCR(t, gvrAccountRole, newAccountRoleCR(roleName, roleSFName, "grant test role"))
	defer roleCleanup()
	waitForReady(t, gvrAccountRole, roleName)

	grantName := roleName + "-usage-db"
	grantCR := newAccountRoleGrantCR(grantName, "USAGE", roleSFName, map[string]interface{}{
		"accountObject": map[string]interface{}{
			"objectType": "DATABASE",
			"objectName": dbSFName,
		},
	})
	grantCleanup := createCR(t, gvrAccountRoleGrant, grantCR)
	defer grantCleanup()
	waitForReady(t, gvrAccountRoleGrant, grantName)

	require.Eventually(t, func() bool {
		return sfGrantExists(t, roleSFName, "USAGE", "DATABASE", dbSFName)
	}, defaultTimeout, defaultInterval, "USAGE grant on DATABASE should exist")

	deleteCR(t, gvrAccountRoleGrant, grantName)
	waitForCRDeleted(t, gvrAccountRoleGrant, grantName)

	require.Eventually(t, func() bool {
		return !sfGrantExists(t, roleSFName, "USAGE", "DATABASE", dbSFName)
	}, defaultTimeout, defaultInterval, "USAGE grant should be revoked after CR deletion")

	deleteCR(t, gvrAccountRole, roleName)
	waitForCRDeleted(t, gvrAccountRole, roleName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}

// TestDatabaseRoleGrant_FullLifecycle creates a database, schema, and database role,
// grants USAGE on the schema to the database role, then cleans up.
func TestDatabaseRoleGrant_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_DRGRANT")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "dbrole grant test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCH_FOR_DRGRANT")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "dbrole grant test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	droleSFName := uniqueName("DROLE_FOR_GRANT")
	droleName := k8sName(droleSFName)
	droleCleanup := createCR(t, gvrDatabaseRole, newDatabaseRoleCR(droleName, droleSFName, dbName, "dbrole for grant"))
	defer droleCleanup()
	waitForReady(t, gvrDatabaseRole, droleName)

	grantName := droleName + "-usage-schema"
	fqDBRole := dbSFName + "." + droleSFName
	grantCR := newDatabaseRoleGrantCR(grantName, "USAGE", fqDBRole, map[string]interface{}{
		"schema": map[string]interface{}{
			"schemaName": dbSFName + "." + schemaSFName,
		},
	})
	grantCleanup := createCR(t, gvrDatabaseRoleGrant, grantCR)
	defer grantCleanup()
	waitForReady(t, gvrDatabaseRoleGrant, grantName)

	fqn := getStatusField(t, gvrDatabaseRoleGrant, grantName, "fullyQualifiedName")
	require.NotEmpty(t, fqn, "grant FQN should be set")

	deleteCR(t, gvrDatabaseRoleGrant, grantName)
	waitForCRDeleted(t, gvrDatabaseRoleGrant, grantName)
	deleteCR(t, gvrDatabaseRole, droleName)
	waitForCRDeleted(t, gvrDatabaseRole, droleName)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}

// TestAccountRoleGrant_FutureGrant grants SELECT on future tables in a
// database to an account role, verifying the future grant is recorded.
func TestAccountRoleGrant_FutureGrant(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_FUTGRANT")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "future grant test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	roleSFName := uniqueName("ROLE_FOR_FUT")
	roleName := k8sName(roleSFName)
	roleCleanup := createCR(t, gvrAccountRole, newAccountRoleCR(roleName, roleSFName, "future grant role"))
	defer roleCleanup()
	waitForReady(t, gvrAccountRole, roleName)

	grantName := roleName + "-future-tables"
	grantCR := newAccountRoleGrantCR(grantName, "SELECT", roleSFName, map[string]interface{}{
		"schemaObject": map[string]interface{}{
			"future": map[string]interface{}{
				"objectTypePlural": "TABLES",
				"inDatabase":       dbSFName,
			},
		},
	})
	grantCleanup := createCR(t, gvrAccountRoleGrant, grantCR)
	defer grantCleanup()
	waitForReady(t, gvrAccountRoleGrant, grantName)

	fqn := getStatusField(t, gvrAccountRoleGrant, grantName, "fullyQualifiedName")
	require.NotEmpty(t, fqn, "future grant FQN should be set")

	deleteCR(t, gvrAccountRoleGrant, grantName)
	waitForCRDeleted(t, gvrAccountRoleGrant, grantName)
	deleteCR(t, gvrAccountRole, roleName)
	waitForCRDeleted(t, gvrAccountRole, roleName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}
