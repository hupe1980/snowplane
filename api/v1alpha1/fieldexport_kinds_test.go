package v1alpha1

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldExportCELKindsInSync verifies that the CEL allowlist in
// fieldexport_types.go matches the authoritative FieldExportValidKinds
// generated from the gen-accessors type registry. If this test fails,
// regenerate with: go run hack/gen-accessors/main.go
func TestFieldExportCELKindsInSync(t *testing.T) {
	t.Parallel()

	// Read the fieldexport_types.go source to extract the CEL kind list.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	typesPath := filepath.Join(filepath.Dir(thisFile), "fieldexport_types.go")

	src, err := os.ReadFile(typesPath)
	require.NoError(t, err)

	// Extract the CEL kind list: everything inside [...] after "self.from.resource.kind in".
	re := regexp.MustCompile(`self\.from\.resource\.kind in \[([^\]]+)\]`)
	matches := re.FindSubmatch(src)
	require.NotEmpty(t, matches, "could not find CEL kind allowlist in fieldexport_types.go")

	// Parse the quoted kind names from the CEL list.
	celStr := string(matches[1])
	parts := strings.Split(celStr, ",")

	var celKinds []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'")
		if p != "" {
			celKinds = append(celKinds, p)
		}
	}

	sort.Strings(celKinds)

	// Compare against the generated list.
	genKinds := make([]string, len(FieldExportValidKinds))
	copy(genKinds, FieldExportValidKinds)
	sort.Strings(genKinds)

	assert.Equal(t, genKinds, celKinds,
		"CEL kind allowlist in fieldexport_types.go is out of sync with gen-accessors type registry. "+
			"Run 'go run hack/gen-accessors/main.go' and update the CEL rule in fieldexport_types.go.")
}
