package snowflake

import (
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// buildLocationList formats a list of URIs for Snowflake SQL, e.g. ('s3://bucket/path/', 's3://bucket/other/').
func buildLocationList(locs []string) string {
	quoted := make([]string, len(locs))
	for i, loc := range locs {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(loc))
	}

	return fmt.Sprintf("(%s)", strings.Join(quoted, ", "))
}

// buildStringListClause builds a "KEYWORD = ('a', 'b', 'c')" SQL clause from a list of strings.
func buildStringListClause(keyword string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
	}

	return fmt.Sprintf("%s = (%s)", keyword, strings.Join(quoted, ", "))
}

// buildEmailListClause builds an email-list SQL clause (alias for buildStringListClause).
func buildEmailListClause(keyword string, vals []string) string {
	return buildStringListClause(keyword, vals)
}
