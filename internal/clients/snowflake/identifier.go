package snowflake

import (
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ObjectIdentifier is the interface all typed Snowflake identifiers implement.
type ObjectIdentifier interface {
	Name() string
	FullyQualifiedName() string
	String() string
}

// quoteIdentifier wraps a name in double quotes, escaping any embedded quotes.
func quoteIdentifier(name string) string {
	return sqlbuilder.QuoteIdentifier(name)
}

// ValidObjectIdentifier returns true when the identifier is non-nil and every
// component is a non-blank string.
func ValidObjectIdentifier(id ObjectIdentifier) bool {
	if id == nil {
		return false
	}
	switch v := id.(type) {
	case AccountObjectIdentifier:
		return strings.TrimSpace(v.name) != ""
	case DatabaseObjectIdentifier:
		return strings.TrimSpace(v.databaseName) != "" && strings.TrimSpace(v.name) != ""
	case SchemaObjectIdentifier:
		return strings.TrimSpace(v.databaseName) != "" && strings.TrimSpace(v.schemaName) != "" && strings.TrimSpace(v.name) != ""
	default:
		return false
	}
}

// ParseDatabaseNameFromFQN extracts the unquoted database name from a
// fully qualified identifier string such as `"MY_DB"`.
func ParseDatabaseNameFromFQN(fqn string) string {
	// Strip surrounding quotes and unescape doubled quotes.
	name := strings.TrimPrefix(fqn, `"`)
	name = strings.TrimSuffix(name, `"`)
	name = strings.ReplaceAll(name, `""`, `"`)

	return name
}

// ParseSchemaNameFromFQN extracts the unquoted schema name from a
// fully qualified DatabaseObjectIdentifier string such as `"MY_DB"."MY_SCHEMA"`.
func ParseSchemaNameFromFQN(fqn string) string {
	const sep = `"."`
	idx := strings.LastIndex(fqn, sep)

	if idx < 0 {
		// Fallback: single-part identifier.
		return ParseDatabaseNameFromFQN(fqn)
	}

	part := fqn[idx+len(sep):]
	part = strings.TrimSuffix(part, `"`)
	part = strings.ReplaceAll(part, `""`, `"`)

	return part
}

// --- AccountObjectIdentifier ---

// AccountObjectIdentifier identifies an account-level object such as a Database
// or Warehouse.
type AccountObjectIdentifier struct {
	name string
}

// NewAccountObjectIdentifier creates a new AccountObjectIdentifier.
func NewAccountObjectIdentifier(name string) AccountObjectIdentifier {
	return AccountObjectIdentifier{name: name}
}

// Name returns the object name.
func (a AccountObjectIdentifier) Name() string { return a.name }

// FullyQualifiedName returns the quoted identifier.
func (a AccountObjectIdentifier) FullyQualifiedName() string { return quoteIdentifier(a.name) }

// String returns the fully qualified name.
func (a AccountObjectIdentifier) String() string { return a.FullyQualifiedName() }

// Equal reports whether two identifiers refer to the same object.
// Comparison is case-insensitive because Snowflake treats unquoted
// identifiers as case-insensitive (stored as uppercase internally).
func (a AccountObjectIdentifier) Equal(other AccountObjectIdentifier) bool {
	return strings.EqualFold(a.name, other.name)
}

// --- DatabaseObjectIdentifier ---

// DatabaseObjectIdentifier identifies a database-level object such as a Schema.
type DatabaseObjectIdentifier struct {
	databaseName string
	name         string
}

// NewDatabaseObjectIdentifier creates a new DatabaseObjectIdentifier.
func NewDatabaseObjectIdentifier(databaseName, name string) DatabaseObjectIdentifier {
	return DatabaseObjectIdentifier{databaseName: databaseName, name: name}
}

// Name returns the object name.
func (d DatabaseObjectIdentifier) Name() string { return d.name }

// DatabaseName returns the parent database name.
func (d DatabaseObjectIdentifier) DatabaseName() string { return d.databaseName }

// FullyQualifiedName returns the quoted "database"."object" identifier.
func (d DatabaseObjectIdentifier) FullyQualifiedName() string {
	return quoteIdentifier(d.databaseName) + "." + quoteIdentifier(d.name)
}

// String returns the fully qualified name.
func (d DatabaseObjectIdentifier) String() string { return d.FullyQualifiedName() }

// Equal reports whether two identifiers refer to the same object.
// Comparison is case-insensitive because Snowflake treats unquoted
// identifiers as case-insensitive (stored as uppercase internally).
func (d DatabaseObjectIdentifier) Equal(other DatabaseObjectIdentifier) bool {
	return strings.EqualFold(d.databaseName, other.databaseName) && strings.EqualFold(d.name, other.name)
}

// --- SchemaObjectIdentifier ---

// SchemaObjectIdentifier identifies a schema-level object such as a Table or View.
type SchemaObjectIdentifier struct {
	databaseName string
	schemaName   string
	name         string
}

// NewSchemaObjectIdentifier creates a new SchemaObjectIdentifier.
func NewSchemaObjectIdentifier(databaseName, schemaName, name string) SchemaObjectIdentifier {
	return SchemaObjectIdentifier{databaseName: databaseName, schemaName: schemaName, name: name}
}

// Name returns the object name.
func (s SchemaObjectIdentifier) Name() string { return s.name }

// SchemaName returns the parent schema name.
func (s SchemaObjectIdentifier) SchemaName() string { return s.schemaName }

// DatabaseName returns the parent database name.
func (s SchemaObjectIdentifier) DatabaseName() string { return s.databaseName }

// FullyQualifiedName returns the quoted "database"."schema"."object" identifier.
func (s SchemaObjectIdentifier) FullyQualifiedName() string {
	return quoteIdentifier(s.databaseName) + "." + quoteIdentifier(s.schemaName) + "." + quoteIdentifier(s.name)
}

// String returns the fully qualified name.
func (s SchemaObjectIdentifier) String() string { return s.FullyQualifiedName() }

// Equal reports whether two identifiers refer to the same object.
// Comparison is case-insensitive because Snowflake treats unquoted
// identifiers as case-insensitive (stored as uppercase internally).
func (s SchemaObjectIdentifier) Equal(other SchemaObjectIdentifier) bool {
	return strings.EqualFold(s.databaseName, other.databaseName) && strings.EqualFold(s.schemaName, other.schemaName) && strings.EqualFold(s.name, other.name)
}
