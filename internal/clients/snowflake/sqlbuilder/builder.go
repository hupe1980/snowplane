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

// IsExpressionLike returns true when s appears to be a SQL expression rather
// than a plain column name.  It checks for characters that indicate function
// calls, arithmetic, or multi-value lists:  parentheses, arithmetic operators
// (+, *, /), and the comma separator.
//
// Note: hyphens (-) are NOT included because they are valid in Snowflake
// identifiers (e.g. "my-column").  The subtraction operator typically appears
// alongside parentheses or spaces (e.g. "col1 - col2"), which are caught
// by other characters.  A bare "a-b" is ambiguous but treating it as an
// identifier is the safer default — incorrect quoting is easily visible,
// whereas silently emitting an unquoted identifier as an expression
// causes hard-to-debug errors.
func IsExpressionLike(s string) bool {
	return strings.ContainsAny(s, "()+*/,")
}

// QuoteIdentifierOrExpression quotes simple column names but passes
// expression-like values through after a SQL injection denylist check.
// Use for CLUSTER BY clauses where the value may be either a column name
// (DATE_COL) or an expression (TO_DATE(col)).
//
// Returns the quoted identifier or sanitized expression. If the expression
// contains forbidden SQL patterns (semicolons, comments, dollar-quoting),
// it is treated as a plain identifier and quoted to neutralize any injection.
func QuoteIdentifierOrExpression(s string) string {
	if IsExpressionLike(s) {
		// Defense-in-depth: reject expressions containing SQL injection markers.
		// Rather than returning an error (which would change the API surface),
		// fall back to quoting, which safely neutralizes the payload.
		if identifierDenyRe.MatchString(s) {
			return QuoteIdentifier(s)
		}

		return s
	}

	return QuoteIdentifier(s)
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

// ValidateIdentifierParts validates that a dot-separated identifier string
// (e.g. "DB.SCHEMA.TABLE") is safe for use as a Snowflake fully-qualified name.
// Each part must be non-empty and must not contain SQL injection markers
// (semicolons, comments, dollar-quoting). Returns nil if valid.
func ValidateIdentifierParts(fqn string) error {
	if fqn == "" {
		return fmt.Errorf("identifier must not be empty")
	}

	parts := strings.Split(fqn, ".")
	if len(parts) > 3 {
		return fmt.Errorf("identifier has too many parts (%d), expected at most 3 (database.schema.object)", len(parts))
	}

	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return fmt.Errorf("identifier part %d is empty", i+1)
		}

		if err := validateIdentifierPart(p); err != nil {
			return fmt.Errorf("identifier part %d (%q): %w", i+1, p, err)
		}
	}

	return nil
}

// identifierDenyRe rejects characters and patterns that are never valid in
// Snowflake object identifiers but could be used for SQL injection.
var identifierDenyRe = regexp.MustCompile(`(?i:;|--\s|/\*|\*/|\$\$)`)

// validateIdentifierPart checks a single component of a multi-part identifier.
func validateIdentifierPart(part string) error {
	if identifierDenyRe.MatchString(part) {
		return fmt.Errorf("contains forbidden SQL pattern (semicolon, comment, or dollar-quoting)")
	}

	return nil
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
// (uppercase letters, digits, underscores, hyphens, and spaces only).
// Use this as a defense-in-depth check for enum-like SetKeyword values.
func ValidateKeywordValue(s string) error {
	for _, c := range s {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != ' ' && c != '-' {
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

// ValidateDollarQuotedValue validates a string that will be embedded inside
// dollar-quoting delimiters ($$...$$). Rejects values containing $$ which
// would prematurely terminate the dollar-quoted literal and allow SQL injection.
// Also rejects semicolons and comment markers as defense-in-depth.
func ValidateDollarQuotedValue(s string) error {
	if strings.Contains(s, "$$") {
		return fmt.Errorf("value must not contain dollar-quoting delimiter ($$)")
	}

	if strings.Contains(s, ";") {
		return fmt.Errorf("value must not contain semicolons")
	}

	if strings.Contains(s, "--") {
		return fmt.Errorf("value must not contain line comment markers (--)") //nolint:goconst // readability
	}

	if strings.Contains(s, "/*") || strings.Contains(s, "*/") {
		return fmt.Errorf("value must not contain block comment markers")
	}

	return nil
}

// policyBodyDenyRe rejects dangerous patterns in masking/row-access policy body
// expressions. Policy bodies are raw SQL (e.g. "CASE WHEN ... THEN ...") and
// cannot be parameterised, but we block statement-breaking patterns to prevent
// SQL injection: semicolons that would start a new statement, comment markers
// that could hide injected SQL, and dollar-quoting that could break context.
var policyBodyDenyRe = regexp.MustCompile(`[;]|--|/\*|\*/|\$\$`)

// ValidatePolicyBody validates a masking policy or row access policy body
// expression. Bodies are raw SQL expressions embedded directly into ALTER ...
// SET BODY -> <expr> and CREATE ... AS (...) -> <expr> statements. Since the
// body cannot be parameterised or quoted, we reject values containing
// statement-breaking patterns as defense-in-depth.
func ValidatePolicyBody(body string) error {
	if body == "" {
		return fmt.Errorf("policy body must not be empty")
	}

	if len(body) > 65536 {
		return fmt.Errorf("policy body too long (%d chars, max 65536)", len(body))
	}

	if policyBodyDenyRe.MatchString(body) {
		return fmt.Errorf("invalid policy body: contains forbidden pattern (semicolons, comment markers, or dollar-quoting)")
	}

	return nil
}

// stageLocationRe validates Snowflake stage reference locations.
// Valid formats: @db.schema.stage, @db.schema.stage/path/, @~, @~/path/.
// Only allows alphanumeric, underscores, dots, forward slashes, tildes, @, and hyphens.
var stageLocationRe = regexp.MustCompile(`^@[A-Za-z0-9_.~][A-Za-z0-9_./~-]*$`)

// ValidateStageLocation validates a Snowflake stage location reference.
// Stage locations use @-prefixed syntax (e.g. "@DB.SCHEMA.STAGE/path/")
// and must not contain SQL metacharacters.
func ValidateStageLocation(loc string) error {
	if loc == "" {
		return fmt.Errorf("stage location must not be empty")
	}

	if len(loc) > 1024 {
		return fmt.Errorf("stage location too long (%d chars, max 1024)", len(loc))
	}

	if !stageLocationRe.MatchString(loc) {
		return fmt.Errorf("invalid stage location %q: must start with @ and contain only alphanumeric, underscores, dots, forward slashes, tildes, and hyphens", loc)
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

	for _, c := range field {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return fmt.Errorf("invalid unset field %q: invalid character %q", field, string(c))
		}
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

// BuildKeywordListClause formats a validated keyword list like
// AUTHENTICATION_METHODS = (PASSWORD, SAML).
// Every value is validated against ValidateKeywordValue to prevent injection.
func BuildKeywordListClause(key string, vals []string) (string, error) {
	for _, v := range vals {
		if err := ValidateKeywordValue(v); err != nil {
			return "", fmt.Errorf("BuildKeywordListClause(%q): value %q: %w", key, v, err)
		}
	}

	return fmt.Sprintf("%s = (%s)", key, strings.Join(vals, ", ")), nil
}

// BuildEscapedListClause formats an escaped quoted list like
// SECURITY_INTEGRATIONS = ('INT1', 'INT2') or ALLOWED_IP_LIST = ('1.2.3.4').
// Each value is single-quoted with EscapeString applied.
func BuildEscapedListClause(key string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", EscapeString(v))
	}

	return fmt.Sprintf("%s = (%s)", key, strings.Join(quoted, ", "))
}

// SetKeywordList writes a validated keyword list like KEY = (VAL1, VAL2) to the
// builder when vals is non-empty.  Each value passes ValidateKeywordValue.
func (b *Builder) SetKeywordList(key string, vals []string) {
	if len(vals) == 0 {
		return
	}

	clause, err := BuildKeywordListClause(key, vals)
	if err != nil {
		b.err = errors.Join(b.err, err)
		return
	}

	fmt.Fprintf(&b.Builder, " %s", clause)
}

// SetEscapedList writes an escaped quoted list like KEY = ('V1', 'V2') to the
// builder when vals is non-empty.  Each value is escaped with EscapeString.
func (b *Builder) SetEscapedList(key string, vals []string) {
	if len(vals) == 0 {
		return
	}

	fmt.Fprintf(&b.Builder, " %s", BuildEscapedListClause(key, vals))
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

// KeywordList appends a keyword list clause like KEY = (VAL1, VAL2) when vals
// is non-empty.  Each value is validated with ValidateKeywordValue.
func (sc *SetClauses) KeywordList(key string, vals []string) {
	if len(vals) == 0 {
		return
	}

	clause, err := BuildKeywordListClause(key, vals)
	if err != nil {
		sc.err = errors.Join(sc.err, err)
		return
	}

	sc.clauses = append(sc.clauses, clause)
}

// EscapedList appends a quoted-escaped list clause like KEY = ('V1', 'V2')
// when vals is non-empty.  Each value is escaped with EscapeString.
func (sc *SetClauses) EscapedList(key string, vals []string) {
	if len(vals) == 0 {
		return
	}

	sc.clauses = append(sc.clauses, BuildEscapedListClause(key, vals))
}

// UnsafeRaw appends a pre-formatted clause string directly.
// Use this for clauses with non-standard syntax that don't fit the other methods.
//
// SAFETY: Callers MUST validate the input before calling UnsafeRaw().
// This method performs no escaping or validation — it trusts the caller
// to have applied appropriate validation (e.g. ValidateFileFormat,
// ValidateEncryptionType) before constructing the clause string.
func (sc *SetClauses) UnsafeRaw(clause string) {
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

// DropIfExistsCascade builds a DROP <objectType> IF EXISTS <identifier> CASCADE statement.
// This drops the object and all dependent objects (e.g., all schemas inside a database).
func DropIfExistsCascade(objectType, identifier string) string {
	return fmt.Sprintf("DROP %s IF EXISTS %s CASCADE", objectType, identifier)
}

// ShowParameters builds a SHOW PARAMETERS IN <objectType> <identifier> statement.
func ShowParameters(objectType, identifier string) string {
	return fmt.Sprintf("SHOW PARAMETERS IN %s %s", objectType, identifier)
}

// BuildCreatePreamble writes the CREATE preamble to b.
//
// When useCreateOrAlter is true the preamble is:
//
//	CREATE OR ALTER [TRANSIENT] <objectType> <fqn>
//
// Otherwise:
//
//	CREATE [TRANSIENT] <objectType> IF NOT EXISTS <fqn>
//
// This covers ~25 buildCreateXxxSQL functions that share the same pattern.
func BuildCreatePreamble(b *Builder, objectType string, fqn string, useCreateOrAlter bool, transient bool) {
	if useCreateOrAlter {
		b.WriteString("CREATE OR ALTER")
	} else {
		b.WriteString("CREATE")
	}

	if transient {
		b.WriteString(" TRANSIENT")
	}

	if useCreateOrAlter {
		_, _ = fmt.Fprintf(b, " %s %s", objectType, fqn)
	} else {
		_, _ = fmt.Fprintf(b, " %s IF NOT EXISTS %s", objectType, fqn)
	}
}
