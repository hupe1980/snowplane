package snowplane_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// metaTypes lists internal k8s API machinery types that get auto-registered
// in every scheme but are not CRDs.
var metaTypes = map[string]bool{
	"CreateOptions": true, "UpdateOptions": true, "DeleteOptions": true,
	"GetOptions": true, "ListOptions": true, "PatchOptions": true,
	"WatchEvent": true,
}

// repoRoot returns the repository root by locating go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root (go.mod)")
		}

		dir = parent
	}
}

// registeredCRDPlurals returns the set of plural resource names for all
// CRD types registered with the snowplane v1alpha1 scheme.
func registeredCRDPlurals(t *testing.T) map[string]bool {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, snowplanev1alpha1.AddToScheme(scheme))

	gv := snowplanev1alpha1.GroupVersion
	knownTypes := scheme.KnownTypes(gv)

	plurals := make(map[string]bool)

	for gvk := range knownTypes {
		if strings.HasSuffix(gvk, "List") {
			continue
		}

		if metaTypes[gvk] {
			continue
		}

		lower := strings.ToLower(gvk)

		var plural string

		switch {
		case strings.HasSuffix(lower, "policy"):
			plural = strings.TrimSuffix(lower, "y") + "ies"
		case strings.HasSuffix(lower, "ss") || strings.HasSuffix(lower, "x") ||
			strings.HasSuffix(lower, "ch") || strings.HasSuffix(lower, "sh"):
			plural = lower + "es"
		case strings.HasSuffix(lower, "s"):
			plural = lower // already ends in s; kubebuilder treats as plural
		default:
			plural = lower + "s"
		}

		plurals[plural] = true
	}

	return plurals
}

// parseRBACResources extracts all resource names under the snowplane apiGroup
// from the RBAC template file. It tracks base, /status, and /finalizers entries.
func parseRBACResources(t *testing.T, rbacPath string) map[string]map[string]bool {
	t.Helper()

	data, err := os.ReadFile(rbacPath)
	require.NoError(t, err)

	resources := make(map[string]map[string]bool)

	lines := strings.Split(string(data), "\n")
	resourceLineRE := regexp.MustCompile(`^\s+-\s+(\S+)\s*$`)

	// State machine: look for snowplane apiGroup blocks only.
	inSnowplaneBlock := false
	inResources := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect apiGroup lines.
		if trimmed == "- snowplane.hupe1980.github.io" {
			inSnowplaneBlock = true
			inResources = false
			continue
		}

		// Non-snowplane apiGroup resets our context.
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, ".") && !strings.Contains(trimmed, "snowplane") {
			inSnowplaneBlock = false
			inResources = false
			continue
		}

		if !inSnowplaneBlock {
			continue
		}

		if trimmed == "resources:" {
			inResources = true
			continue
		}

		// "verbs:" ends the resource list.
		if trimmed == "verbs:" {
			inResources = false
			inSnowplaneBlock = false
			continue
		}

		if inResources {
			m := resourceLineRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			res := m[1]
			subResource := "base"

			parts := strings.SplitN(res, "/", 2)
			if len(parts) == 2 {
				res = parts[0]
				subResource = parts[1]
			}

			if resources[res] == nil {
				resources[res] = make(map[string]bool)
			}

			resources[res][subResource] = true
		}
	}

	return resources
}

// TestRBACCoversAllCRDs verifies that every CRD type registered in the
// snowplane v1alpha1 scheme has corresponding RBAC entries in the Helm chart.
// This prevents the common mistake of adding a new CRD without updating rbac.yaml.
func TestRBACCoversAllCRDs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	rbacPath := filepath.Join(root, "charts", "snowplane", "templates", "rbac.yaml")

	crdPlurals := registeredCRDPlurals(t)
	rbacResources := parseRBACResources(t, rbacPath)

	for plural := range crdPlurals {
		t.Run(plural, func(t *testing.T) {
			t.Parallel()

			rbac, ok := rbacResources[plural]
			if !ok {
				assert.Fail(t, "CRD resource missing from RBAC",
					"CRD %q is registered in the scheme but has no entry in rbac.yaml."+
						" Add it to all ClusterRole resource lists (resources, /status, /finalizers).",
					plural)

				return
			}

			assert.True(t, rbac["base"],
				"CRD %q is missing from the main resources list in rbac.yaml", plural)
			assert.True(t, rbac["status"],
				"CRD %q/status is missing from rbac.yaml", plural)
			assert.True(t, rbac["finalizers"],
				"CRD %q/finalizers is missing from rbac.yaml", plural)
		})
	}
}

// TestRBACNoOrphanedEntries verifies that every snowplane resource in the RBAC
// template is registered in the scheme. Catches stale entries after CRD removal.
func TestRBACNoOrphanedEntries(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	rbacPath := filepath.Join(root, "charts", "snowplane", "templates", "rbac.yaml")

	crdPlurals := registeredCRDPlurals(t)
	rbacResources := parseRBACResources(t, rbacPath)

	for res := range rbacResources {
		if !crdPlurals[res] {
			assert.Fail(t, "Orphaned RBAC entry",
				"Resource %q is in rbac.yaml under the snowplane apiGroup but not registered in the scheme. "+
					"Remove it if the CRD no longer exists.", res)
		}
	}
}

// TestRBACConsistentAcrossRoles verifies that the viewer and editor roles
// contain the same set of snowplane CRD resources as the main operator ClusterRole.
func TestRBACConsistentAcrossRoles(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	rbacPath := filepath.Join(root, "charts", "snowplane", "templates", "rbac.yaml")

	data, err := os.ReadFile(rbacPath)
	require.NoError(t, err)

	// Split YAML documents on "---".
	content := string(data)
	docs := strings.Split(content, "---")

	type roleResources struct {
		name      string
		resources map[string]bool
	}

	var roles []roleResources

	resourceLineRE := regexp.MustCompile(`^\s+-\s+(\S+)\s*$`)

	for _, doc := range docs {
		if !strings.Contains(doc, "kind: ClusterRole") {
			continue
		}

		var name string

		switch {
		case strings.Contains(doc, "-viewer"):
			name = "viewer"
		case strings.Contains(doc, "-editor"):
			name = "editor"
		default:
			name = "operator"
		}

		// Only collect resources under the snowplane apiGroup.
		resources := make(map[string]bool)
		inSnowplane := false
		inResources := false

		for _, line := range strings.Split(doc, "\n") {
			trimmed := strings.TrimSpace(line)

			if trimmed == "- snowplane.hupe1980.github.io" {
				inSnowplane = true
				inResources = false
				continue
			}

			if !inSnowplane {
				continue
			}

			if trimmed == "resources:" {
				inResources = true
				continue
			}

			if trimmed == "verbs:" {
				inResources = false
				inSnowplane = false
				continue
			}

			if inResources {
				m := resourceLineRE.FindStringSubmatch(line)
				if m == nil {
					continue
				}

				res := m[1]
				if !strings.Contains(res, "/") {
					resources[res] = true
				}
			}
		}

		if len(resources) > 0 {
			roles = append(roles, roleResources{name: name, resources: resources})
		}
	}

	require.GreaterOrEqual(t, len(roles), 3,
		"Expected at least 3 ClusterRoles (operator, viewer, editor)")

	var operatorRes map[string]bool

	for _, r := range roles {
		if r.name == "operator" {
			operatorRes = r.resources
			break
		}
	}

	require.NotNil(t, operatorRes, "Operator ClusterRole not found")

	for _, r := range roles {
		if r.name == "operator" {
			continue
		}

		for res := range operatorRes {
			assert.True(t, r.resources[res],
				"Resource %q is in the operator ClusterRole but missing from the %s ClusterRole",
				res, r.name)
		}

		for res := range r.resources {
			assert.True(t, operatorRes[res],
				"Resource %q is in the %s ClusterRole but missing from the operator ClusterRole",
				res, r.name)
		}
	}
}

// --------------------------------------------------------------------------
// Kustomize RBAC sync tests (H-1)
// --------------------------------------------------------------------------

// TestKustomizeRBACCoversAllCRDs verifies that the kustomize role.yaml contains
// every CRD type registered in the scheme. This prevents the common mistake of
// adding a new CRD and only updating the Helm chart RBAC, leaving kustomize stale.
func TestKustomizeRBACCoversAllCRDs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")

	crdPlurals := registeredCRDPlurals(t)
	rbacResources := parseRBACResources(t, rolePath)

	for plural := range crdPlurals {
		t.Run(plural, func(t *testing.T) {
			t.Parallel()

			rbac, ok := rbacResources[plural]
			if !ok {
				assert.Fail(t, "CRD resource missing from kustomize RBAC",
					"CRD %q is registered in the scheme but has no entry in config/rbac/role.yaml."+
						" Add it to all three sections (resources, /status, /finalizers).",
					plural)
				return
			}
			assert.True(t, rbac["base"],
				"CRD %q is missing from the main resources list in config/rbac/role.yaml", plural)
			assert.True(t, rbac["status"],
				"CRD %q/status is missing from config/rbac/role.yaml", plural)
			assert.True(t, rbac["finalizers"],
				"CRD %q/finalizers is missing from config/rbac/role.yaml", plural)
		})
	}
}

// TestKustomizeEditorViewerCoverAllCRDs verifies that the kustomize
// role_editor.yaml and role_viewer.yaml contain every CRD type registered
// in the scheme.
func TestKustomizeEditorViewerCoverAllCRDs(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	crdPlurals := registeredCRDPlurals(t)

	files := []struct {
		name string
		path string
	}{
		{"editor", filepath.Join(root, "config", "rbac", "role_editor.yaml")},
		{"viewer", filepath.Join(root, "config", "rbac", "role_viewer.yaml")},
	}

	for _, f := range files {
		f := f
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			rbacResources := parseRBACResources(t, f.path)
			for plural := range crdPlurals {
				rbac, ok := rbacResources[plural]
				if !ok {
					assert.Fail(t, "CRD resource missing from kustomize "+f.name+" role",
						"CRD %q is registered in the scheme but has no entry in config/rbac/%s."+
							" Add it to resources and /status sections.",
						plural, filepath.Base(f.path))
					continue
				}
				assert.True(t, rbac["base"],
					"CRD %q is missing from the main resources list in %s", plural, f.name)
				assert.True(t, rbac["status"],
					"CRD %q/status is missing from %s", plural, f.name)
			}
		})
	}
}

// TestKustomizeRBACMatchesHelmRBAC verifies that the kustomize and Helm RBAC
// files contain the same set of snowplane resources. This catches drift between
// the two deployment paths.
func TestKustomizeRBACMatchesHelmRBAC(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	helmPath := filepath.Join(root, "charts", "snowplane", "templates", "rbac.yaml")
	kustomizePath := filepath.Join(root, "config", "rbac", "role.yaml")

	helmResources := parseRBACResources(t, helmPath)
	kustomizeResources := parseRBACResources(t, kustomizePath)

	// Every Helm resource must be in kustomize
	for res := range helmResources {
		assert.Contains(t, kustomizeResources, res,
			"Resource %q is in Helm rbac.yaml but missing from kustomize role.yaml", res)
	}

	// Every kustomize resource must be in Helm
	for res := range kustomizeResources {
		assert.Contains(t, helmResources, res,
			"Resource %q is in kustomize role.yaml but missing from Helm rbac.yaml", res)
	}
}
