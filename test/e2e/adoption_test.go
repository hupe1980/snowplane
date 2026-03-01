//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdoption_Database(t *testing.T) {
	sfName := uniqueName("DB_ADOPT")
	name := k8sName(sfName)

	sfExec(t, `CREATE DATABASE "`+sfName+`" COMMENT = 'pre-existing'`)
	defer sfDrop(t, "DATABASE", sfName)

	require.True(t, sfExists(t, "DATABASES", sfName), "pre-created database should exist")

	cr := newDatabaseCR(name, sfName, "adopted")
	// Set adoption policy via spec field.
	spec := cr.Object["spec"].(map[string]interface{})
	spec["managementPolicies"] = map[string]interface{}{
		"adoptionPolicy": "adopt",
	}
	cleanup := createCR(t, gvrDatabase, cr)
	defer cleanup()

	waitForReady(t, gvrDatabase, name)

	obj := getCR(t, gvrDatabase, name)
	annotations := obj.GetAnnotations()
	require.Equal(t, "true", annotations["internal.snowplane.hupe1980.github.io/late-initialized"],
		"late-initialized annotation should be set after adoption")

	deleteCR(t, gvrDatabase, name)
	waitForCRDeleted(t, gvrDatabase, name)
}

func TestAdoption_FailIfExists(t *testing.T) {
	sfName := uniqueName("DB_FAIL_ADOPT")
	name := k8sName(sfName)

	sfExec(t, `CREATE DATABASE "`+sfName+`" COMMENT = 'blocking'`)
	defer sfDrop(t, "DATABASE", sfName)

	cr := newDatabaseCR(name, sfName, "should fail")
	cleanup := createCR(t, gvrDatabase, cr)
	defer cleanup()

	require.Eventually(t, func() bool {
		reason := getConditionReason(t, gvrDatabase, name, "Ready")
		return reason == "ResourceAlreadyExists" || reason == "TerminalError"
	}, defaultTimeout, defaultInterval, "should get a terminal error for pre-existing resource")

	deleteCR(t, gvrDatabase, name)
}
