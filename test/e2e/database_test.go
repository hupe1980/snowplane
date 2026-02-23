//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDatabase_FullLifecycle(t *testing.T) {
	sfName := uniqueName("DB")
	name := k8sName(sfName)
	comment := "e2e initial comment"

	cr := newDatabaseCR(name, sfName, comment)
	cleanup := createCR(t, gvrDatabase, cr)
	defer cleanup()

	waitForReady(t, gvrDatabase, name)

	require.True(t, sfExists(t, "DATABASES", sfName), "database should exist in Snowflake")

	fqn := getStatusField(t, gvrDatabase, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	showName := getStatusField(t, gvrDatabase, name, "showOutput", "name")
	assert.Equal(t, sfName, showName)

	// Update comment
	updatedComment := "e2e updated comment"
	updateCR(t, gvrDatabase, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		return sfGetComment(t, "DATABASES", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "database comment was not updated in Snowflake")

	// Drift detection
	sfExec(t, `ALTER DATABASE "`+sfName+`" SET COMMENT = 'external drift'`)

	require.Eventually(t, func() bool {
		return sfGetComment(t, "DATABASES", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "drift was not corrected")

	// Delete
	deleteCR(t, gvrDatabase, name)
	waitForCRDeleted(t, gvrDatabase, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "DATABASES", sfName)
	}, defaultTimeout, defaultInterval, "database should be dropped from Snowflake")
}

func TestDatabase_DeleteWithOrphanPolicy(t *testing.T) {
	sfName := uniqueName("DB_ORPHAN")
	name := k8sName(sfName)

	cr := withDeletionPolicy(newDatabaseCR(name, sfName, "orphan test"), "Orphan")
	cleanup := createCR(t, gvrDatabase, cr)
	defer cleanup()
	defer sfDrop(t, "DATABASE", sfName)

	waitForReady(t, gvrDatabase, name)
	require.True(t, sfExists(t, "DATABASES", sfName))

	deleteCR(t, gvrDatabase, name)
	waitForCRDeleted(t, gvrDatabase, name)

	require.True(t, sfExists(t, "DATABASES", sfName), "orphaned database should still exist in Snowflake")
	sfDrop(t, "DATABASE", sfName)
}
