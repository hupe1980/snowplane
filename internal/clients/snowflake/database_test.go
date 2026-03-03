package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

func ptr[T any](v T) *T { return &v }

func TestCreateDatabaseOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("MY_DB"),
		Comment:                    ptr("test db"),
		DataRetentionTimeInDays:    ptr(int32(30)),
		StorageSerializationPolicy: ptr("COMPATIBLE"),
		LogLevel:                   ptr("INFO"),
		MetricLevel:                ptr("ALL"),
		TraceLevel:                 ptr("ON_EVENT"),
	}
	require.NoError(t, opts.Validate())
}

func TestCreateDatabaseOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name: NewAccountObjectIdentifier(""),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database name is required")
}

func TestCreateDatabaseOptions_Validate_RetentionOutOfRange(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:                    NewAccountObjectIdentifier("DB"),
		DataRetentionTimeInDays: ptr(int32(100)),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataRetentionTimeInDays must be 0–90")
}

func TestCreateDatabaseOptions_Validate_NegativeRetention(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:                    NewAccountObjectIdentifier("DB"),
		DataRetentionTimeInDays: ptr(int32(-1)),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataRetentionTimeInDays must be 0–90")
}

func TestCreateDatabaseOptions_Validate_InvalidStoragePolicy(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("DB"),
		StorageSerializationPolicy: ptr("INVALID"),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storageSerializationPolicy must be COMPATIBLE or OPTIMIZED")
}

func TestCreateDatabaseOptions_Validate_InvalidLogLevel(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:     NewAccountObjectIdentifier("DB"),
		LogLevel: ptr("VERBOSE"),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "logLevel must be one of")
}

func TestCreateDatabaseOptions_Validate_InvalidMetricLevel(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:        NewAccountObjectIdentifier("DB"),
		MetricLevel: ptr("SOME"),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metricLevel must be NONE or ALL")
}

func TestCreateDatabaseOptions_Validate_InvalidTraceLevel(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:       NewAccountObjectIdentifier("DB"),
		TraceLevel: ptr("MAYBE"),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "traceLevel must be ALWAYS, ON_EVENT, or OFF")
}

func TestCreateDatabaseOptions_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	opts := CreateDatabaseOptions{
		Name:                    NewAccountObjectIdentifier(""),
		DataRetentionTimeInDays: ptr(int32(200)),
		LogLevel:                ptr("BAD"),
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database name is required")
	assert.Contains(t, err.Error(), "dataRetentionTimeInDays")
	assert.Contains(t, err.Error(), "logLevel")
}

func TestAlterDatabaseOptions_HasChanges(t *testing.T) {
	t.Parallel()

	opts := AlterDatabaseOptions{
		Name: NewAccountObjectIdentifier("DB"),
	}
	assert.False(t, opts.HasChanges())

	opts.Comment = ptr("new comment")
	assert.True(t, opts.HasChanges())
}

func TestAlterDatabaseOptions_HasChanges_AllFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		set  func(o *AlterDatabaseOptions)
	}{
		{"Comment", func(o *AlterDatabaseOptions) { o.Comment = ptr("c") }},
		{"DataRetention", func(o *AlterDatabaseOptions) { o.DataRetentionTimeInDays = ptr(int32(1)) }},
		{"MaxDataExtension", func(o *AlterDatabaseOptions) { o.MaxDataExtensionTimeInDays = ptr(int32(1)) }},
		{"Catalog", func(o *AlterDatabaseOptions) { o.Catalog = ptr("c") }},
		{"ExternalVolume", func(o *AlterDatabaseOptions) { o.ExternalVolume = ptr("v") }},
		{"ReplaceInvalidChars", func(o *AlterDatabaseOptions) { o.ReplaceInvalidCharacters = ptr(true) }},
		{"DefaultDDLCollation", func(o *AlterDatabaseOptions) { o.DefaultDDLCollation = ptr("utf8") }},
		{"StoragePolicy", func(o *AlterDatabaseOptions) { o.StorageSerializationPolicy = ptr("OPTIMIZED") }},
		{"LogLevel", func(o *AlterDatabaseOptions) { o.LogLevel = ptr("INFO") }},
		{"MetricLevel", func(o *AlterDatabaseOptions) { o.MetricLevel = ptr("ALL") }},
		{"TraceLevel", func(o *AlterDatabaseOptions) { o.TraceLevel = ptr("OFF") }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := AlterDatabaseOptions{Name: NewAccountObjectIdentifier("DB")}
			assert.False(t, o.HasChanges())
			tc.set(&o)
			assert.True(t, o.HasChanges())
		})
	}
}

func TestEscapeString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello", sqlbuilder.EscapeString("hello"))
	assert.Equal(t, "it''s", sqlbuilder.EscapeString("it's"))
	assert.Equal(t, "a''b''c", sqlbuilder.EscapeString("a'b'c"))
	assert.Equal(t, `back\\slash`, sqlbuilder.EscapeString(`back\slash`))
	assert.Equal(t, `\\`, sqlbuilder.EscapeString(`\`), "lone backslash must be escaped")
	assert.Equal(t, `\\''`, sqlbuilder.EscapeString(`\'`), "backslash-quote combo")
}

func TestAlterDatabaseOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	retention := int32(7)
	o := AlterDatabaseOptions{
		Name:                    NewAccountObjectIdentifier("DB"),
		DataRetentionTimeInDays: &retention,
	}
	assert.NoError(t, o.Validate())
}

func TestAlterDatabaseOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	o := AlterDatabaseOptions{
		Name: NewAccountObjectIdentifier(""),
	}
	err := o.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database name is required")
}

func TestAlterDatabaseOptions_Validate_RetentionOutOfRange(t *testing.T) {
	t.Parallel()

	retention := int32(100)
	o := AlterDatabaseOptions{
		Name:                    NewAccountObjectIdentifier("DB"),
		DataRetentionTimeInDays: &retention,
	}
	err := o.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataRetentionTimeInDays must be 0–90")
}

func TestAlterDatabaseOptions_Validate_InvalidEnums(t *testing.T) {
	t.Parallel()

	bad := "INVALID"
	o := AlterDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("DB"),
		StorageSerializationPolicy: &bad,
		LogLevel:                   &bad,
		MetricLevel:                &bad,
		TraceLevel:                 &bad,
	}
	err := o.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storageSerializationPolicy")
	assert.Contains(t, err.Error(), "logLevel")
	assert.Contains(t, err.Error(), "metricLevel")
	assert.Contains(t, err.Error(), "traceLevel")
}

func TestParseInt32(t *testing.T) {
	t.Parallel()

	v, ok := parseInt32("42")
	assert.True(t, ok)
	assert.Equal(t, int32(42), v)

	v, ok = parseInt32("0")
	assert.True(t, ok)
	assert.Equal(t, int32(0), v)

	_, ok = parseInt32("abc")
	assert.False(t, ok)

	_, ok = parseInt32("")
	assert.False(t, ok)

	// Trailing garbage must be rejected (B7 fix).
	_, ok = parseInt32("42abc")
	assert.False(t, ok)

	// Negative values
	v, ok = parseInt32("-5")
	assert.True(t, ok)
	assert.Equal(t, int32(-5), v)
}

func TestDatabaseObservation_NotFound(t *testing.T) {
	t.Parallel()

	obs := &DatabaseObservation{Exists: false}
	assert.False(t, obs.Exists)
	assert.Nil(t, obs.ShowOutput)
	assert.Nil(t, obs.Parameters)
}

// ---------------------------------------------------------------------------
// SQL builder tests (T1 HIGH)
// ---------------------------------------------------------------------------

func TestBuildCreateSQL_MinimalOptions(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name: NewAccountObjectIdentifier("MY_DB"),
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE DATABASE IF NOT EXISTS "MY_DB"`, sql)
}

func TestBuildCreateSQL_Transient(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name:      NewAccountObjectIdentifier("TMP"),
		Transient: true,
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE TRANSIENT DATABASE IF NOT EXISTS "TMP"`, sql)
}

func TestBuildCreateSQL_AllOptions(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("FULL_DB"),
		Comment:                    ptr("my comment"),
		DataRetentionTimeInDays:    ptr(int32(7)),
		MaxDataExtensionTimeInDays: ptr(int32(14)),
		Transient:                  true,
		Catalog:                    ptr("my_catalog"),
		ExternalVolume:             ptr("my_volume"),
		ReplaceInvalidCharacters:   ptr(true),
		DefaultDDLCollation:        ptr("en-ci"),
		StorageSerializationPolicy: ptr("OPTIMIZED"),
		LogLevel:                   ptr("INFO"),
		MetricLevel:                ptr("ALL"),
		TraceLevel:                 ptr("ON_EVENT"),
	})
	require.NoError(t, err)

	expected := `CREATE TRANSIENT DATABASE IF NOT EXISTS "FULL_DB"` +
		` COMMENT = 'my comment'` +
		` DATA_RETENTION_TIME_IN_DAYS = 7` +
		` MAX_DATA_EXTENSION_TIME_IN_DAYS = 14` +
		` CATALOG = 'my_catalog'` +
		` EXTERNAL_VOLUME = 'my_volume'` +
		` REPLACE_INVALID_CHARACTERS = TRUE` +
		` DEFAULT_DDL_COLLATION = 'en-ci'` +
		` STORAGE_SERIALIZATION_POLICY = OPTIMIZED` +
		` LOG_LEVEL = 'INFO'` +
		` METRIC_LEVEL = ALL` +
		` TRACE_LEVEL = ON_EVENT`
	assert.Equal(t, expected, sql)
}

func TestBuildCreateSQL_SpecialCharactersInComment(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name:    NewAccountObjectIdentifier("DB"),
		Comment: ptr("it's a 'test'"),
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE DATABASE IF NOT EXISTS "DB" COMMENT = 'it''s a ''test'''`, sql)
}

func TestBuildCreateSQL_CreateOrAlter(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name:             NewAccountObjectIdentifier("MY_DB"),
		Comment:          ptr("managed"),
		UseCreateOrAlter: true,
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE OR ALTER DATABASE "MY_DB" COMMENT = 'managed'`, sql)
}

func TestBuildCreateSQL_CreateOrAlter_Transient(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSQL(CreateDatabaseOptions{
		Name:             NewAccountObjectIdentifier("TMP"),
		Transient:        true,
		UseCreateOrAlter: true,
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE OR ALTER TRANSIENT DATABASE "TMP"`, sql)
}

func TestBuildAlterStatements_NoChanges(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name: NewAccountObjectIdentifier("DB"),
	})
	require.NoError(t, err)
	assert.Empty(t, stmts)
}

func TestBuildAlterStatements_SingleField(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:    NewAccountObjectIdentifier("DB"),
		Comment: ptr("updated"),
	})
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER DATABASE "DB" SET COMMENT = 'updated'`, stmts[0])
}

func TestBuildAlterStatements_MultipleFields(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:                    NewAccountObjectIdentifier("MY_DB"),
		DataRetentionTimeInDays: ptr(int32(30)),
		LogLevel:                ptr("WARN"),
	})
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER DATABASE "MY_DB" SET DATA_RETENTION_TIME_IN_DAYS = 30 LOG_LEVEL = 'WARN'`, stmts[0])
}

func TestBuildAlterStatements_AllFields(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("X"),
		Comment:                    ptr("c"),
		DataRetentionTimeInDays:    ptr(int32(1)),
		MaxDataExtensionTimeInDays: ptr(int32(2)),
		Catalog:                    ptr("cat"),
		ExternalVolume:             ptr("vol"),
		ReplaceInvalidCharacters:   ptr(false),
		DefaultDDLCollation:        ptr("utf8"),
		StorageSerializationPolicy: ptr("COMPATIBLE"),
		LogLevel:                   ptr("DEBUG"),
		MetricLevel:                ptr("NONE"),
		TraceLevel:                 ptr("OFF"),
	})
	require.NoError(t, err)

	expected := `ALTER DATABASE "X" SET` +
		` COMMENT = 'c'` +
		` DATA_RETENTION_TIME_IN_DAYS = 1` +
		` MAX_DATA_EXTENSION_TIME_IN_DAYS = 2` +
		` CATALOG = 'cat'` +
		` EXTERNAL_VOLUME = 'vol'` +
		` REPLACE_INVALID_CHARACTERS = FALSE` +
		` DEFAULT_DDL_COLLATION = 'utf8'` +
		` STORAGE_SERIALIZATION_POLICY = COMPATIBLE` +
		` LOG_LEVEL = 'DEBUG'` +
		` METRIC_LEVEL = NONE` +
		` TRACE_LEVEL = OFF`
	require.Len(t, stmts, 1)
	assert.Equal(t, expected, stmts[0])
}

func TestBuildDropSQL(t *testing.T) {
	t.Parallel()

	sql := buildDropSQL(NewAccountObjectIdentifier("OLD_DB"))
	assert.Equal(t, `DROP DATABASE IF EXISTS "OLD_DB"`, sql)
}

func TestBuildShowByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowByIDSQL(NewAccountObjectIdentifier("MYDB"))
	assert.Equal(t, `SHOW DATABASES LIKE 'MYDB'`, sql)
}

func TestBuildShowByIDSQL_UnderscoreEscaping(t *testing.T) {
	t.Parallel()

	// Underscores in identifiers must be escaped in LIKE patterns.
	sql := buildShowByIDSQL(NewAccountObjectIdentifier("MY_DB"))
	assert.Equal(t, `SHOW DATABASES LIKE 'MY\_DB'`, sql)
}

func TestBuildShowByIDSQL_WildcardEscaping(t *testing.T) {
	t.Parallel()

	// Database name with LIKE wildcards must be escaped (B6 fix).
	sql := buildShowByIDSQL(NewAccountObjectIdentifier("DB_100%"))
	assert.Equal(t, `SHOW DATABASES LIKE 'DB\_100\%'`, sql)
}

func TestBuildShowParametersSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowParametersSQL(NewAccountObjectIdentifier("MY_DB"))
	assert.Equal(t, `SHOW PARAMETERS IN DATABASE "MY_DB"`, sql)
}

func TestEscapeLikePattern(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "hello", sqlbuilder.EscapeLikePattern("hello"))
	assert.Equal(t, `it''s`, sqlbuilder.EscapeLikePattern("it's"))
	assert.Equal(t, `100\%`, sqlbuilder.EscapeLikePattern("100%"))
	assert.Equal(t, `DB\_NAME`, sqlbuilder.EscapeLikePattern("DB_NAME"))
	assert.Equal(t, `a\_b\%c''d`, sqlbuilder.EscapeLikePattern("a_b%c'd"))
}

func TestBoolToSQL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "TRUE", sqlbuilder.BoolToSQL(true))
	assert.Equal(t, "FALSE", sqlbuilder.BoolToSQL(false))
}

// ---------------------------------------------------------------------------
// UNSET support tests (C-2)
// ---------------------------------------------------------------------------

func TestAlterDatabaseOptions_HasChanges_UnsetFields(t *testing.T) {
	t.Parallel()

	opts := AlterDatabaseOptions{
		Name:        NewAccountObjectIdentifier("DB"),
		UnsetFields: []string{"COMMENT"},
	}
	assert.True(t, opts.HasChanges())
}

func TestBuildAlterStatements_OnlyUnset(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:        NewAccountObjectIdentifier("DB"),
		UnsetFields: []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER DATABASE "DB" UNSET COMMENT, DATA_RETENTION_TIME_IN_DAYS`, stmts[0])
}

func TestBuildAlterStatements_SetAndUnset(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:        NewAccountObjectIdentifier("DB"),
		Comment:     ptr("new"),
		UnsetFields: []string{"LOG_LEVEL"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 2)
	assert.Equal(t, `ALTER DATABASE "DB" SET COMMENT = 'new'`, stmts[0])
	assert.Equal(t, `ALTER DATABASE "DB" UNSET LOG_LEVEL`, stmts[1])
}

func TestBuildAlterStatements_OnlySet(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:    NewAccountObjectIdentifier("DB"),
		Comment: ptr("hello"),
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER DATABASE "DB" SET COMMENT = 'hello'`, stmts[0])
}

func TestBuildAlterStatements_UnsetFieldsGenerateUnsetClause(t *testing.T) {
	t.Parallel()

	// When only UnsetFields are set, buildAlterStatements should produce
	// an UNSET statement only (no SET clause).
	stmts, err := buildAlterStatements(AlterDatabaseOptions{
		Name:        NewAccountObjectIdentifier("DB"),
		UnsetFields: []string{"COMMENT"},
	})
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER DATABASE "DB" UNSET COMMENT`, stmts[0])
}

func TestEscapeLikePattern_Backslash(t *testing.T) {
	t.Parallel()

	// Backslash is the LIKE escape character and must be escaped first.
	result := sqlbuilder.EscapeLikePattern(`my\db`)
	assert.Equal(t, `my\\db`, result)
}

func TestEscapeLikePattern_BackslashBeforeWildcard(t *testing.T) {
	t.Parallel()

	// Ensure backslash is escaped BEFORE wildcard escaping to avoid double-escaping.
	result := sqlbuilder.EscapeLikePattern(`my\%db`)
	assert.Equal(t, `my\\\%db`, result)
}

func TestEscapeLikePattern_AllSpecialChars(t *testing.T) {
	t.Parallel()

	result := sqlbuilder.EscapeLikePattern(`a'b%c_d\e`)
	assert.Equal(t, `a''b\%c\_d\\e`, result)
}

func TestValidateMaxDataExtension_Valid(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateMaxDataExtension(nil))
	v := int32(0)
	assert.NoError(t, validateMaxDataExtension(&v))
	v = 90
	assert.NoError(t, validateMaxDataExtension(&v))
}

func TestValidateMaxDataExtension_OutOfRange(t *testing.T) {
	t.Parallel()

	v := int32(91)
	err := validateMaxDataExtension(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}

func TestValidateMaxDataExtension_Negative(t *testing.T) {
	t.Parallel()

	v := int32(-1)
	err := validateMaxDataExtension(&v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}

func TestCreateDatabaseOptions_Validate_MaxDataExtension(t *testing.T) {
	t.Parallel()

	v := int32(100)
	opts := CreateDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("DB"),
		MaxDataExtensionTimeInDays: &v,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}

func TestAlterDatabaseOptions_Validate_MaxDataExtension(t *testing.T) {
	t.Parallel()

	v := int32(100)
	opts := AlterDatabaseOptions{
		Name:                       NewAccountObjectIdentifier("DB"),
		MaxDataExtensionTimeInDays: &v,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}
