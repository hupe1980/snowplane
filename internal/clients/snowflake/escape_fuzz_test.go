package snowflake

import (
	"strings"
	"testing"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// FuzzEscapeString verifies that EscapeString always produces output that is
// safe inside a single-quoted SQL string literal.  The escaped result must:
//  1. Never contain an unescaped single quote (odd run of quotes).
//  2. Never contain an unescaped backslash.
//  3. Round-trip: unescaping (\\\\ -> \\, ” -> ') yields original.
func FuzzEscapeString(f *testing.F) {
	// Seed corpus with edge cases.
	seeds := []string{
		"",
		"hello",
		"it's",
		"a'b'c",
		"''",
		"'''",
		"' OR 1=1 --",
		"'; DROP TABLE x; --",
		"\x00",
		"null\x00byte",
		"back\\slash",
		"unicode: \u00f1 \u00e9 \U0001f60e",
		"tab\there",
		"newline\nhere",
		"carriage\rreturn",
		"mixed'quote\"double",
		"\\",
		"\\'",
		"'\\",
		"C:\\Users\\docs",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		escaped := sqlbuilder.EscapeString(input)

		// Property 1: No unescaped single quotes.  After replacing all ''
		// pairs with nothing, no ' should remain.
		stripped := strings.ReplaceAll(escaped, "''", "")
		if strings.Contains(stripped, "'") {
			t.Errorf("EscapeString(%q) = %q contains unescaped single quote", input, escaped)
		}

		// Property 2: No unescaped backslashes.  After replacing all \\\\
		// pairs with nothing, no \ should remain.
		strippedBS := strings.ReplaceAll(escaped, `\\`, "")
		if strings.Contains(strippedBS, `\`) {
			t.Errorf("EscapeString(%q) = %q contains unescaped backslash", input, escaped)
		}

		// Property 3: Round-trip.  Unescaping '' -> ' and \\\\ -> \\ should
		// yield the original (with NUL bytes stripped, since EscapeString
		// intentionally removes them for safety).
		unescaped := strings.ReplaceAll(escaped, "''", "'")
		unescaped = strings.ReplaceAll(unescaped, `\\`, `\`)
		expected := strings.ReplaceAll(input, "\x00", "")
		if unescaped != expected {
			t.Errorf("EscapeString(%q) = %q does not round-trip: unescaped=%q, expected=%q", input, escaped, unescaped, expected)
		}
	})
}

// FuzzEscapeLikePattern verifies that EscapeLikePattern produces safe LIKE
// pattern text.  The result must:
//  1. Not contain unescaped LIKE wildcards (% or _).
//  2. Not contain unescaped single quotes.
//  3. Preserve literal backslashes as double-backslashes.
func FuzzEscapeLikePattern(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"100%",
		"DB_NAME",
		"a_b%c'd",
		`my\db`,
		`my\%db`,
		`a'b%c_d\e`,
		"' OR 1=1 --",
		"\x00",
		"back\\slash%wild_card'quote",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		escaped := sqlbuilder.EscapeLikePattern(input)

		// Property 1: After removing all escape sequences (\%, \_, \\, ''),
		// no literal %, _, \, or ' should remain.
		cleaned := escaped
		cleaned = strings.ReplaceAll(cleaned, `\\`, "") // remove escaped backslashes
		cleaned = strings.ReplaceAll(cleaned, `\%`, "") // remove escaped %
		cleaned = strings.ReplaceAll(cleaned, `\_`, "") // remove escaped _
		cleaned = strings.ReplaceAll(cleaned, "''", "") // remove escaped quotes

		if strings.ContainsAny(cleaned, `%_\`) {
			t.Errorf("EscapeLikePattern(%q) = %q contains unescaped special chars after cleanup: %q",
				input, escaped, cleaned)
		}

		if strings.Contains(cleaned, "'") {
			t.Errorf("EscapeLikePattern(%q) = %q contains unescaped single quote after cleanup: %q",
				input, escaped, cleaned)
		}
	})
}

// FuzzQuoteIdentifier verifies that quoteIdentifier always produces output
// that is a valid double-quoted SQL identifier.  The result must:
//  1. Start and end with double quotes.
//  2. Never contain an unescaped double quote (odd run inside).
//  3. Round-trip back to the original name.
func FuzzQuoteIdentifier(f *testing.F) {
	seeds := []string{
		"MY_DB",
		`has"quote`,
		`" OR 1=1 --`,
		"",
		"a",
		`""`,
		`"""`,
		"\x00",
		"Ã±",
		"back\\slash",
		"tab\there",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		quoted := quoteIdentifier(input)

		// Property 1: Must start and end with double quote.
		if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
			t.Fatalf("quoteIdentifier(%q) = %q is not wrapped in double quotes", input, quoted)
		}

		// Property 2: Inner content must not contain unescaped double quotes.
		inner := quoted[1 : len(quoted)-1]
		stripped := strings.ReplaceAll(inner, `""`, "")
		if strings.Contains(stripped, `"`) {
			t.Errorf("quoteIdentifier(%q) = %q contains unescaped double quote in inner: %q", input, quoted, inner)
		}

		// Property 3: Round-trip via ParseDatabaseNameFromFQN should yield
		// original (with NUL bytes stripped, since QuoteIdentifier
		// intentionally removes them for safety).
		roundTripped := ParseDatabaseNameFromFQN(quoted)
		expected := strings.ReplaceAll(input, "\x00", "")
		if roundTripped != expected {
			t.Errorf("quoteIdentifier(%q) = %q does not round-trip: got %q, expected %q", input, quoted, roundTripped, expected)
		}
	})
}
