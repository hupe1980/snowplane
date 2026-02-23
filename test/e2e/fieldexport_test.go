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
