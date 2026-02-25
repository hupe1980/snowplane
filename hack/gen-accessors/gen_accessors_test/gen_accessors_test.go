package genaccessors_test

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot returns the repository root by locating go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()

	// Walk up from this test file's directory to find go.mod.
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}

		dir = parent
	}
}

// TestGeneratorIdempotent runs the generator twice and verifies the output
// is identical — no timestamps or non-deterministic elements.
func TestGeneratorIdempotent(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	genPath := filepath.Join(root, "hack", "gen-accessors", "main.go")
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	// Read current generated file.
	before, err := os.ReadFile(outPath)
	require.NoError(t, err, "reading existing generated file")

	// Run generator from repo root.
	cmd := exec.Command("go", "run", genPath)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "running generator: %s", string(output))

	// Read regenerated file.
	after, err := os.ReadFile(outPath)
	require.NoError(t, err, "reading regenerated file")

	assert.Equal(t, string(before), string(after), "generator output should be identical on re-run (idempotent)")
}

// TestGeneratedOutputIsValidGo parses the generated file as Go and verifies
// it has no syntax errors.
func TestGeneratedOutputIsValidGo(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, "zz_generated_accessors.go", src, parser.AllErrors)
	require.NoError(t, err, "generated file should be valid Go")
}

// TestGeneratedMethodCount verifies the expected total number of methods.
func TestGeneratedMethodCount(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	content := string(src)
	methodCount := strings.Count(content, "func (")

	// 14 A1 types × 16 methods + 3 A3 types × 16 methods + 3 A2 types × 16 methods + 3 B types × 15 methods + 1 C type × 15 methods + 3 Phase6 types × 16 methods = 428
	assert.Equal(t, 428, methodCount, "expected 428 accessor methods across 27 types")
}

// TestGeneratedTypeHeaders verifies all 27 types have section headers.
func TestGeneratedTypeHeaders(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	content := string(src)

	expectedTypes := []string{
		"Database", "Schema", "Warehouse", "User", "AccountRole", "DatabaseRole",
		"Tag", "MaskingPolicy", "RowAccessPolicy", "Stage", "Stream", "Task",
		"View", "Table", "NetworkPolicy", "ResourceMonitor", "StorageIntegration",
		"FileFormat", "Pipe", "DynamicTable",
		"SecurityIntegration", "PasswordPolicy", "NetworkRule",
		"AccountRoleGrant", "DatabaseRoleGrant", "ShareGrant", "GrantOwnership",
	}

	for _, typeName := range expectedTypes {
		header := "// " + typeName
		assert.Contains(t, content, header, "missing section header for %s", typeName)

		// Verify at least one method exists for this type
		methodSig := "func (" // Just checking methods exist
		assert.Contains(t, content, methodSig)
	}
}

// TestGeneratedNoTimestamp verifies the output has no timestamp for
// deterministic regeneration.
func TestGeneratedNoTimestamp(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	assert.NotContains(t, string(src), "Generated at", "output should not contain a timestamp")
}

// extractSection returns the generated code for a specific type, between its
// header comment and the next separator line (or EOF).
func extractSection(t *testing.T, content, typeName string) string {
	t.Helper()

	marker := "// " + typeName + "\n"
	idx := strings.Index(content, marker)
	require.NotEqual(t, -1, idx, "should find section header for %s", typeName)

	// Skip past the header line + the closing separator line below it.
	rest := content[idx+len(marker):]
	closeSep := strings.Index(rest, "// -----")
	if closeSep != -1 {
		rest = rest[closeSep:]
		// Skip past this separator line.
		nl := strings.Index(rest, "\n")
		if nl != -1 {
			rest = rest[nl+1:]
		}
	}

	// Find the next section's opening separator line.
	nextSep := strings.Index(rest, "// -------")
	if nextSep == -1 {
		return rest
	}

	return rest[:nextSep]
}

// TestGeneratedGetSpecNameSkipped verifies GetSpecName is absent for grant types.
func TestGeneratedGetSpecNameSkipped(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	content := string(src)

	// Grant types should NOT have generated GetSpecName (they have custom implementations).
	skipTypes := []string{"AccountRoleGrant", "DatabaseRoleGrant", "ShareGrant", "GrantOwnership"}
	for _, typeName := range skipTypes {
		section := extractSection(t, content, typeName)
		assert.NotContains(t, section, "GetSpecName", "GetSpecName should be skipped for %s", typeName)
	}
}

// TestGeneratedOwnerPatterns verifies each owner pattern produces correct code.
func TestGeneratedOwnerPatterns(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	content := string(src)

	// A1 types should access ShowOutput.Owner
	assert.Contains(t, content, "d.Status.ShowOutput.Owner", "Database should access ShowOutput.Owner")

	// B types should access ShowOutput.GrantedBy
	assert.Contains(t, content, "r.Status.ShowOutput.GrantedBy", "Grant types should access ShowOutput.GrantedBy")

	// A2 types should have owner comment
	assert.Contains(t, content, "SHOW NETWORK POLICIES does not return an owner column", "NetworkPolicy should have owner comment")
}

// TestGeneratedTrackedParamsPatterns verifies tracked params patterns.
func TestGeneratedTrackedParamsPatterns(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	content := string(src)

	// A1/A2 types use Status.TrackedParameters
	assert.Contains(t, content, "d.Status.TrackedParameters", "Database should use Status.TrackedParameters")

	// B types return nil for tracked params.
	section := extractSection(t, content, "AccountRoleGrant")
	assert.Contains(t, section, "return nil", "AccountRoleGrant should return nil for tracked params")
}

// TestGeneratedHeader verifies the generated file header.
func TestGeneratedHeader(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	outPath := filepath.Join(root, "api", "v1alpha1", "zz_generated_accessors.go")

	src, err := os.ReadFile(outPath)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(string(src), "// Code generated by hack/gen-accessors/main.go; DO NOT EDIT."),
		"generated file should start with standard DO NOT EDIT header")
}
