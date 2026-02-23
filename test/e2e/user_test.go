//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestUser_FullLifecycle(t *testing.T) {
	sfName := uniqueName("USR")
	name := k8sName(sfName)
	comment := "e2e user comment"

	cr := newUserCR(name, sfName, comment)
	cleanup := createCR(t, gvrUser, cr)
	defer cleanup()

	waitForReady(t, gvrUser, name)

	require.True(t, sfExists(t, "USERS", sfName), "user should exist in Snowflake")

	fqn := getStatusField(t, gvrUser, name, "fullyQualifiedName")
	assert.Contains(t, fqn, sfName)

	// Update comment
	updatedComment := "e2e updated user comment"
	updateCR(t, gvrUser, name, func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedField(obj.Object, updatedComment, "spec", "comment")
	})

	require.Eventually(t, func() bool {
		return sfGetComment(t, "USERS", sfName) == updatedComment
	}, defaultTimeout, defaultInterval, "user comment was not updated")

	// Delete
	deleteCR(t, gvrUser, name)
	waitForCRDeleted(t, gvrUser, name)

	require.Eventually(t, func() bool {
		return !sfExists(t, "USERS", sfName)
	}, defaultTimeout, defaultInterval, "user should be dropped")
}
