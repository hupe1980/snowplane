package snowflake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// --------------------------------------------------------------------------
// Tests: CreateExternalFunctionOptions.Validate
// --------------------------------------------------------------------------

func TestCreateExternalFunctionOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "https://example.com/api",
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateExternalFunctionOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", ""),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "https://example.com/api",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateExternalFunctionOptions_Validate_EmptyReturnType(t *testing.T) {
	t.Parallel()

	opts := CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "",
		APIIntegration: "MY_API",
		URL:            "https://example.com/api",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateExternalFunctionOptions_Validate_EmptyAPIIntegration(t *testing.T) {
	t.Parallel()

	opts := CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT",
		APIIntegration: "",
		URL:            "https://example.com/api",
	}
	assert.Error(t, opts.Validate())
}

func TestCreateExternalFunctionOptions_Validate_EmptyURL(t *testing.T) {
	t.Parallel()

	opts := CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "",
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterExternalFunctionOptions.Validate
// --------------------------------------------------------------------------

func TestAlterExternalFunctionOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := AlterExternalFunctionOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
	}
	assert.NoError(t, opts.Validate())
}

func TestAlterExternalFunctionOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := AlterExternalFunctionOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: AlterExternalFunctionOptions.HasChanges
// --------------------------------------------------------------------------

func TestAlterExternalFunctionOptions_HasChanges_None(t *testing.T) {
	t.Parallel()

	opts := AlterExternalFunctionOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "F")}
	assert.False(t, opts.HasChanges())
}

func TestAlterExternalFunctionOptions_HasChanges_Comment(t *testing.T) {
	t.Parallel()

	c := "hello"
	opts := AlterExternalFunctionOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "F"), Comment: &c}
	assert.True(t, opts.HasChanges())
}

func TestAlterExternalFunctionOptions_HasChanges_UnsetFields(t *testing.T) {
	t.Parallel()

	opts := AlterExternalFunctionOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "F"), UnsetFields: []string{"COMMENT"}}
	assert.True(t, opts.HasChanges())
}

// --------------------------------------------------------------------------
// Tests: buildShowExternalFunctionByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowExternalFunctionByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowExternalFunctionByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"))
	assert.Contains(t, sql, "SHOW EXTERNAL FUNCTIONS")
	assert.Contains(t, sql, "LIKE")
}

// --------------------------------------------------------------------------
// Tests: ExternalFunctionClient (validation only, no DB)
// --------------------------------------------------------------------------

func TestExternalFunctionClient_Create_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", ""),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "https://example.com",
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestExternalFunctionClient_Create_InvalidArgType(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "https://example.com",
		Args: []v1alpha1.ExternalFunctionArg{
			{Name: "x", Type: "VARCHAR); DROP TABLE x; --"},
		},
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestExternalFunctionClient_Create_InvalidReturnType(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT); DROP TABLE x; --",
		APIIntegration: "MY_API",
		URL:            "https://example.com",
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestExternalFunctionClient_Drop_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	err := client.Drop(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""), nil)
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestExternalFunctionClient_ShowByID_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	_, err := client.ShowByID(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestExternalFunctionClient_Alter_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewExternalFunctionClient(nil)
	err := client.Alter(t.Context(), AlterExternalFunctionOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: Create SQL generation
// --------------------------------------------------------------------------

func TestExternalFunctionClient_Create_BasicSQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	client := NewExternalFunctionClient(mock)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:     "VARIANT",
		APIIntegration: "MY_API",
		URL:            "https://example.com/api",
	})
	require.NoError(t, err)

	assert.Contains(t, captured, `CREATE EXTERNAL FUNCTION "DB"."SCH"."MY_FUNC"(`)
	assert.Contains(t, captured, "RETURNS VARIANT")
	assert.Contains(t, captured, `API_INTEGRATION = "MY_API"`)
	assert.Contains(t, captured, "AS 'https://example.com/api'")
}

func TestExternalFunctionClient_Create_FullSQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	maxBatch := int32(100)
	compression := v1alpha1.ExternalFunctionCompression("GZIP")
	reqTranslator := "db.sch.req_translator"
	respTranslator := "db.sch.resp_translator"
	comment := "my ext func"
	returnBehavior := "IMMUTABLE"

	client := NewExternalFunctionClient(mock)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:             NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:       "VARIANT",
		ReturnNullValues: ptr(true),
		ReturnBehavior:   &returnBehavior,
		APIIntegration:   "MY_API",
		URL:              "https://example.com/api",
		Args: []v1alpha1.ExternalFunctionArg{
			{Name: "input", Type: "VARCHAR"},
			{Name: "count", Type: "NUMBER"},
		},
		Headers: []v1alpha1.ExternalFunctionHeader{
			{Name: "x-custom", Value: "myvalue"},
		},
		MaxBatchRows:       &maxBatch,
		Compression:        &compression,
		RequestTranslator:  &reqTranslator,
		ResponseTranslator: &respTranslator,
		Comment:            &comment,
	})
	require.NoError(t, err)

	assert.Contains(t, captured, `"input" VARCHAR, "count" NUMBER`)
	assert.Contains(t, captured, "RETURNS VARIANT")
	assert.Contains(t, captured, "RETURNS NULL ON NULL INPUT")
	assert.Contains(t, captured, "IMMUTABLE")
	assert.Contains(t, captured, `API_INTEGRATION = "MY_API"`)
	assert.Contains(t, captured, "HEADERS = ('x-custom' = 'myvalue')")
	assert.Contains(t, captured, "MAX_BATCH_ROWS = 100")
	assert.Contains(t, captured, "COMPRESSION = 'GZIP'")
	assert.Contains(t, captured, "REQUEST_TRANSLATOR = 'db.sch.req_translator'")
	assert.Contains(t, captured, "RESPONSE_TRANSLATOR = 'db.sch.resp_translator'")
	assert.Contains(t, captured, "COMMENT = 'my ext func'")
}

func TestExternalFunctionClient_Create_NotNullReturn(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	client := NewExternalFunctionClient(mock)
	err := client.Create(t.Context(), CreateExternalFunctionOptions{
		Name:             NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ReturnType:       "VARIANT",
		ReturnNullValues: ptr(false),
		APIIntegration:   "MY_API",
		URL:              "https://example.com/api",
	})
	require.NoError(t, err)

	assert.Contains(t, captured, "NOT NULL")
	assert.NotContains(t, captured, "RETURNS NULL ON NULL INPUT")
}

// --------------------------------------------------------------------------
// Tests: Alter SQL generation
// --------------------------------------------------------------------------

func TestExternalFunctionClient_Alter_SetComment(t *testing.T) {
	t.Parallel()

	var captured []string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = append(captured, sql)
			return nil
		},
	}

	client := NewExternalFunctionClient(mock)
	err := client.Alter(t.Context(), AlterExternalFunctionOptions{
		Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
		ArgTypes: []string{"VARCHAR"},
		Comment:  ptr("updated"),
	})
	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Contains(t, captured[0], `ALTER FUNCTION "DB"."SCH"."MY_FUNC"(VARCHAR) SET COMMENT`)
	assert.Contains(t, captured[0], "'updated'")
}

func TestExternalFunctionClient_Drop_SQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	client := NewExternalFunctionClient(mock)
	err := client.Drop(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"), []string{"VARCHAR", "NUMBER"})
	require.NoError(t, err)
	assert.Contains(t, captured, `DROP FUNCTION IF EXISTS "DB"."SCH"."MY_FUNC"(VARCHAR, NUMBER)`)
}
