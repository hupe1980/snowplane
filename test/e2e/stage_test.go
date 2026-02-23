//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStage_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_STAGE")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "stage test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCHEMA_FOR_STAGE")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "stage test schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	sfName := uniqueName("STG")
	name := k8sName(sfName)
	comment := "e2e stage comment"

	cr := newStageCR(name, sfName, dbName, schemaName, comment)
	cleanup := createCR(t, gvrStage, cr)
	defer cleanup()

	waitForReady(t, gvrStage, name)

	require.True(t, sfExistsInSchema(t, "STAGES", dbSFName, schemaSFName, sfName),
		"stage should exist in Snowflake")

	fqn := getStatusField(t, gvrStage, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated stage comment"
	updateCR(t, gvrStage, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrStage, name)
		c, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "comment")
		return c == updatedComment
	}, defaultTimeout, defaultInterval, "stage comment was not updated in status")

	// Delete
	deleteCR(t, gvrStage, name)
	waitForCRDeleted(t, gvrStage, name)

	require.Eventually(t, func() bool {
		return !sfExistsInSchema(t, "STAGES", dbSFName, schemaSFName, sfName)
	}, defaultTimeout, defaultInterval, "stage should be dropped")

	deleteCR(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
}
