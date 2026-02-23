//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSchema_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_SCHEMA")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "schema test db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	sfName := uniqueName("SCHEMA")
	name := k8sName(sfName)
	comment := "e2e schema comment"

	cr := newSchemaCR(name, sfName, dbName, comment)
	cleanup := createCR(t, gvrSchema, cr)
	defer cleanup()

	waitForReady(t, gvrSchema, name)

	require.True(t, sfExistsInDB(t, "SCHEMAS", dbSFName, sfName), "schema should exist in Snowflake")

	fqn := getStatusField(t, gvrSchema, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated schema comment"
	updateCR(t, gvrSchema, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		return sfGetSchemaComment(t, dbSFName, sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "schema comment was not updated")

	// Delete
	deleteCR(t, gvrSchema, name)
	waitForCRDeleted(t, gvrSchema, name)

	require.Eventually(t, func() bool {
		return !sfExistsInDB(t, "SCHEMAS", dbSFName, sfName)
	}, defaultTimeout, defaultInterval, "schema should be dropped")

	deleteCR(t, gvrDatabase, dbName)
}
