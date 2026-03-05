//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAPIIntegration_FullLifecycle(t *testing.T) {
	sfName := uniqueName("APII")
	name := k8sName(sfName)
	comment := "e2e api integration comment"

	cr := newAPIIntegrationCR(name, sfName,
		[]interface{}{"https://example.com/api/"},
		comment,
	)
	cleanup := createCR(t, gvrAPIIntegration, cr)
	defer cleanup()

	waitForReady(t, gvrAPIIntegration, name)

	require.True(t, sfExists(t, "INTEGRATIONS", sfName),
		"API integration should exist in Snowflake")

	fqn := getStatusField(t, gvrAPIIntegration, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	showName := getStatusField(t, gvrAPIIntegration, name, "showOutput", "name")
	assert.Equal(t, sfName, showName)

	// Update comment.
	updatedComment := "e2e updated api integration comment"
	updateCR(t, gvrAPIIntegration, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		obj := getCR(t, gvrAPIIntegration, name)
		c, _, _ := unstructured.NestedString(obj.Object, "status", "showOutput", "comment")
		return c == updatedComment
	}, defaultTimeout, defaultInterval, "API integration comment should be updated")

	// Delete.
	deleteCR(t, gvrAPIIntegration, name)
	waitForCRDeleted(t, gvrAPIIntegration, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "INTEGRATIONS", sfName)
	}, defaultTimeout, defaultInterval, "API integration should be dropped from Snowflake")
}

func TestAPIIntegration_DeleteWithOrphanPolicy(t *testing.T) {
	sfName := uniqueName("APII_ORPHAN")
	name := k8sName(sfName)

	cr := withDeletionPolicy(newAPIIntegrationCR(name, sfName,
		[]interface{}{"https://example.com/api/"},
		"orphan test",
	), "Orphan")
	cleanup := createCR(t, gvrAPIIntegration, cr)
	defer cleanup()
	defer sfDrop(t, "INTEGRATION", sfName)

	waitForReady(t, gvrAPIIntegration, name)
	require.True(t, sfExists(t, "INTEGRATIONS", sfName))

	deleteCR(t, gvrAPIIntegration, name)
	waitForCRDeleted(t, gvrAPIIntegration, name)

	require.True(t, sfExists(t, "INTEGRATIONS", sfName),
		"orphaned API integration should still exist in Snowflake")
	sfDrop(t, "INTEGRATION", sfName)
}
