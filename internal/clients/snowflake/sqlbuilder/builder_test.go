package sqlbuilder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------------------
// EscapeString
// ---------------------------------------------------------------------------

func TestEscapeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no special chars", "hello", "hello"},
		{"single quote", "it's", "it''s"},
		{"backslash", `a\b`, `a\\b`},
		{"both", `it\'s`, `it\\''s`},
		{"multiple quotes", "a''b", "a''''b"},
		{"multiple backslashes", `a\\b`, `a\\\\b`},
		{"nul byte stripped", "a\x00b", "ab"},
		{"nul byte only", "\x00", ""},
		{"nul with special chars", "it\x00's", "it''s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, EscapeString(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// EscapeLikePattern
// ---------------------------------------------------------------------------

func TestEscapeLikePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"no special chars", "hello", "hello"},
		{"percent", "a%b", `a\%b`},
		{"underscore", "a_b", `a\_b`},
		{"quote", "it's", "it''s"},
		{"backslash", `a\b`, `a\\b`},
		{"all special", `%\_'`, `\%\\\_''`},
		{"nul byte stripped", "a\x00b", "ab"},
		{"nul with wildcards", "a\x00%b", `a\%b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, EscapeLikePattern(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// QuoteIdentifier
// ---------------------------------------------------------------------------

func TestQuoteIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "MY_DB", `"MY_DB"`},
		{"with quote", `my"db`, `"my""db"`},
		{"empty", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, QuoteIdentifier(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// BoolToSQL
// ---------------------------------------------------------------------------

func TestBoolToSQL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "TRUE", BoolToSQL(true))
	assert.Equal(t, "FALSE", BoolToSQL(false))
}

// ---------------------------------------------------------------------------
// Builder
// ---------------------------------------------------------------------------

func TestBuilder_SetString(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE DATABASE foo")
	b.SetString("COMMENT", ptr("hello 'world'"))
	assert.Equal(t, "CREATE DATABASE foo COMMENT = 'hello ''world'''", b.String())
}

func TestBuilder_SetString_Nil(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE DATABASE foo")
	b.SetString("COMMENT", nil)
	assert.Equal(t, "CREATE DATABASE foo", b.String())
}

func TestBuilder_SetInt32(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE DATABASE foo")
	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", ptr(int32(7)))
	assert.Equal(t, "CREATE DATABASE foo DATA_RETENTION_TIME_IN_DAYS = 7", b.String())
}

func TestBuilder_SetBool(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE DATABASE foo")
	b.SetBool("REPLACE_INVALID_CHARACTERS", ptr(true))
	assert.Equal(t, "CREATE DATABASE foo REPLACE_INVALID_CHARACTERS = TRUE", b.String())
}

func TestBuilder_SetKeyword(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE WAREHOUSE wh")
	b.SetKeyword("STORAGE_SERIALIZATION_POLICY", ptr("OPTIMIZED"))
	assert.Equal(t, "CREATE WAREHOUSE wh STORAGE_SERIALIZATION_POLICY = OPTIMIZED", b.String())
}

func TestBuilder_SetQuotedKeyword(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("CREATE DATABASE foo")
	b.SetQuotedKeyword("LOG_LEVEL", ptr("INFO"))
	assert.Equal(t, "CREATE DATABASE foo LOG_LEVEL = 'INFO'", b.String())
}

func TestBuilder_Combined(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString(`CREATE DATABASE IF NOT EXISTS "MY_DB"`)
	b.SetString("COMMENT", ptr("test"))
	b.SetInt32("DATA_RETENTION_TIME_IN_DAYS", ptr(int32(7)))
	b.SetBool("REPLACE_INVALID_CHARACTERS", ptr(false))
	b.SetKeyword("METRIC_LEVEL", ptr("ALL"))
	expected := `CREATE DATABASE IF NOT EXISTS "MY_DB" COMMENT = 'test' DATA_RETENTION_TIME_IN_DAYS = 7 REPLACE_INVALID_CHARACTERS = FALSE METRIC_LEVEL = ALL`
	assert.Equal(t, expected, b.String())
}

// ---------------------------------------------------------------------------
// SetClauses
// ---------------------------------------------------------------------------

func TestSetClauses_BuildAlter(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	sc.String("COMMENT", ptr("hello"))
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", ptr(int32(7)))
	sc.Bool("REPLACE_INVALID_CHARACTERS", ptr(true))
	sc.Keyword("METRIC_LEVEL", ptr("ALL"))
	sc.QuotedKeyword("LOG_LEVEL", ptr("INFO"))

	result, err := sc.BuildAlter("DATABASE", `"MY_DB"`)
	require.NoError(t, err)
	expected := `ALTER DATABASE "MY_DB" SET COMMENT = 'hello' DATA_RETENTION_TIME_IN_DAYS = 7 REPLACE_INVALID_CHARACTERS = TRUE METRIC_LEVEL = ALL LOG_LEVEL = 'INFO'`
	assert.Equal(t, expected, result)
}

func TestSetClauses_Empty(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	assert.False(t, sc.HasClauses())
	result, err := sc.BuildAlter("DATABASE", `"MY_DB"`)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestSetClauses_NilValues(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	sc.String("COMMENT", nil)
	sc.Int32("DATA_RETENTION_TIME_IN_DAYS", nil)
	sc.Bool("X", nil)
	sc.Keyword("Y", nil)
	sc.QuotedKeyword("Z", nil)
	assert.False(t, sc.HasClauses())
}

func TestSetClauses_Raw(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	sc.Raw("DEFAULT_SECONDARY_ROLES = ('ALL')")
	assert.True(t, sc.HasClauses())
	result, err := sc.BuildAlter("USER", `"JDOE"`)
	require.NoError(t, err)
	assert.Equal(t, `ALTER USER "JDOE" SET DEFAULT_SECONDARY_ROLES = ('ALL')`, result)
}

// ---------------------------------------------------------------------------
// BuildUnset / BuildAlterStatements
// ---------------------------------------------------------------------------

func TestBuildUnset(t *testing.T) {
	t.Parallel()
	result, err := BuildUnset("DATABASE", `"MY_DB"`, []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"})
	require.NoError(t, err)
	assert.Equal(t, `ALTER DATABASE "MY_DB" UNSET COMMENT, DATA_RETENTION_TIME_IN_DAYS`, result)
}

func TestBuildUnset_Empty(t *testing.T) {
	t.Parallel()
	r1, err1 := BuildUnset("DATABASE", `"MY_DB"`, nil)
	require.NoError(t, err1)
	assert.Equal(t, "", r1)
	r2, err2 := BuildUnset("DATABASE", `"MY_DB"`, []string{})
	require.NoError(t, err2)
	assert.Equal(t, "", r2)
}

func TestBuildUnset_InvalidField(t *testing.T) {
	t.Parallel()
	_, err := BuildUnset("DATABASE", `"MY_DB"`, []string{"COMMENT; DROP TABLE"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid unset field")
}

func TestBuildAlterStatements_SetAndUnset(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	sc.String("COMMENT", ptr("new"))
	stmts, err := BuildAlterStatements("DATABASE", `"MY_DB"`, &sc, []string{"LOG_LEVEL"})
	require.NoError(t, err)
	assert.Len(t, stmts, 2)
	assert.Equal(t, `ALTER DATABASE "MY_DB" SET COMMENT = 'new'`, stmts[0])
	assert.Equal(t, `ALTER DATABASE "MY_DB" UNSET LOG_LEVEL`, stmts[1])
}

func TestBuildAlterStatements_SetOnly(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	sc.String("COMMENT", ptr("new"))
	stmts, err := BuildAlterStatements("DATABASE", `"MY_DB"`, &sc, nil)
	require.NoError(t, err)
	assert.Len(t, stmts, 1)
}

func TestBuildAlterStatements_UnsetOnly(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	stmts, err := BuildAlterStatements("DATABASE", `"MY_DB"`, &sc, []string{"COMMENT"})
	require.NoError(t, err)
	assert.Len(t, stmts, 1)
	assert.Contains(t, stmts[0], "UNSET")
}

func TestBuildAlterStatements_Empty(t *testing.T) {
	t.Parallel()
	var sc SetClauses
	stmts, err := BuildAlterStatements("DATABASE", `"MY_DB"`, &sc, nil)
	require.NoError(t, err)
	assert.Empty(t, stmts)
}

// ---------------------------------------------------------------------------
// ShowLike / ShowLikeIn / DropIfExists / ShowParameters
// ---------------------------------------------------------------------------

func TestShowLike(t *testing.T) {
	t.Parallel()
	assert.Equal(t, `SHOW DATABASES LIKE 'MY\_DB'`, ShowLike("DATABASES", "MY_DB"))
}

func TestShowLike_SpecialChars(t *testing.T) {
	t.Parallel()
	assert.Equal(t, `SHOW DATABASES LIKE 'a\%b'`, ShowLike("DATABASES", "a%b"))
}

func TestShowLikeIn(t *testing.T) {
	t.Parallel()
	result := ShowLikeIn("SCHEMAS", "MY_SCHEMA", `DATABASE "MY_DB"`)
	assert.Equal(t, `SHOW SCHEMAS LIKE 'MY\_SCHEMA' IN DATABASE "MY_DB"`, result)
}

func TestDropIfExists(t *testing.T) {
	t.Parallel()
	assert.Equal(t, `DROP DATABASE IF EXISTS "MY_DB"`, DropIfExists("DATABASE", `"MY_DB"`))
}

func TestShowParameters(t *testing.T) {
	t.Parallel()
	assert.Equal(t, `SHOW PARAMETERS IN DATABASE "MY_DB"`, ShowParameters("DATABASE", `"MY_DB"`))
}

// ---------------------------------------------------------------------------
// QuoteIdentifierParts
// ---------------------------------------------------------------------------

func TestQuoteIdentifierParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single part", "MY_DB", `"MY_DB"`},
		{"two parts", "MY_DB.PUBLIC", `"MY_DB"."PUBLIC"`},
		{"three parts", "DB.SCH.TBL", `"DB"."SCH"."TBL"`},
		{"with spaces", " MY_DB . PUBLIC ", `"MY_DB"."PUBLIC"`},
		{"embedded quotes", `MY"DB.PUB"LIC`, `"MY""DB"."PUB""LIC"`},
		{"empty parts", ".", `"".""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, QuoteIdentifierParts(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateObjectType
// ---------------------------------------------------------------------------

func TestValidateObjectType(t *testing.T) {
	t.Parallel()

	t.Run("valid types", func(t *testing.T) {
		t.Parallel()
		for _, ty := range []string{
			"DATABASE", "WAREHOUSE", "TABLE", "VIEW", "SCHEMA",
			"TABLES", "VIEWS", "STAGES", "ACCOUNT",
			"RESOURCE MONITOR", "MASKING POLICY",
		} {
			assert.NoError(t, ValidateObjectType(ty), "expected %q to be valid", ty)
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateObjectType("database"))
		assert.NoError(t, ValidateObjectType("Table"))
	})

	t.Run("invalid type", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, ValidateObjectType("INVALID_TYPE"))
		assert.Error(t, ValidateObjectType(`"; DROP`))
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, ValidateObjectType(""))
	})
}

// ---------------------------------------------------------------------------
// ValidateKeywordValue
// ---------------------------------------------------------------------------

func TestValidateKeywordValue(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateKeywordValue("XSMALL"))
		assert.NoError(t, ValidateKeywordValue("STANDARD"))
		assert.NoError(t, ValidateKeywordValue("X_LARGE_2"))
	})

	t.Run("invalid chars", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, ValidateKeywordValue(`XSMALL"; DROP`))
		assert.Error(t, ValidateKeywordValue("A'B"))
	})
}

// ---------------------------------------------------------------------------
// SetKeyword / SetQuotedKeyword defense-in-depth (L-17)
// ---------------------------------------------------------------------------

func TestSetKeyword_ErrorOnInvalidValue(t *testing.T) {
	t.Parallel()
	var b Builder
	bad := `XSMALL"; DROP`
	b.SetKeyword("WAREHOUSE_SIZE", &bad)
	require.Error(t, b.Err())
	assert.Contains(t, b.Err().Error(), "SetKeyword")
}

func TestSetKeyword_ValidValue(t *testing.T) {
	t.Parallel()
	var b Builder
	b.WriteString("ALTER WAREHOUSE")
	v := "XSMALL"
	b.SetKeyword("WAREHOUSE_SIZE", &v)
	require.NoError(t, b.Err())
	assert.Equal(t, "ALTER WAREHOUSE WAREHOUSE_SIZE = XSMALL", b.String())
}

func TestSetQuotedKeyword_ErrorOnInvalidValue(t *testing.T) {
	t.Parallel()
	var b Builder
	bad := `INFO'; DROP`
	b.SetQuotedKeyword("LOG_LEVEL", &bad)
	require.Error(t, b.Err())
	assert.Contains(t, b.Err().Error(), "SetQuotedKeyword")
}

func TestSetClauses_Keyword_ErrorOnInvalidValue(t *testing.T) {
	t.Parallel()
	sc := &SetClauses{}
	bad := `XSMALL"; DROP`
	sc.Keyword("SIZE", &bad)
	require.Error(t, sc.Err())
	assert.Contains(t, sc.Err().Error(), "SetClauses.Keyword")
}

func TestSetClauses_QuotedKeyword_ErrorOnInvalidValue(t *testing.T) {
	t.Parallel()
	sc := &SetClauses{}
	bad := `INFO'; DROP`
	sc.QuotedKeyword("LOG_LEVEL", &bad)
	require.Error(t, sc.Err())
	assert.Contains(t, sc.Err().Error(), "SetClauses.QuotedKeyword")
}

func TestSetClauses_BuildAlter_PropagatError(t *testing.T) {
	t.Parallel()
	sc := &SetClauses{}
	bad := `EVIL"; DROP`
	sc.Keyword("SIZE", &bad)
	_, err := sc.BuildAlter("WAREHOUSE", `"WH"`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SetClauses.Keyword")
}

// ---------------------------------------------------------------------------
// ValidateColumnType
// ---------------------------------------------------------------------------

func TestValidateColumnType(t *testing.T) {
	t.Parallel()

	valid := []string{"VARCHAR", "NUMBER(38,0)", "TIMESTAMP_LTZ", "ARRAY", "VARCHAR(100)", "FLOAT", "TIMESTAMP WITH TIME ZONE"}
	for _, v := range valid {
		require.NoError(t, ValidateColumnType(v), "expected valid: %s", v)
	}

	invalid := []string{"", "VARCHAR;DROP", "NUMBER(38)--", "type\nbreak"}
	for _, v := range invalid {
		require.Error(t, ValidateColumnType(v), "expected invalid: %s", v)
	}

	// Max length
	require.Error(t, ValidateColumnType(strings.Repeat("A", 257)))
}

// ---------------------------------------------------------------------------
// ValidateColumnDefault
// ---------------------------------------------------------------------------

func TestValidateColumnDefault(t *testing.T) {
	t.Parallel()

	valid := []string{"NULL", "0", "'hello'", "CURRENT_TIMESTAMP()", "SEQ.NEXTVAL", "'2024-01-01'::DATE", "TRUE", "FALSE", "-1.5", "''"}
	for _, v := range valid {
		require.NoError(t, ValidateColumnDefault(v), "expected valid: %s", v)
	}

	invalid := []struct {
		name string
		val  string
	}{
		{"empty", ""},
		{"semicolon", "1; DROP TABLE x"},
		{"line comment", "value -- comment"},
		{"block comment open", "value /* injected */"},
		{"block comment close", "start */ end"},
		{"dollar quoting", "$$malicious$$"},
		{"copy keyword", "COPY INTO @stage"},
		{"execute keyword", "EXECUTE IMMEDIATE 'DROP TABLE x'"},
		{"call keyword", "CALL system_fn()"},
		{"system dollar", "SYSTEM$TYPEOF(1)"},
		{"unbalanced quotes", "'unbalanced"},
		{"triple quote", "'a'b'"},
		{"too long", strings.Repeat("x", 1025)},
	}
	for _, tt := range invalid {
		require.Error(t, ValidateColumnDefault(tt.val), "expected invalid (%s): %s", tt.name, tt.val)
	}
}

// ---------------------------------------------------------------------------
// ValidateEncryptionType
// ---------------------------------------------------------------------------

func TestValidateEncryptionType(t *testing.T) {
	t.Parallel()

	valid := []string{"SNOWFLAKE_FULL", "SNOWFLAKE_SSE", "AWS_SSE_S3", "AWS_SSE_KMS", "AWS_CSE", "GCS_SSE_KMS", "AZURE_CSE", "NONE"}
	for _, v := range valid {
		require.NoError(t, ValidateEncryptionType(v), "expected valid: %s", v)
	}

	// Case insensitive
	require.NoError(t, ValidateEncryptionType("snowflake_full"))

	invalid := []string{"", "SNOWFLAKE_FULL'; DROP DATABASE x;--", "UNKNOWN_TYPE", "INVALID"}
	for _, v := range invalid {
		require.Error(t, ValidateEncryptionType(v), "expected invalid: %s", v)
	}
}

// ---------------------------------------------------------------------------
// ValidateFileFormat
// ---------------------------------------------------------------------------

func TestValidateFileFormat(t *testing.T) {
	t.Parallel()

	valid := []string{"FORMAT_NAME = 'MY_FORMAT'", "TYPE = CSV", "TYPE = CSV FIELD_DELIMITER = ','"}
	for _, v := range valid {
		require.NoError(t, ValidateFileFormat(v), "expected valid: %s", v)
	}

	invalid := []string{"", "TYPE = CSV; DROP TABLE x", "FORMAT_NAME = 'x' -- comment", strings.Repeat("x", 2049)}
	for _, v := range invalid {
		require.Error(t, ValidateFileFormat(v), "expected invalid: %s", v)
	}
}

// ---------------------------------------------------------------------------
// ValidateUnsetField
// ---------------------------------------------------------------------------

func TestValidateUnsetField(t *testing.T) {
	t.Parallel()

	valid := []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS", "DEFAULT_DDL_COLLATION"}
	for _, v := range valid {
		require.NoError(t, ValidateUnsetField(v), "expected valid: %s", v)
	}

	invalid := []string{"", "COMMENT; DROP", "field--inject"}
	for _, v := range invalid {
		require.Error(t, ValidateUnsetField(v), "expected invalid: %s", v)
	}
}
