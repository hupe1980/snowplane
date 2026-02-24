// Package sqlbuilder provides type-safe SQL DDL construction for Snowflake.
//
// The package centralises escaping, quoting, and statement assembly so that
// each resource file only describes *what* to set, not *how* to safely embed
// values.
//
// Because the gosnowflake driver does not support bind parameters (?) for DDL
// statements, all values must be embedded directly in the SQL string.  This
// package ensures they are correctly escaped.
package sqlbuilder

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// EscapeString escapes backslashes and single quotes for Snowflake
// single-quoted string literals.  Backslash must be escaped first because
// Snowflake treats it as an escape character by default (e.g. \' is
// interpreted as an escaped quote, which would leave the literal unterminated).
// NUL bytes are stripped because they are never valid in Snowflake string
// literals and can cause truncation in C-based database drivers.
//
// Ref: https://docs.snowflake.com/en/sql-reference/data-types-text#single-quoted-string-constants
func EscapeString(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "''")

	return s
}

// EscapeLikePattern escapes SQL LIKE wildcard characters (%, _), backslash
// (the LIKE escape character), and single quotes so that the pattern matches
// the literal name.  NUL bytes are stripped for the same safety reasons as
// EscapeString.
func EscapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "''")
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)

	return s
}

// QuoteIdentifier wraps a name in double quotes, escaping any embedded quotes.
// NUL bytes are stripped for safety.
func QuoteIdentifier(name string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
}

// QuoteIdentifierParts quotes a multi-part identifier by splitting on "."
// and quoting each component individually.
// Examples:
//
//	"MY_DB"       → `"MY_DB"`
//	"MY_DB.PUBLIC" → `"MY_DB"."PUBLIC"`
//	"DB.SCH.TBL"  → `"DB"."SCH"."TBL"`
func QuoteIdentifierParts(fqn string) string {
	parts := strings.Split(fqn, ".")

	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = QuoteIdentifier(strings.TrimSpace(p))
	}

	return strings.Join(quoted, ".")
}

// validObjectTypes is the set of valid Snowflake object type keywords used in
// GRANT statements.  These are SQL keywords, not user identifiers, so they
// must NOT be quoted but MUST be validated against this allowlist to prevent
// SQL injection via the ObjectType fields.
var validObjectTypes = map[string]bool{
	// Account-level objects
	"DATABASE": true, "WAREHOUSE": true, "USER": true,
	"RESOURCE MONITOR": true, "COMPUTE POOL": true,
	"INTEGRATION": true, "CONNECTION": true,
	"FAILOVER GROUP": true, "REPLICATION GROUP": true,
	"EXTERNAL VOLUME": true,
	// Schema-level objects
	"TABLE": true, "VIEW": true, "MATERIALIZED VIEW": true,
	"STAGE": true, "FILE FORMAT": true, "FUNCTION": true,
	"PROCEDURE": true, "STREAM": true, "TASK": true,
	"PIPE": true, "SEQUENCE": true, "TAG": true,
	"MASKING POLICY": true, "ROW ACCESS POLICY": true,
	"ALERT": true, "SECRET": true, "MODEL": true,
	"DYNAMIC TABLE": true, "ICEBERG TABLE": true,
	"EVENT TABLE": true, "EXTERNAL TABLE": true,
	// Plural forms for ALL/FUTURE grants
	"TABLES": true, "VIEWS": true, "MATERIALIZED VIEWS": true,
	"STAGES": true, "FILE FORMATS": true, "FUNCTIONS": true,
	"PROCEDURES": true, "STREAMS": true, "TASKS": true,
	"PIPES": true, "SEQUENCES": true, "TAGS": true,
	"MASKING POLICIES": true, "ROW ACCESS POLICIES": true,
	"ALERTS": true, "SECRETS": true, "MODELS": true,
	"DYNAMIC TABLES": true, "ICEBERG TABLES": true,
	"EVENT TABLES": true, "EXTERNAL TABLES": true,
	"SCHEMAS": true,
	// Schema itself
	"SCHEMA": true,
	// Account
	"ACCOUNT": true,
}

// ValidateObjectType checks whether ty is a known Snowflake object type keyword.
// Returns an error if the type is not in the allowlist, preventing injection
// through the ObjectType fields in GRANT statements.
func ValidateObjectType(ty string) error {
	upper := strings.ToUpper(strings.TrimSpace(ty))
	if upper == "" {
		return fmt.Errorf("object type must not be empty")
	}

	if !validObjectTypes[upper] {
		return fmt.Errorf("unknown object type %q", ty)
	}

	return nil
}

// ValidateKeywordValue asserts that s matches the pattern for SQL keyword values
// (uppercase letters, digits, underscores, and spaces only).
// Use this as a defense-in-depth check for enum-like SetKeyword values.
func ValidateKeywordValue(s string) error {
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != ' ' {
			return fmt.Errorf("invalid keyword value character %q in %q", string(c), s)
		}
	}

	return nil
}

// columnTypeRe matches valid Snowflake column type declarations.
// Permits letters, digits, underscores, parentheses (for precision/scale),
// commas, spaces, and hyphens (e.g. TIMESTAMP WITH TIME ZONE).
// Max length 256 prevents abuse.
var columnTypeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ (),]*$`)

// ValidateColumnType validates a Snowflake column type string.
// Examples of valid types: VARCHAR, NUMBER(38,0), TIMESTAMP_LTZ, ARRAY,
// VARCHAR(100), FLOAT.
func ValidateColumnType(ty string) error {
	if ty == "" {
		return fmt.Errorf("column type must not be empty")
	}

	if len(ty) > 256 {
		return fmt.Errorf("column type too long (%d chars)", len(ty))
	}

	if !columnTypeRe.MatchString(ty) {
		return fmt.Errorf("invalid column type %q: must contain only alphanumeric, spaces, underscores, parentheses, and commas", ty)
	}

	return nil
}

// columnDefaultDenyRe matches dangerous patterns in column default expressions.
// Rejects: semicolons, SQL comment markers (-- and /* */), dollar-quoting ($$),
// and COPY/EXECUTE/CALL statements that could be abused for injection.
var columnDefaultDenyRe = regexp.MustCompile(`(?i)[;]|--|/\*|\*/|\$\$|\bCOPY\b|\bEXECUTE\b|\bCALL\b|\bSYSTEM\$`)

// ValidateColumnDefault validates a Snowflake column DEFAULT expression.
// Rejects values containing semicolons, SQL comment markers, dollar-quoting,
// or dangerous keywords to prevent SQL injection.  Also validates that
// single quotes are balanced (an odd number would break out of context).
func ValidateColumnDefault(def string) error {
	if def == "" {
		return fmt.Errorf("column default must not be empty")
	}

	if len(def) > 1024 {
		return fmt.Errorf("column default too long (%d chars)", len(def))
	}

	if columnDefaultDenyRe.MatchString(def) {
		return fmt.Errorf("invalid column default %q: contains forbidden pattern (semicolons, comment markers, dollar-quoting, or dangerous keywords)", def)
	}

	// Check for balanced single quotes — an odd count would leave a literal
	// unterminated and allow injection in the surrounding SQL context.
	if count := strings.Count(def, "'"); count%2 != 0 {
		return fmt.Errorf("invalid column default %q: unbalanced single quotes", def)
	}

	return nil
}

// validEncryptionTypes is the set of valid Snowflake stage encryption types.
var validEncryptionTypes = map[string]bool{
	"SNOWFLAKE_FULL": true,
	"SNOWFLAKE_SSE":  true,
	"AWS_SSE_S3":     true,
	"AWS_SSE_KMS":    true,
	"AWS_CSE":        true,
	"GCS_SSE_KMS":    true,
	"AZURE_CSE":      true,
	"NONE":           true,
}

// ValidateEncryptionType checks whether ty is a known Snowflake stage encryption type.
func ValidateEncryptionType(ty string) error {
	upper := strings.ToUpper(strings.TrimSpace(ty))
	if upper == "" {
		return fmt.Errorf("encryption type must not be empty")
	}

	if !validEncryptionTypes[upper] {
		return fmt.Errorf("unknown encryption type %q", ty)
	}

	return nil
}

// fileFormatDenyRe rejects dangerous patterns in FILE_FORMAT expressions.
// Aligned with columnDefaultDenyRe: blocks semicolons, line comments (--),
// block comments (/* */), dollar-quoting ($$), and dangerous Snowflake
// keywords (COPY, EXECUTE, CALL, SYSTEM$).
var fileFormatDenyRe = regexp.MustCompile(`(?i)[;]|--|/\*|\*/|\$\$|\bCOPY\b|\bEXECUTE\b|\bCALL\b|\bSYSTEM\$`)

// ValidateFileFormat validates a Snowflake FILE_FORMAT expression.
// FILE_FORMAT values can be complex (FORMAT_NAME = 'name' or TYPE = CSV
// FIELD_DELIMITER = ',' etc.) but must not contain semicolons, comment
// markers, dollar-quoting, or dangerous keywords that could enable SQL
// injection.
func ValidateFileFormat(ff string) error {
	if ff == "" {
		return fmt.Errorf("file format must not be empty")
	}

	if len(ff) > 2048 {
		return fmt.Errorf("file format too long (%d chars)", len(ff))
	}

	if fileFormatDenyRe.MatchString(ff) {
		return fmt.Errorf("invalid file format %q: contains forbidden pattern (semicolons, comment markers, dollar-quoting, or dangerous keywords)", ff)
	}

	return nil
}

// ValidateUnsetField validates a field name used in UNSET clauses.
// Only allows uppercase letters, digits, and underscores — the format used
// by Snowflake session and object parameter names.
func ValidateUnsetField(field string) error {
	if field == "" {
		return fmt.Errorf("unset field name must not be empty")
	}

	if err := ValidateKeywordValue(field); err != nil {
		return fmt.Errorf("invalid unset field %q: %w", field, err)
	}

	return nil
}

// BoolToSQL returns the uppercase SQL boolean literal "TRUE" or "FALSE".
func BoolToSQL(v bool) string {
	if v {
		return "TRUE"
	}

	return "FALSE"
}

// Builder assists in building Snowflake DDL statements.
// It wraps a strings.Builder and provides methods for appending type-safe
// Snowflake-specific SQL fragments.
//
// Methods that validate input accumulate errors internally (via errors.Join)
// rather than panicking.  Callers must check Err() after building the statement.
type Builder struct {
	strings.Builder
	err error
}

// Err returns all validation errors accumulated during statement building,
// or nil if no errors occurred.  Multiple errors are joined via errors.Join.
func (b *Builder) Err() error {
	return b.err
}

// SetString appends ` KEY = 'escaped_value'` when value is non-nil.
func (b *Builder) SetString(key string, value *string) {
	if value != nil {
		fmt.Fprintf(&b.Builder, " %s = '%s'", key, EscapeString(*value))
	}
}

// SetInt32 appends ` KEY = value` when value is non-nil.
func (b *Builder) SetInt32(key string, value *int32) {
	if value != nil {
		fmt.Fprintf(&b.Builder, " %s = %d", key, *value)
	}
}

// SetBool appends ` KEY = TRUE|FALSE` when value is non-nil.
func (b *Builder) SetBool(key string, value *bool) {
	if value != nil {
		fmt.Fprintf(&b.Builder, " %s = %s", key, BoolToSQL(*value))
	}
}

// SetKeyword appends ` KEY = value` (unquoted, no escaping) when value is non-nil.
// Use this for enum-like Snowflake parameters such as WAREHOUSE_TYPE, WAREHOUSE_SIZE,
// STORAGE_SERIALIZATION_POLICY, METRIC_LEVEL, TRACE_LEVEL, etc.
// Records an error (retrievable via Err()) if the value contains characters
// outside [A-Za-z0-9_ ] as defense-in-depth.
func (b *Builder) SetKeyword(key string, value *string) {
	if value != nil {
		if err := ValidateKeywordValue(*value); err != nil {
			b.err = errors.Join(b.err, fmt.Errorf("SetKeyword(%q, %q): %w", key, *value, err))
			return
		}

		fmt.Fprintf(&b.Builder, " %s = %s", key, *value)
	}
}

// SetQuotedKeyword appends ` KEY = 'value'` (quoted but NOT string-escaped) when value is non-nil.
// Use this for enum-like parameters that Snowflake expects in single quotes,
// such as LOG_LEVEL.
// Records an error (retrievable via Err()) if the value contains characters
// outside [A-Za-z0-9_ ] as defense-in-depth.
func (b *Builder) SetQuotedKeyword(key string, value *string) {
	if value != nil {
		if err := ValidateKeywordValue(*value); err != nil {
			b.err = errors.Join(b.err, fmt.Errorf("SetQuotedKeyword(%q, %q): %w", key, *value, err))
			return
		}

		fmt.Fprintf(&b.Builder, " %s = '%s'", key, *value)
	}
}

// SetClauses collects individual SET clauses for ALTER statements and
// provides methods to emit them as a single ALTER ... SET statement.
//
// Methods that validate input accumulate errors internally (via errors.Join)
// rather than panicking.  Callers must check Err() after building clauses.
type SetClauses struct {
	clauses []string
	err     error
}

// Err returns all validation errors accumulated during clause building,
// or nil if no errors occurred.  Multiple errors are joined via errors.Join.
func (sc *SetClauses) Err() error {
	return sc.err
}

// String appends a `KEY = 'escaped_value'` clause when value is non-nil.
func (sc *SetClauses) String(key string, value *string) {
	if value != nil {
		sc.clauses = append(sc.clauses, fmt.Sprintf("%s = '%s'", key, EscapeString(*value)))
	}
}

// Int32 appends a `KEY = value` clause when value is non-nil.
func (sc *SetClauses) Int32(key string, value *int32) {
	if value != nil {
		sc.clauses = append(sc.clauses, fmt.Sprintf("%s = %d", key, *value))
	}
}

// Bool appends a `KEY = TRUE|FALSE` clause when value is non-nil.
func (sc *SetClauses) Bool(key string, value *bool) {
	if value != nil {
		sc.clauses = append(sc.clauses, fmt.Sprintf("%s = %s", key, BoolToSQL(*value)))
	}
}

// Keyword appends a `KEY = value` (unquoted) clause when value is non-nil.
// Records an error (retrievable via Err()) if the value contains characters
// outside [A-Za-z0-9_ ] as defense-in-depth.
func (sc *SetClauses) Keyword(key string, value *string) {
	if value != nil {
		if err := ValidateKeywordValue(*value); err != nil {
			sc.err = errors.Join(sc.err, fmt.Errorf("SetClauses.Keyword(%q, %q): %w", key, *value, err))
			return
		}

		sc.clauses = append(sc.clauses, fmt.Sprintf("%s = %s", key, *value))
	}
}

// QuotedKeyword appends a `KEY = 'value'` (quoted, not string-escaped) clause
// when value is non-nil.
// Records an error (retrievable via Err()) if the value contains characters
// outside [A-Za-z0-9_ ] as defense-in-depth.
func (sc *SetClauses) QuotedKeyword(key string, value *string) {
	if value != nil {
		if err := ValidateKeywordValue(*value); err != nil {
			sc.err = errors.Join(sc.err, fmt.Errorf("SetClauses.QuotedKeyword(%q, %q): %w", key, *value, err))
			return
		}

		sc.clauses = append(sc.clauses, fmt.Sprintf("%s = '%s'", key, *value))
	}
}

// Raw appends a pre-formatted clause string directly.
// Use this for clauses with non-standard syntax that don't fit the other methods.
//
// SAFETY: Callers MUST validate the input before calling Raw().
// This method performs no escaping or validation — it trusts the caller
// to have applied appropriate validation (e.g. ValidateFileFormat,
// ValidateEncryptionType) before constructing the clause string.
func (sc *SetClauses) Raw(clause string) {
	sc.clauses = append(sc.clauses, clause)
}

// HasClauses reports whether any clauses have been added.
func (sc *SetClauses) HasClauses() bool {
	return len(sc.clauses) > 0
}

// BuildAlter returns the full ALTER <objectType> <identifier> SET <clauses> statement.
// Returns ("", nil) if no clauses were added.
// Returns an error if any Keyword/QuotedKeyword validation failed.
func (sc *SetClauses) BuildAlter(objectType, identifier string) (string, error) {
	if sc.err != nil {
		return "", sc.err
	}

	if len(sc.clauses) == 0 {
		return "", nil
	}

	return fmt.Sprintf("ALTER %s %s SET %s", objectType, identifier, strings.Join(sc.clauses, " ")), nil
}

// BuildUnset returns the ALTER <objectType> <identifier> UNSET <fields> statement.
// Returns "" if unsetFields is empty.
// Returns an error if any field name is invalid.
func BuildUnset(objectType, identifier string, unsetFields []string) (string, error) {
	if len(unsetFields) == 0 {
		return "", nil
	}

	for _, f := range unsetFields {
		if err := ValidateUnsetField(f); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("ALTER %s %s UNSET %s", objectType, identifier, strings.Join(unsetFields, ", ")), nil
}

// BuildAlterStatements returns a list of ALTER statements (SET and/or UNSET)
// based on the given SetClauses and unsetFields.
func BuildAlterStatements(objectType, identifier string, sc *SetClauses, unsetFields []string) ([]string, error) {
	var statements []string

	set, err := sc.BuildAlter(objectType, identifier)
	if err != nil {
		return nil, fmt.Errorf("building SET clause: %w", err)
	}

	if set != "" {
		statements = append(statements, set)
	}

	unset, err := BuildUnset(objectType, identifier, unsetFields)
	if err != nil {
		return nil, fmt.Errorf("building UNSET clause: %w", err)
	}

	if unset != "" {
		statements = append(statements, unset)
	}

	return statements, nil
}

// ShowLike builds a SHOW <objectType> LIKE '<pattern>' statement.
func ShowLike(objectType, name string) string {
	return fmt.Sprintf("SHOW %s LIKE '%s'", objectType, EscapeLikePattern(name))
}

// ShowLikeIn builds a SHOW <objectType> LIKE '<pattern>' IN <scope> statement.
func ShowLikeIn(objectType, name, scope string) string {
	return fmt.Sprintf("SHOW %s LIKE '%s' IN %s", objectType, EscapeLikePattern(name), scope)
}

// DropIfExists builds a DROP <objectType> IF EXISTS <identifier> statement.
func DropIfExists(objectType, identifier string) string {
	return fmt.Sprintf("DROP %s IF EXISTS %s", objectType, identifier)
}

// ShowParameters builds a SHOW PARAMETERS IN <objectType> <identifier> statement.
func ShowParameters(objectType, identifier string) string {
	return fmt.Sprintf("SHOW PARAMETERS IN %s %s", objectType, identifier)
}
