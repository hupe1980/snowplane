package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

const testInvalidEnum = "INVALID"

// --------------------------------------------------------------------------
// Tests: buildCreateSchemaSQL
// --------------------------------------------------------------------------

func TestBuildCreateSchemaSQL_Minimal(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSchemaSQL(CreateSchemaOptions{
		Name: NewDatabaseObjectIdentifier("ANALYTICS", "PUBLIC"),
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE SCHEMA IF NOT EXISTS "ANALYTICS"."PUBLIC"`, sql)
}

func TestBuildCreateSchemaSQL_Transient(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSchemaSQL(CreateSchemaOptions{
		Name:      NewDatabaseObjectIdentifier("ANALYTICS", "STAGING"),
		Transient: true,
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE TRANSIENT SCHEMA IF NOT EXISTS "ANALYTICS"."STAGING"`, sql)
}

func TestBuildCreateSchemaSQL_ManagedAccess(t *testing.T) {
	t.Parallel()

	sql, err := buildCreateSchemaSQL(CreateSchemaOptions{
		Name:          NewDatabaseObjectIdentifier("ANALYTICS", "SECURE"),
		ManagedAccess: true,
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE SCHEMA IF NOT EXISTS "ANALYTICS"."SECURE" WITH MANAGED ACCESS`, sql)
}

func TestBuildCreateSchemaSQL_AllOptions(t *testing.T) {
	t.Parallel()

	logLevel := "INFO"
	metricLevel := "ALL"
	traceLevel := "ON_EVENT"
	ssp := "OPTIMIZED"

	sql, err := buildCreateSchemaSQL(CreateSchemaOptions{
		Name:                       NewDatabaseObjectIdentifier("DB", "FULL_SCHEMA"),
		Transient:                  true,
		ManagedAccess:              true,
		Comment:                    ptr("test schema"),
		DataRetentionTimeInDays:    ptr(int32(7)),
		MaxDataExtensionTimeInDays: ptr(int32(14)),
		ReplaceInvalidCharacters:   ptr(true),
		DefaultDDLCollation:        ptr("en-ci"),
		StorageSerializationPolicy: &ssp,
		LogLevel:                   &logLevel,
		MetricLevel:                &metricLevel,
		TraceLevel:                 &traceLevel,
	})
	require.NoError(t, err)

	expected := `CREATE TRANSIENT SCHEMA IF NOT EXISTS "DB"."FULL_SCHEMA" WITH MANAGED ACCESS` +
		` COMMENT = 'test schema'` +
		` DATA_RETENTION_TIME_IN_DAYS = 7` +
		` MAX_DATA_EXTENSION_TIME_IN_DAYS = 14` +
		` REPLACE_INVALID_CHARACTERS = TRUE` +
		` DEFAULT_DDL_COLLATION = 'en-ci'` +
		` STORAGE_SERIALIZATION_POLICY = OPTIMIZED` +
		` LOG_LEVEL = 'INFO'` +
		` METRIC_LEVEL = ALL` +
		` TRACE_LEVEL = ON_EVENT`

	assert.Equal(t, expected, sql)
}

// --------------------------------------------------------------------------
// Tests: buildDropSchemaSQL
// --------------------------------------------------------------------------

func TestBuildDropSchemaSQL(t *testing.T) {
	t.Parallel()

	sql := buildDropSchemaSQL(NewDatabaseObjectIdentifier("DB", "SCHEMA"))
	assert.Equal(t, `DROP SCHEMA IF EXISTS "DB"."SCHEMA"`, sql)
}

// --------------------------------------------------------------------------
// Tests: buildShowSchemaByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowSchemaByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowSchemaByIDSQL(NewDatabaseObjectIdentifier("MY_DB", "MY_SCHEMA"))
	assert.Equal(t, `SHOW SCHEMAS LIKE 'MY\_SCHEMA' IN DATABASE "MY_DB"`, sql)
}

// --------------------------------------------------------------------------
// Tests: buildShowSchemaParametersSQL
// --------------------------------------------------------------------------

func TestBuildShowSchemaParametersSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowSchemaParametersSQL(NewDatabaseObjectIdentifier("MY_DB", "MY_SCHEMA"))
	assert.Equal(t, `SHOW PARAMETERS IN SCHEMA "MY_DB"."MY_SCHEMA"`, sql)
}

// --------------------------------------------------------------------------
// Tests: HasChanges
// --------------------------------------------------------------------------

func TestAlterSchemaOptions_HasChanges(t *testing.T) {
	t.Parallel()

	opts := AlterSchemaOptions{Name: NewDatabaseObjectIdentifier("DB", "S")}
	assert.False(t, opts.HasChanges())

	opts.Comment = ptr("x")
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: Validate
// --------------------------------------------------------------------------

func TestCreateSchemaOptions_Validate(t *testing.T) {
	t.Parallel()

	opts := CreateSchemaOptions{Name: NewDatabaseObjectIdentifier("DB", "S")}
	assert.NoError(t, opts.Validate())

	opts = CreateSchemaOptions{} // invalid: empty name
	assert.Error(t, opts.Validate())
}

func TestAlterSchemaOptions_Validate(t *testing.T) {
	t.Parallel()

	opts := AlterSchemaOptions{Name: NewDatabaseObjectIdentifier("DB", "S")}
	assert.NoError(t, opts.Validate())

	opts = AlterSchemaOptions{} // invalid: empty name
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: UNSET support (C-2)
// --------------------------------------------------------------------------

func TestAlterSchemaOptions_HasChanges_UnsetFields(t *testing.T) {
	t.Parallel()

	opts := AlterSchemaOptions{
		Name:        NewDatabaseObjectIdentifier("DB", "S"),
		UnsetFields: []string{"COMMENT"},
	}
	assert.True(t, opts.HasChanges())
}

func TestBuildAlterSchemaStatements_OnlyUnset(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:        NewDatabaseObjectIdentifier("DB", "S"),
		UnsetFields: []string{"COMMENT", "DATA_RETENTION_TIME_IN_DAYS"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" UNSET COMMENT, DATA_RETENTION_TIME_IN_DAYS`, stmts[0])
}

func TestBuildAlterSchemaStatements_SetAndUnset(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:        NewDatabaseObjectIdentifier("DB", "S"),
		Comment:     ptr("new"),
		UnsetFields: []string{"LOG_LEVEL"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 2)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" SET COMMENT = 'new'`, stmts[0])
	assert.Equal(t, `ALTER SCHEMA "DB"."S" UNSET LOG_LEVEL`, stmts[1])
}

func TestBuildAlterSchemaStatements_ManagedAccessAndUnset(t *testing.T) {
	t.Parallel()

	ma := true
	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:             NewDatabaseObjectIdentifier("DB", "S"),
		SetManagedAccess: &ma,
		UnsetFields:      []string{"COMMENT"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 2)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" ENABLE MANAGED ACCESS`, stmts[0])
	assert.Equal(t, `ALTER SCHEMA "DB"."S" UNSET COMMENT`, stmts[1])
}

func TestBuildAlterSchemaStatements_AllThreeKinds(t *testing.T) {
	t.Parallel()

	ma := false
	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:             NewDatabaseObjectIdentifier("DB", "S"),
		SetManagedAccess: &ma,
		Comment:          ptr("x"),
		UnsetFields:      []string{"LOG_LEVEL"},
	})
	require.NoError(t, err)

	require.Len(t, stmts, 3)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" DISABLE MANAGED ACCESS`, stmts[0])
	assert.Equal(t, `ALTER SCHEMA "DB"."S" SET COMMENT = 'x'`, stmts[1])
	assert.Equal(t, `ALTER SCHEMA "DB"."S" UNSET LOG_LEVEL`, stmts[2])
}

func TestBuildAlterSchemaStatements_NoChanges(t *testing.T) {
	t.Parallel()

	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name: NewDatabaseObjectIdentifier("DB", "S"),
	})
	require.NoError(t, err)

	assert.Empty(t, stmts)
}

// --------------------------------------------------------------------------
// Tests: buildAlterSchemaStatements — managed access
// --------------------------------------------------------------------------

func TestBuildAlterSchemaStatements_ManagedAccessEnable(t *testing.T) {
	t.Parallel()

	ma := true
	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:             NewDatabaseObjectIdentifier("DB", "S"),
		SetManagedAccess: &ma,
	})
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" ENABLE MANAGED ACCESS`, stmts[0])
}

func TestBuildAlterSchemaStatements_ManagedAccessDisable(t *testing.T) {
	t.Parallel()

	ma := false
	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:             NewDatabaseObjectIdentifier("DB", "S"),
		SetManagedAccess: &ma,
	})
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" DISABLE MANAGED ACCESS`, stmts[0])
}

func TestBuildAlterSchemaStatements_ManagedAccessAndSet(t *testing.T) {
	t.Parallel()

	ma := true
	stmts, err := buildAlterSchemaStatements(AlterSchemaOptions{
		Name:             NewDatabaseObjectIdentifier("DB", "S"),
		SetManagedAccess: &ma,
		Comment:          ptr("x"),
	})
	require.NoError(t, err)
	require.Len(t, stmts, 2)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" ENABLE MANAGED ACCESS`, stmts[0])
	assert.Equal(t, `ALTER SCHEMA "DB"."S" SET COMMENT = 'x'`, stmts[1])
}

func TestCreateSchemaOptions_Validate_MaxDataExtension(t *testing.T) {
	t.Parallel()

	v := int32(100)
	opts := CreateSchemaOptions{
		Name:                       NewDatabaseObjectIdentifier("DB", "S"),
		MaxDataExtensionTimeInDays: &v,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}

func TestAlterSchemaOptions_Validate_MaxDataExtension(t *testing.T) {
	t.Parallel()

	v := int32(-1)
	opts := AlterSchemaOptions{
		Name:                       NewDatabaseObjectIdentifier("DB", "S"),
		MaxDataExtensionTimeInDays: &v,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxDataExtensionTimeInDays")
}

// --------------------------------------------------------------------------
// Tests: IsManagedAccess
// --------------------------------------------------------------------------

func TestIsManagedAccess_True(t *testing.T) {
	t.Parallel()

	o := v1alpha1.SchemaShowOutput{Options: "MANAGED ACCESS"}
	assert.True(t, IsManagedAccess(&o))
}

func TestIsManagedAccess_False(t *testing.T) {
	t.Parallel()

	o := v1alpha1.SchemaShowOutput{Options: ""}
	assert.False(t, IsManagedAccess(&o))
}

func TestIsManagedAccess_Substring(t *testing.T) {
	t.Parallel()

	o := v1alpha1.SchemaShowOutput{Options: "TRANSIENT, MANAGED ACCESS"}
	assert.True(t, IsManagedAccess(&o))
}

// --------------------------------------------------------------------------
// Tests: HasChanges for all individual fields (C7)
// --------------------------------------------------------------------------

func TestAlterSchemaOptions_HasChanges_AllFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		set  func(o *AlterSchemaOptions)
	}{
		{"Comment", func(o *AlterSchemaOptions) { o.Comment = ptr("c") }},
		{"DataRetention", func(o *AlterSchemaOptions) { o.DataRetentionTimeInDays = ptr(int32(1)) }},
		{"MaxDataExtension", func(o *AlterSchemaOptions) { o.MaxDataExtensionTimeInDays = ptr(int32(1)) }},
		{"DefaultDDLCollation", func(o *AlterSchemaOptions) { o.DefaultDDLCollation = ptr("utf8") }},
		{"ReplaceInvalidChars", func(o *AlterSchemaOptions) { o.ReplaceInvalidCharacters = ptr(true) }},
		{"StoragePolicy", func(o *AlterSchemaOptions) { o.StorageSerializationPolicy = ptr("OPTIMIZED") }},
		{"LogLevel", func(o *AlterSchemaOptions) { o.LogLevel = ptr("INFO") }},
		{"MetricLevel", func(o *AlterSchemaOptions) { o.MetricLevel = ptr("ALL") }},
		{"TraceLevel", func(o *AlterSchemaOptions) { o.TraceLevel = ptr("OFF") }},
		{"ManagedAccess", func(o *AlterSchemaOptions) { o.SetManagedAccess = ptr(true) }},
		{"UnsetFields", func(o *AlterSchemaOptions) { o.UnsetFields = []string{"COMMENT"} }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := AlterSchemaOptions{Name: NewDatabaseObjectIdentifier("DB", "S")}
			assert.False(t, o.HasChanges())
			tc.set(&o)
			assert.True(t, o.HasChanges())
		})
	}
}

// --------------------------------------------------------------------------
// Tests: CreateSchemaOptions.Validate with multiple errors (C8)
// --------------------------------------------------------------------------

func TestCreateSchemaOptions_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	bad := testInvalidEnum
	opts := CreateSchemaOptions{
		Name:                       NewDatabaseObjectIdentifier("DB", "S"),
		StorageSerializationPolicy: &bad,
		LogLevel:                   &bad,
		MetricLevel:                &bad,
		TraceLevel:                 &bad,
	}
	err := opts.Validate()
	require.Error(t, err)
	errStr := err.Error()
	assert.Contains(t, errStr, "storageSerializationPolicy")
	assert.Contains(t, errStr, "logLevel")
	assert.Contains(t, errStr, "metricLevel")
	assert.Contains(t, errStr, "traceLevel")
}

// --------------------------------------------------------------------------
// Tests: AlterSchemaOptions.Validate with multiple invalid enums (C6)
// --------------------------------------------------------------------------

func TestAlterSchemaOptions_Validate_InvalidEnums(t *testing.T) {
	t.Parallel()

	bad := testInvalidEnum
	opts := AlterSchemaOptions{
		Name:                       NewDatabaseObjectIdentifier("DB", "S"),
		StorageSerializationPolicy: &bad,
		LogLevel:                   &bad,
		MetricLevel:                &bad,
		TraceLevel:                 &bad,
	}
	err := opts.Validate()
	require.Error(t, err)
	errStr := err.Error()
	assert.Contains(t, errStr, "storageSerializationPolicy")
	assert.Contains(t, errStr, "logLevel")
	assert.Contains(t, errStr, "metricLevel")
	assert.Contains(t, errStr, "traceLevel")
}

// --------------------------------------------------------------------------
// Tests: buildAlterSchemaStatements only SET (C9)
// --------------------------------------------------------------------------

func TestBuildAlterSchemaStatements_OnlySet(t *testing.T) {
	t.Parallel()

	opts := AlterSchemaOptions{
		Name:    NewDatabaseObjectIdentifier("DB", "S"),
		Comment: ptr("hello"),
	}
	stmts, err := buildAlterSchemaStatements(opts)
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, `ALTER SCHEMA "DB"."S" SET COMMENT = 'hello'`, stmts[0])
}
