//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestView_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_VIEW")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "view test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCHEMA_FOR_VIEW")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "view test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	// Create a source table directly in Snowflake for the view to reference
	srcTable := uniqueName("SRC_TABLE")
	sfExec(t, fmt.Sprintf(`CREATE TABLE "%s"."%s"."%s" (ID NUMBER, NAME VARCHAR(100))`, dbSFName, schemaSFName, srcTable))
	defer sfDropInSchema(t, "TABLE", dbSFName, schemaSFName, srcTable)

	sfName := uniqueName("VW")
	name := k8sName(sfName)
	statement := fmt.Sprintf(`SELECT ID, NAME FROM "%s"."%s"."%s"`, dbSFName, schemaSFName, srcTable)

	cr := newViewCR(name, sfName, dbName, schemaName, statement)
	cleanup := createCR(t, gvrView, cr)
	defer cleanup()

	waitForReady(t, gvrView, name)

	require.True(t, sfExistsInSchema(t, "VIEWS", dbSFName, schemaSFName, sfName),
		"view should exist in Snowflake")

	fqn := getStatusField(t, gvrView, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	// Delete
	deleteCR(t, gvrView, name)
	waitForCRDeleted(t, gvrView, name)

	require.Eventually(t, func() bool {
		return !sfExistsInSchema(t, "VIEWS", dbSFName, schemaSFName, sfName)
	}, defaultTimeout, defaultInterval, "view should be dropped")

	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)

	deleteCR(t, gvrDatabase, dbName)
}
