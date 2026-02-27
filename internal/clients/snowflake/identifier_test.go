package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAccountObjectIdentifier(t *testing.T) {
	t.Parallel()
	id := NewAccountObjectIdentifier("MY_DB")
	assert.Equal(t, "MY_DB", id.Name())
	assert.Equal(t, `"MY_DB"`, id.FullyQualifiedName())
}

func TestAccountObjectIdentifier_Equal(t *testing.T) {
	t.Parallel()
	a := NewAccountObjectIdentifier("ANALYTICS")
	b := NewAccountObjectIdentifier("ANALYTICS")
	c := NewAccountObjectIdentifier("OTHER")
	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))
}

func TestAccountObjectIdentifier_EqualCaseInsensitive(t *testing.T) {
	t.Parallel()
	a := NewAccountObjectIdentifier("ANALYTICS")
	b := NewAccountObjectIdentifier("analytics")
	c := NewAccountObjectIdentifier("Analytics")
	assert.True(t, a.Equal(b))
	assert.True(t, a.Equal(c))
}

func TestAccountObjectIdentifier_CasePreservation(t *testing.T) {
	t.Parallel()
	id := NewAccountObjectIdentifier("MixedCase")
	assert.Equal(t, `"MixedCase"`, id.FullyQualifiedName())
}

func TestNewDatabaseObjectIdentifier(t *testing.T) {
	t.Parallel()
	id := NewDatabaseObjectIdentifier("MY_DB", "MY_SCHEMA")
	assert.Equal(t, "MY_SCHEMA", id.Name())
	assert.Equal(t, "MY_DB", id.DatabaseName())
	assert.Equal(t, `"MY_DB"."MY_SCHEMA"`, id.FullyQualifiedName())
}

func TestDatabaseObjectIdentifier_Equal(t *testing.T) {
	t.Parallel()
	a := NewDatabaseObjectIdentifier("DB", "ROLE_A")
	b := NewDatabaseObjectIdentifier("DB", "ROLE_A")
	c := NewDatabaseObjectIdentifier("DB", "ROLE_B")
	d := NewDatabaseObjectIdentifier("OTHER_DB", "ROLE_A")
	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))
	assert.False(t, a.Equal(d))
}

func TestDatabaseObjectIdentifier_EqualCaseInsensitive(t *testing.T) {
	t.Parallel()
	a := NewDatabaseObjectIdentifier("DB", "ROLE_A")
	b := NewDatabaseObjectIdentifier("db", "role_a")
	assert.True(t, a.Equal(b))
}

func TestNewSchemaObjectIdentifier(t *testing.T) {
	t.Parallel()
	id := NewSchemaObjectIdentifier("DB", "SCHEMA", "TABLE")
	assert.Equal(t, "TABLE", id.Name())
	assert.Equal(t, "SCHEMA", id.SchemaName())
	assert.Equal(t, "DB", id.DatabaseName())
	assert.Equal(t, `"DB"."SCHEMA"."TABLE"`, id.FullyQualifiedName())
}

func TestSchemaObjectIdentifier_Equal(t *testing.T) {
	t.Parallel()
	a := NewSchemaObjectIdentifier("DB", "SCH", "TBL")
	b := NewSchemaObjectIdentifier("DB", "SCH", "TBL")
	c := NewSchemaObjectIdentifier("DB", "SCH", "OTHER")
	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))
}

func TestSchemaObjectIdentifier_EqualCaseInsensitive(t *testing.T) {
	t.Parallel()
	a := NewSchemaObjectIdentifier("DB", "SCH", "TBL")
	b := NewSchemaObjectIdentifier("db", "sch", "tbl")
	assert.True(t, a.Equal(b))
}

func TestValidObjectIdentifier_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   ObjectIdentifier
	}{
		{"account", NewAccountObjectIdentifier("DB")},
		{"database", NewDatabaseObjectIdentifier("DB", "SCHEMA")},
		{"schema", NewSchemaObjectIdentifier("DB", "SCHEMA", "TABLE")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, ValidObjectIdentifier(tt.id))
		})
	}
}

func TestValidObjectIdentifier_EmptyName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   ObjectIdentifier
	}{
		{"account_empty", NewAccountObjectIdentifier("")},
		{"database_empty_name", NewDatabaseObjectIdentifier("DB", "")},
		{"database_empty_db", NewDatabaseObjectIdentifier("", "SCHEMA")},
		{"schema_empty_name", NewSchemaObjectIdentifier("DB", "SCHEMA", "")},
		{"schema_empty_schema", NewSchemaObjectIdentifier("DB", "", "TABLE")},
		{"schema_empty_db", NewSchemaObjectIdentifier("", "SCHEMA", "TABLE")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.False(t, ValidObjectIdentifier(tt.id))
		})
	}
}

func TestValidObjectIdentifier_WhitespaceName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   ObjectIdentifier
	}{
		{"account_spaces", NewAccountObjectIdentifier("   ")},
		{"account_tab", NewAccountObjectIdentifier("\t")},
		{"database_spaces", NewDatabaseObjectIdentifier("DB", "   ")},
		{"schema_spaces", NewSchemaObjectIdentifier("DB", "SCHEMA", "   ")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.False(t, ValidObjectIdentifier(tt.id))
		})
	}
}

func TestValidObjectIdentifier_Nil(t *testing.T) {
	t.Parallel()
	require.False(t, ValidObjectIdentifier(nil))
}

func TestAccountObjectIdentifier_String(t *testing.T) {
	t.Parallel()
	id := NewAccountObjectIdentifier("WAREHOUSE_XS")
	assert.Equal(t, `"WAREHOUSE_XS"`, id.String())
}

func TestDatabaseObjectIdentifier_String(t *testing.T) {
	t.Parallel()
	id := NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC")
	assert.Equal(t, `"ANALYTICS"."PUBLIC"`, id.String())
}

func TestSchemaObjectIdentifier_String(t *testing.T) {
	t.Parallel()
	id := NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "EVENTS")
	assert.Equal(t, `"ANALYTICS"."PUBLIC"."EVENTS"`, id.String())
}

func TestQuoteIdentifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", `"simple"`},
		{`has"quote`, `"has""quote"`},
		{"", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, quoteIdentifier(tt.input))
		})
	}
}

func TestParseDatabaseNameFromFQN(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fqn      string
		expected string
	}{
		{"quoted simple", `"MY_DB"`, "MY_DB"},
		{"unquoted", "MY_DB", "MY_DB"},
		{"quoted with escaped quotes", `"MY""DB"`, `MY"DB`},
		{"empty string", "", ""},
		{"only quotes", `""`, ""},
		{"mixed case", `"Analytics"`, "Analytics"},
		{"no surrounding quotes", "PLAIN_NAME", "PLAIN_NAME"},
		{"single leading quote", `"LEADING`, "LEADING"},
		{"single trailing quote", `TRAILING"`, "TRAILING"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, ParseDatabaseNameFromFQN(tt.fqn))
		})
	}
}
