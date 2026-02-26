//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNetworkPolicy_FullLifecycle(t *testing.T) {
	sfName := uniqueName("NETPOL")
	name := k8sName(sfName)

	cr := newNetworkPolicyCR(name, sfName, []interface{}{"192.168.1.0/24"}, "e2e network policy")
	cleanup := createCR(t, gvrNetworkPolicy, cr)
	defer cleanup()

	waitForReady(t, gvrNetworkPolicy, name)
	require.True(t, sfExists(t, "NETWORK POLICIES", sfName), "network policy should exist")

	fqn := getStatusField(t, gvrNetworkPolicy, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	updateCR(t, gvrNetworkPolicy, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedStringSlice(obj.Object, []string{"192.168.1.0/24", "10.0.0.0/8"}, "spec", "allowedIPList")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrNetworkPolicy, name)
		count, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "entriesInAllowedIPList")
		return count == "2"
	}, defaultTimeout, defaultInterval, "allowed IP list should have 2 entries")

	deleteCR(t, gvrNetworkPolicy, name)
	waitForCRDeleted(t, gvrNetworkPolicy, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "NETWORK POLICIES", sfName)
	}, defaultTimeout, defaultInterval, "network policy should be dropped")
}

func TestPasswordPolicy_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_PWPOL")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "pw policy test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCH_FOR_PWPOL")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "pw policy schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	sfName := uniqueName("PWPOL")
	name := k8sName(sfName)
	cr := newPasswordPolicyCR(name, sfName, dbName, schemaName, int64(10), "e2e password policy")
	cleanup := createCR(t, gvrPasswordPolicy, cr)
	defer cleanup()

	waitForReady(t, gvrPasswordPolicy, name)
	require.True(t, sfExistsInSchema(t, "PASSWORD POLICIES", dbSFName, schemaSFName, sfName))

	fqn := getStatusField(t, gvrPasswordPolicy, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	updateCR(t, gvrPasswordPolicy, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, int64(12), "spec", "passwordMinLength")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrPasswordPolicy, name)
		v, _, _ := unstructured.NestedString(obj.Object, "status", "describeOutput", "PASSWORD_MIN_LENGTH")
		return v == "12"
	}, defaultTimeout, defaultInterval, "password min length should be updated to 12")

	deleteCR(t, gvrPasswordPolicy, name)
	waitForCRDeleted(t, gvrPasswordPolicy, name)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}

func TestMaskingPolicy_FullLifecycle(t *testing.T) {
	dbSFName := uniqueName("DB_FOR_MASK")
	dbName := k8sName(dbSFName)
	dbCleanup := createCR(t, gvrDatabase, newDatabaseCR(dbName, dbSFName, "masking policy test"))
	defer dbCleanup()
	waitForReady(t, gvrDatabase, dbName)

	schemaSFName := uniqueName("SCH_FOR_MASK")
	schemaName := k8sName(schemaSFName)
	schemaCleanup := createCR(t, gvrSchema, newSchemaCR(schemaName, schemaSFName, dbName, "masking policy schema"))
	defer schemaCleanup()
	waitForReady(t, gvrSchema, schemaName)

	sfName := uniqueName("MASKPOL")
	name := k8sName(sfName)
	body := "CASE WHEN current_role() IN ('SYSADMIN') THEN val ELSE '***' END"
	cr := newMaskingPolicyCR(name, sfName, dbName, schemaName, body)
	cleanup := createCR(t, gvrMaskingPolicy, cr)
	defer cleanup()

	waitForReady(t, gvrMaskingPolicy, name)
	require.True(t, sfExistsInSchema(t, "MASKING POLICIES", dbSFName, schemaSFName, sfName))

	fqn := getStatusField(t, gvrMaskingPolicy, name, "fullyQualifiedName")
	require.Contains(t, fqn, sfName)

	updatedBody := "CASE WHEN current_role() IN ('SYSADMIN') THEN val ELSE '###' END"
	updateCR(t, gvrMaskingPolicy, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedBody, "spec", "body")
	})

	// Masking policy status does not include the body; verify the controller
	// processed the update by checking the observedGeneration catches up.
	require.Eventually(t, func() bool {
		obj := getCR(t, gvrMaskingPolicy, name)
		gen := obj.GetGeneration()
		obsGen, _, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration")
		return obsGen >= gen && isReady(gvrMaskingPolicy, name)
	}, defaultTimeout, defaultInterval, "masking policy body should be updated")

	deleteCR(t, gvrMaskingPolicy, name)
	waitForCRDeleted(t, gvrMaskingPolicy, name)
	deleteCR(t, gvrSchema, schemaName)
	waitForCRDeleted(t, gvrSchema, schemaName)
	deleteCR(t, gvrDatabase, dbName)
	waitForCRDeleted(t, gvrDatabase, dbName)
}
