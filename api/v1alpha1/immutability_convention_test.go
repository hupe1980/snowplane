package v1alpha1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImmutableFieldsHaveCELRules is a convention test that verifies every
// struct field documented as "Immutable after creation" has a corresponding
// CEL XValidation rule on the enclosing spec struct.
//
// This catches the case where a developer adds a kubebuilder comment marking
// a field immutable but forgets to add (or update) the CEL rule that actually
// enforces it at admission time.
func TestImmutableFieldsHaveCELRules(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dir := filepath.Dir(thisFile)

	files, err := filepath.Glob(filepath.Join(dir, "*_types.go"))
	require.NoError(t, err)

	// Fields that are immutable in CommonSpec and must have a CEL rule on
	// every embedding spec struct.
	commonImmutableFields := []string{"useRole"}

	type missing struct {
		File   string
		Struct string
		Field  string
	}

	var missingRules []missing

	for _, file := range files {
		src, err := os.ReadFile(file)
		require.NoError(t, err)

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, src, parser.ParseComments)
		require.NoError(t, err)

		baseName := filepath.Base(file)

		for _, decl := range f.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				structName := typeSpec.Name.Name
				if !strings.HasSuffix(structName, "Spec") {
					continue
				}

				// Collect CEL XValidation rules from the doc comments
				// on the struct definition.
				var celRules []string
				if genDecl.Doc != nil {
					for _, c := range genDecl.Doc.List {
						if strings.Contains(c.Text, "XValidation") && strings.Contains(c.Text, "oldSelf") {
							celRules = append(celRules, c.Text)
						}
					}
				}

				// Check if this struct embeds CommonSpec.
				embedsCommonSpec := false

				// Find fields documented as "Immutable after creation".
				for _, field := range structType.Fields.List {
					// Check for anonymously embedded CommonSpec.
					if len(field.Names) == 0 {
						if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "CommonSpec" {
							embedsCommonSpec = true
						}

						continue
					}

					fieldName := field.Names[0].Name

					// Get JSON tag name.
					jsonName := ""
					if field.Tag != nil {
						jsonName = extractJSONName(field.Tag.Value)
					}

					if jsonName == "" {
						jsonName = strings.ToLower(fieldName[:1]) + fieldName[1:]
					}

					// Check doc + line comments for "Immutable".
					isImmutable := false

					if field.Doc != nil {
						for _, c := range field.Doc.List {
							if strings.Contains(c.Text, "Immutable") {
								isImmutable = true

								break
							}
						}
					}

					if !isImmutable && field.Comment != nil {
						for _, c := range field.Comment.List {
							if strings.Contains(c.Text, "Immutable") {
								isImmutable = true

								break
							}
						}
					}

					if !isImmutable {
						continue
					}

					// Verify a CEL rule exists for this field.
					if !hasCELRuleForField(celRules, jsonName) {
						missingRules = append(missingRules, missing{
							File:   baseName,
							Struct: structName,
							Field:  jsonName,
						})
					}
				}

				// If the struct embeds CommonSpec, check that CommonSpec
				// immutable fields have CEL rules.
				if embedsCommonSpec {
					for _, f := range commonImmutableFields {
						if !hasCELRuleForField(celRules, f) {
							missingRules = append(missingRules, missing{
								File:   baseName,
								Struct: structName,
								Field:  f + " (from CommonSpec)",
							})
						}
					}
				}
			}
		}
	}

	if len(missingRules) > 0 {
		var sb strings.Builder
		sb.WriteString("The following fields are documented as 'Immutable after creation' but lack a CEL XValidation rule:\n\n")

		for _, m := range missingRules {
			sb.WriteString("  - ")
			sb.WriteString(m.File)
			sb.WriteString(" → ")
			sb.WriteString(m.Struct)
			sb.WriteString(".")
			sb.WriteString(m.Field)
			sb.WriteString("\n")
		}

		sb.WriteString("\nAdd a +kubebuilder:validation:XValidation rule using oldSelf to enforce immutability at admission time.")
		assert.Fail(t, sb.String())
	}
}

// extractJSONName returns the JSON field name from a struct tag value like `json:"name,omitempty"`.
func extractJSONName(tagValue string) string {
	re := regexp.MustCompile(`json:"([^",]+)`)
	matches := re.FindStringSubmatch(tagValue)

	if len(matches) < 2 {
		return ""
	}

	name := matches[1]
	if name == "-" {
		return ""
	}

	return name
}

// hasCELRuleForField checks whether any CEL rule mentions the field in an
// immutability pattern like "self.<field> == oldSelf.<field>".
func hasCELRuleForField(celRules []string, jsonFieldName string) bool {
	for _, rule := range celRules {
		// Match patterns like:
		//   self.name == oldSelf.name
		//   has(oldSelf.useRole) == has(self.useRole) && ...
		if strings.Contains(rule, "oldSelf."+jsonFieldName) && strings.Contains(rule, "self."+jsonFieldName) {
			return true
		}
	}

	return false
}
