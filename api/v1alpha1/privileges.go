package v1alpha1

import "strings"

// knownPrivileges is the set of Snowflake privilege names recognised by
// Snowplane. The set is intentionally not exhaustive — Snowflake adds new
// privileges over time. Unknown privileges trigger a Warning event rather
// than a blocking error.
var knownPrivileges = map[string]bool{
	// Global (account-level) privileges.
	"ALL PRIVILEGES":               true,
	"ALL":                          true,
	"CREATE ACCOUNT":               true,
	"CREATE DATA EXCHANGE LISTING": true,
	"CREATE DATABASE":              true,
	"CREATE FAILOVER GROUP":        true,
	"CREATE INTEGRATION":           true,
	"CREATE NETWORK POLICY":        true,
	"CREATE REPLICATION GROUP":     true,
	"CREATE ROLE":                  true,
	"CREATE SHARE":                 true,
	"CREATE USER":                  true,
	"CREATE WAREHOUSE":             true,
	"APPLY MASKING POLICY":         true,
	"APPLY ROW ACCESS POLICY":      true,
	"APPLY SESSION POLICY":         true,
	"APPLY TAG":                    true,
	"ATTACH POLICY":                true,
	"EXECUTE MANAGED TASK":         true,
	"EXECUTE TASK":                 true,
	"IMPORT SHARE":                 true,
	"MANAGE GRANTS":                true,
	"MONITOR EXECUTION":            true,
	"MONITOR USAGE":                true,
	"OVERRIDE SHARE RESTRICTIONS":  true,
	"RESOLVE ALL":                  true,
	"CREATE APPLICATION":           true,
	"CREATE APPLICATION PACKAGE":   true,
	"CREATE COMPUTE POOL":          true,
	"CREATE EXTERNAL VOLUME":       true,

	// Database privileges.
	"CREATE SCHEMA":       true,
	"IMPORTED PRIVILEGES": true,
	"MODIFY":              true,
	"MONITOR":             true,
	"USAGE":               true,

	// Schema privileges.
	"CREATE TABLE":                 true,
	"CREATE EXTERNAL TABLE":        true,
	"CREATE VIEW":                  true,
	"CREATE MATERIALIZED VIEW":     true,
	"CREATE TEMPORARY TABLE":       true,
	"CREATE SEQUENCE":              true,
	"CREATE FUNCTION":              true,
	"CREATE PROCEDURE":             true,
	"CREATE FILE FORMAT":           true,
	"CREATE STAGE":                 true,
	"CREATE PIPE":                  true,
	"CREATE STREAM":                true,
	"CREATE TASK":                  true,
	"CREATE TAG":                   true,
	"CREATE MASKING POLICY":        true,
	"CREATE ROW ACCESS POLICY":     true,
	"CREATE SESSION POLICY":        true,
	"CREATE PASSWORD POLICY":       true,
	"CREATE AUTHENTICATION POLICY": true,
	"CREATE NETWORK RULE":          true,
	"CREATE SECRET":                true,
	"CREATE ALERT":                 true,
	"CREATE DYNAMIC TABLE":         true,
	"CREATE STREAMLIT":             true,
	"ADD SEARCH OPTIMIZATION":      true,

	// Object-level privileges.
	"SELECT":          true,
	"INSERT":          true,
	"UPDATE":          true,
	"DELETE":          true,
	"TRUNCATE":        true,
	"REFERENCES":      true,
	"REBUILD":         true,
	"OPERATE":         true,
	"READ":            true,
	"WRITE":           true,
	"OWNERSHIP":       true,
	"EVOLVE SCHEMA":   true,
	"REFERENCE_USAGE": true,
	"APPLY BUDGET":    true,
	"APPLYBUDGET":     true,
}

// IsKnownPrivilege reports whether the given privilege name is in the set of
// known Snowflake privileges. Comparison is case-insensitive. Unknown
// privileges are not necessarily invalid — Snowflake may have added new
// privilege names that Snowplane has not yet catalogued.
func IsKnownPrivilege(privilege string) bool {
	return knownPrivileges[strings.ToUpper(strings.TrimSpace(privilege))]
}
