//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAccountRole_FullLifecycle(t *testing.T) {
	sfName := uniqueName("AROLE")
	name := k8sName(sfName)
	comment := "e2e account role"

	cr := newAccountRoleCR(name, sfName, comment)
	cleanup := createCR(t, gvrAccountRole, cr)
	defer cleanup()

	waitForReady(t, gvrAccountRole, name)

	require.True(t, sfExists(t, "ROLES", sfName), "account role should exist in Snowflake")

	fqn := getStatusField(t, gvrAccountRole, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated role comment"
	updateCR(t, gvrAccountRole, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		return sfGetComment(t, "ROLES", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "account role comment was not updated")

	// Delete
	deleteCR(t, gvrAccountRole, name)
	waitForCRDeleted(t, gvrAccountRole, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "ROLES", sfName)
	}, defaultTimeout, defaultInterval, "account role should be dropped")
}

func TestDatabaseRole_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_DROLE")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "dbrole test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	sfName := uniqueName("DROLE")
	name := k8sName(sfName)
	comment := "e2e database role"

	cr := newDatabaseRoleCR(name, sfName, dbName, comment)
	cleanup := createCR(t, gvrDatabaseRole, cr)
	defer cleanup()

	waitForReady(t, gvrDatabaseRole, name)

	require.Eventually(t, func() bool {
		return sfExistsInDB(t, "DATABASE ROLES", dbSFName, sfName)
	}, defaultTimeout, defaultInterval, "database role should exist")

	fqn := getStatusField(t, gvrDatabaseRole, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated dbrole comment"
	updateCR(t, gvrDatabaseRole, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrDatabaseRole, name)
		c, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "comment")
		return c == updatedComment
	}, defaultTimeout, defaultInterval, "database role comment was not updated in status")

	// Delete
	deleteCR(t, gvrDatabaseRole, name)
	waitForCRDeleted(t, gvrDatabaseRole, name)
	deleteCR(t, gvrDatabase, dbName)
}
