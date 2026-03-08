package snowflake

import (
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
