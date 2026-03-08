// Package sqlstatement implements the reconciler for SQLStatement resources.
package sqlstatement

import (
	"fmt"
	"regexp"
	"strings"
)

// StatementDenylist blocks SQL statements whose leading keyword matches any
// denied pattern. Each entry is a case-insensitive word boundary pattern
// compiled from a simple keyword string (e.g., "DROP DATABASE" becomes
// `(?i)\bDROP\s+DATABASE\b`).
//
// The denylist is evaluated against the raw SQL before execution. If any
// statement in a multi-statement SQL string matches, the entire execution
// is rejected. This is defense-in-depth — it does not replace Snowflake
// RBAC or Kubernetes RBAC for SQLStatement resources.
type StatementDenylist struct {
	patterns []*denyEntry
}

type denyEntry struct {
	raw string
	re  *regexp.Regexp
}

// NewStatementDenylist creates a denylist from a list of keyword patterns.
// Each keyword is a space-separated SQL keyword sequence:
//
//	"DROP DATABASE"  → blocks "DROP DATABASE foo", "drop database bar"
//	"ALTER USER"     → blocks "ALTER USER admin SET ..."
//	"TRUNCATE"       → blocks "TRUNCATE TABLE ..."
//	"DROP SCHEMA"    → blocks "DROP SCHEMA public"
//
// Returns an error if any keyword pattern is empty or cannot be compiled.
func NewStatementDenylist(keywords []string) (*StatementDenylist, error) {
	entries := make([]*denyEntry, 0, len(keywords))

	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}

		// Build regex: word-boundary + each keyword word separated by \s+
		words := strings.Fields(strings.ToUpper(kw))
		pattern := `(?i)\b` + strings.Join(words, `\s+`) + `\b`

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid denylist pattern %q: %w", kw, err)
		}

		entries = append(entries, &denyEntry{raw: strings.Join(words, " "), re: re})
	}

	return &StatementDenylist{patterns: entries}, nil
}

// ParseStatementDenylist creates a denylist from a comma-separated string
// of keyword patterns (as used by the CLI flag and Helm values).
func ParseStatementDenylist(csv string) (*StatementDenylist, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return &StatementDenylist{}, nil
	}

	parts := strings.Split(csv, ",")
	keywords := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keywords = append(keywords, p)
		}
	}

	return NewStatementDenylist(keywords)
}

// Check validates SQL against the denylist.
// Returns nil if the SQL is allowed, or an error describing the first match.
func (d *StatementDenylist) Check(sql string) error {
	if d == nil || len(d.patterns) == 0 {
		return nil
	}

	for _, entry := range d.patterns {
		if entry.re.MatchString(sql) {
			return fmt.Errorf("statement denied: SQL matches blocked pattern %q", entry.raw)
		}
	}

	return nil
}

// IsEmpty returns true when the denylist has no patterns configured.
func (d *StatementDenylist) IsEmpty() bool {
	return d == nil || len(d.patterns) == 0
}

// Len returns the number of deny patterns.
func (d *StatementDenylist) Len() int {
	if d == nil {
		return 0
	}

	return len(d.patterns)
}
