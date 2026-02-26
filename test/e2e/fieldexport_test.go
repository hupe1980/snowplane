//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFieldExport_ExportsToConfigMap(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_FE")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "fieldexport test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	feName := dbName + "-fe"
	cmName := dbName + "-cm"
	fe := newFieldExportCR(feName, "Database", dbName, ".status.showOutput.name", "ConfigMap", cmName, "db-name")
	feCleanup := createCR(t, gvrFieldExport, fe)
	defer feCleanup()

	waitForReady(t, gvrFieldExport, feName)

	val := waitForConfigMapKey(t, testNamespace, cmName, "db-name")
	require.Equal(t, dbSFName, val, "ConfigMap should contain the Snowflake database name")

	hash := getStatusField(t, gvrFieldExport, feName, "lastExportedValueHash")
	require.NotEmpty(t, hash, "lastExportedValueHash should be set")

	deleteCR(t, gvrFieldExport, feName)
	waitForCRDeleted(t, gvrFieldExport, feName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}

func TestFieldExport_ExportsToSecret(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_FE_SEC")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "fieldexport secret test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	feName := dbName + "-fe-sec"
	secretName := dbName + "-secret"
	fe := newFieldExportCR(feName, "Database", dbName, ".status.showOutput.name", "Secret", secretName, "db-name")
	feCleanup := createCR(t, gvrFieldExport, fe)
	defer feCleanup()

	waitForReady(t, gvrFieldExport, feName)

	require.Eventually(t, func() bool {
		s, err := k8sClient.CoreV1().Secrets(testNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		_, ok := s.Data["db-name"]
		return ok
	}, defaultTimeout, defaultInterval, "Secret should have db-name key")

	s := getSecret(t, testNamespace, secretName)
	require.Equal(t, dbSFName, string(s.Data["db-name"]))

	deleteCR(t, gvrFieldExport, feName)
	deleteCR(t, gvrDatabase, dbName)
}

// TestFieldExport_FromWarehouse verifies FieldExport works with a Warehouse
// source, exporting the warehouse name into a ConfigMap.
func TestFieldExport_FromWarehouse(t *testing.T) {
	whSFName := uniqueName("WH_FOR_FE")
	whName := k8sName(whSFName)
	whCleanup := createCR(t, gvrWarehouse, newWarehouseCR(whName, whSFName, "fieldexport wh test"))
	defer whCleanup()
	waitForReady(t, gvrWarehouse, whName)

	feName := whName + "-fe"
	cmName := whName + "-cm"
	fe := newFieldExportCR(feName, "Warehouse", whName, ".status.showOutput.name", "ConfigMap", cmName, "wh-name")
	feCleanup := createCR(t, gvrFieldExport, fe)
	defer feCleanup()

	waitForReady(t, gvrFieldExport, feName)

	val := waitForConfigMapKey(t, testNamespace, cmName, "wh-name")
	require.Equal(t, whSFName, val, "ConfigMap should contain the Snowflake warehouse name")

	// Delete
	deleteCR(t, gvrFieldExport, feName)
	waitForCRDeleted(t, gvrFieldExport, feName)
	deleteCR(t, gvrWarehouse, whName)
	waitForCRDeleted(t, gvrWarehouse, whName)
}

// TestFieldExport_MultiResource verifies two FieldExports from different source
// kinds both write into a single ConfigMap with different keys.
func TestFieldExport_MultiResource(t *testing.T) {
	// Create database
	dbSFName := uniqueName("DB_FOR_MULTI_FE")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "multi-fe db"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	// Create warehouse
	whSFName := uniqueName("WH_FOR_MULTI_FE")
	whName := k8sName(whSFName)
	whCleanup := createCR(t, gvrWarehouse, newWarehouseCR(whName, whSFName, "multi-fe wh"))
	defer whCleanup()
	waitForReady(t, gvrWarehouse, whName)

	// Shared ConfigMap name
	cmName := dbName + "-shared-cm"

	// FieldExport 1: database name
	feName1 := dbName + "-fe1"
	fe1 := newFieldExportCR(feName1, "Database", dbName, ".status.showOutput.name", "ConfigMap", cmName, "db-name")
	fe1Cleanup := createCR(t, gvrFieldExport, fe1)
	defer fe1Cleanup()
	waitForReady(t, gvrFieldExport, feName1)

	// FieldExport 2: warehouse name
	feName2 := whName + "-fe2"
	fe2 := newFieldExportCR(feName2, "Warehouse", whName, ".status.showOutput.name", "ConfigMap", cmName, "wh-name")
	fe2Cleanup := createCR(t, gvrFieldExport, fe2)
	defer fe2Cleanup()
	waitForReady(t, gvrFieldExport, feName2)

	// Verify both keys exist in the same ConfigMap
	dbVal := waitForConfigMapKey(t, testNamespace, cmName, "db-name")
	require.Equal(t, dbSFName, dbVal)
	whVal := waitForConfigMapKey(t, testNamespace, cmName, "wh-name")
	require.Equal(t, whSFName, whVal)

	// Cleanup
	deleteCR(t, gvrFieldExport, feName1)
	deleteCR(t, gvrFieldExport, feName2)
	waitForCRDeleted(t, gvrFieldExport, feName1)
	waitForCRDeleted(t, gvrFieldExport, feName2)
	deleteCR(t, gvrWarehouse, whName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrWarehouse, whName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}
