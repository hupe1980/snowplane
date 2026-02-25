package sqlbuilder

import (
	"strings"
	"testing"
)

// FuzzValidateColumnDefault verifies that ValidateColumnDefault either accepts
// or rejects every input, and that accepted values never contain the denied
// patterns that could enable SQL injection.
func FuzzValidateColumnDefault(f *testing.F) {
	// Seed corpus: known-good and known-bad inputs.
	seeds := []string{
		"",
		"NULL",
		"0",
		"'hello'",
		"CURRENT_TIMESTAMP()",
		"SEQ.NEXTVAL",
		"'2024-01-01'::DATE",
		"TRUE",
		"FALSE",
		"-1.5",
		"''",
		"1; DROP TABLE x",
		"value -- comment",
		"value /* injected */",
		"start */ end",
		"$$malicious$$",
		"COPY INTO @stage",
		"EXECUTE IMMEDIATE 'DROP TABLE x'",
		"CALL system_fn()",
		"SYSTEM$TYPEOF(1)",
		"'unbalanced",
		"'a'b'",
		strings.Repeat("x", 1025),
		"normal_value",
		"'it''s escaped'",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		err := ValidateColumnDefault(input)
		if err != nil {
			// Rejected — that's fine, the validator caught something.
			return
		}

		// If accepted, the input must not contain any of the denied patterns.
		// This is the key safety property we want to verify via fuzzing.
		upper := strings.ToUpper(input)

		if strings.Contains(input, ";") {
			t.Errorf("ValidateColumnDefault accepted input with semicolon: %q", input)
		}
		if strings.Contains(input, "--") {
			t.Errorf("ValidateColumnDefault accepted input with line comment: %q", input)
		}
		if strings.Contains(input, "/*") {
			t.Errorf("ValidateColumnDefault accepted input with block comment open: %q", input)
		}
		if strings.Contains(input, "*/") {
			t.Errorf("ValidateColumnDefault accepted input with block comment close: %q", input)
		}
		if strings.Contains(input, "$$") {
			t.Errorf("ValidateColumnDefault accepted input with dollar quoting: %q", input)
		}
		if containsWord(upper, "COPY") {
			t.Errorf("ValidateColumnDefault accepted input with COPY keyword: %q", input)
		}
		if containsWord(upper, "EXECUTE") {
			t.Errorf("ValidateColumnDefault accepted input with EXECUTE keyword: %q", input)
		}
		if containsWord(upper, "CALL") {
			t.Errorf("ValidateColumnDefault accepted input with CALL keyword: %q", input)
		}
		if strings.Contains(upper, "SYSTEM$") {
			// The denylist uses \bSYSTEM\$ — only blocks at word boundaries.
			// Check if SYSTEM$ appears at a word boundary.
			idx := strings.Index(upper, "SYSTEM$")
			if idx == 0 || !isWordChar(upper[idx-1]) {
				t.Errorf("ValidateColumnDefault accepted input with SYSTEM$ keyword: %q", input)
			}
		}
		if strings.Count(input, "'")%2 != 0 {
			t.Errorf("ValidateColumnDefault accepted input with unbalanced quotes: %q", input)
		}
		if len(input) > 1024 {
			t.Errorf("ValidateColumnDefault accepted input exceeding max length: len=%d", len(input))
		}
	})
}

// FuzzValidateFileFormat verifies that ValidateFileFormat either accepts
// or rejects every input, and that accepted values never contain the denied
// patterns that could enable SQL injection.
func FuzzValidateFileFormat(f *testing.F) {
	seeds := []string{
		"",
		"FORMAT_NAME = 'MY_FORMAT'",
		"TYPE = CSV",
		"TYPE = CSV FIELD_DELIMITER = ','",
		"TYPE = CSV; DROP TABLE x",
		"FORMAT_NAME = 'x' -- comment",
		"TYPE = CSV /* block comment */",
		"TYPE = CSV $$ dollar $$",
		"TYPE = CSV COPY INTO x",
		"TYPE = CSV EXECUTE IMMEDIATE",
		"TYPE = CSV CALL proc()",
		"TYPE = CSV SYSTEM$TYPEOF(x)",
		strings.Repeat("x", 2049),
		"TYPE = JSON STRIP_OUTER_ARRAY = TRUE",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		err := ValidateFileFormat(input)
		if err != nil {
			return
		}

		upper := strings.ToUpper(input)

		if strings.Contains(input, ";") {
			t.Errorf("ValidateFileFormat accepted input with semicolon: %q", input)
		}
		if strings.Contains(input, "--") {
			t.Errorf("ValidateFileFormat accepted input with line comment: %q", input)
		}
		if strings.Contains(input, "/*") {
			t.Errorf("ValidateFileFormat accepted input with block comment open: %q", input)
		}
		if strings.Contains(input, "*/") {
			t.Errorf("ValidateFileFormat accepted input with block comment close: %q", input)
		}
		if strings.Contains(input, "$$") {
			t.Errorf("ValidateFileFormat accepted input with dollar quoting: %q", input)
		}
		if containsWord(upper, "COPY") {
			t.Errorf("ValidateFileFormat accepted input with COPY keyword: %q", input)
		}
		if containsWord(upper, "EXECUTE") {
			t.Errorf("ValidateFileFormat accepted input with EXECUTE keyword: %q", input)
		}
		if containsWord(upper, "CALL") {
			t.Errorf("ValidateFileFormat accepted input with CALL keyword: %q", input)
		}
		if strings.Contains(upper, "SYSTEM$") {
			idx := strings.Index(upper, "SYSTEM$")
			if idx == 0 || !isWordChar(upper[idx-1]) {
				t.Errorf("ValidateFileFormat accepted input with SYSTEM$ keyword: %q", input)
			}
		}
		if len(input) > 2048 {
			t.Errorf("ValidateFileFormat accepted input exceeding max length: len=%d", len(input))
		}
	})
}

// containsWord checks if s contains the word w surrounded by word boundaries.
// This mimics the \b regex behavior used in the deny regexes.
func containsWord(s, w string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], w)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		beforeOK := absPos == 0 || !isWordChar(s[absPos-1])
		afterPos := absPos + len(w)
		afterOK := afterPos >= len(s) || !isWordChar(s[afterPos])
		if beforeOK && afterOK {
			return true
		}
		idx = absPos + 1
	}
}

func isWordChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}
