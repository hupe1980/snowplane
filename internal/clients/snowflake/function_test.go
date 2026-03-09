package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildFuncArgClause(t *testing.T) {
	t.Parallel()

	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()
		got := buildFuncArgClause(nil)
		assert.Equal(t, "()", got)
	})

	t.Run("SingleArg", func(t *testing.T) {
		t.Parallel()
		got := buildFuncArgClause([]FunctionArgument{
			{Name: "x", Type: "VARCHAR"},
		})
		assert.Equal(t, `("x" VARCHAR)`, got)
	})

	t.Run("MultipleArgs", func(t *testing.T) {
		t.Parallel()
		got := buildFuncArgClause([]FunctionArgument{
			{Name: "name", Type: "VARCHAR"},
			{Name: "age", Type: "NUMBER"},
		})
		assert.Equal(t, `("name" VARCHAR, "age" NUMBER)`, got)
	})
}

func TestBuildCreateFunctionSQL(t *testing.T) {
	t.Parallel()

	t.Run("MinimalSQL", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
			Returns:  "NUMBER",
			Language: "JAVASCRIPT",
			Body:     &body,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Equal(t,
			`CREATE FUNCTION IF NOT EXISTS "DB"."SCH"."MY_FUNC"() RETURNS NUMBER LANGUAGE JAVASCRIPT AS $$return 1$$`,
			got)
	})

	t.Run("CreateOrAlter", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		opts := CreateFunctionOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "MY_FUNC"),
			Returns:          "NUMBER",
			Language:         "JAVASCRIPT",
			Body:             &body,
			UseCreateOrAlter: true,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "CREATE OR ALTER FUNCTION")
		assert.NotContains(t, got, "IF NOT EXISTS")
	})

	t.Run("WithArguments", func(t *testing.T) {
		t.Parallel()
		body := "return x + y"
		opts := CreateFunctionOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "ADD_NUMBERS"),
			Arguments: []FunctionArgument{
				{Name: "x", Type: "NUMBER"},
				{Name: "y", Type: "NUMBER"},
			},
			Returns:  "NUMBER",
			Language: "JAVASCRIPT",
			Body:     &body,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `("x" NUMBER, "y" NUMBER)`)
		assert.Contains(t, got, "RETURNS NUMBER")
	})

	t.Run("PythonWithPackages", func(t *testing.T) {
		t.Parallel()
		body := "import pandas\nreturn pandas.__version__"
		handler := "main"
		runtime := "3.10"
		opts := CreateFunctionOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "PY_FUNC"),
			Arguments: []FunctionArgument{
				{Name: "input", Type: "VARCHAR"},
			},
			Returns:        "VARCHAR",
			Language:       "PYTHON",
			RuntimeVersion: &runtime,
			Handler:        &handler,
			Packages:       []string{"pandas==1.5.3", "snowflake-snowpark-python"},
			Body:           &body,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "LANGUAGE PYTHON")
		assert.Contains(t, got, "RUNTIME_VERSION = '3.10'")
		assert.Contains(t, got, "PACKAGES = ('pandas==1.5.3', 'snowflake-snowpark-python')")
		assert.Contains(t, got, "HANDLER = 'main'")
		assert.Contains(t, got, "AS $$")
	})

	t.Run("JavaWithImportsAndTargetPath", func(t *testing.T) {
		t.Parallel()
		handler := "MyClass.myMethod"
		runtime := "17"
		targetPath := "@~/my_func.jar"
		opts := CreateFunctionOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "JAVA_FUNC"),
			Arguments: []FunctionArgument{
				{Name: "input", Type: "VARCHAR"},
			},
			Returns:        "VARCHAR",
			Language:       "JAVA",
			RuntimeVersion: &runtime,
			Handler:        &handler,
			TargetPath:     &targetPath,
			Imports:        []string{"@~/lib1.jar", "@~/lib2.jar"},
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "LANGUAGE JAVA")
		assert.Contains(t, got, "RUNTIME_VERSION = '17'")
		assert.Contains(t, got, "IMPORTS = ('@~/lib1.jar', '@~/lib2.jar')")
		assert.Contains(t, got, "HANDLER = 'MyClass.myMethod'")
		assert.Contains(t, got, "TARGET_PATH = '@~/my_func.jar'")
		assert.NotContains(t, got, "AS $$") // No body for compiled languages
	})

	t.Run("WithExternalAccessAndSecrets", func(t *testing.T) {
		t.Parallel()
		body := "fetch(url)"
		opts := CreateFunctionOptions{
			Name:                       NewSchemaObjectIdentifier("DB", "SCH", "EXT_FUNC"),
			Returns:                    "VARCHAR",
			Language:                   "PYTHON",
			Body:                       &body,
			ExternalAccessIntegrations: []string{"MY_EAI"},
			Secrets: map[string]string{
				"api_key": `"DB"."SCH"."MY_SECRET"`,
			},
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "EXTERNAL_ACCESS_INTEGRATIONS = (MY_EAI)")
		assert.Contains(t, got, "SECRETS = ('api_key' = \"DB\".\"SCH\".\"MY_SECRET\")")
	})

	t.Run("WithNullInputBehaviorAndVolatility", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		nullBeh := "RETURNS NULL ON NULL INPUT"
		vol := "IMMUTABLE"
		opts := CreateFunctionOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:           "NUMBER",
			Language:          "SQL",
			Body:              &body,
			NullInputBehavior: &nullBeh,
			Volatility:        &vol,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "RETURNS NULL ON NULL INPUT")
		assert.Contains(t, got, "IMMUTABLE")
	})

	t.Run("Secure", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Secure:   true,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		// SECURE must come before FUNCTION in Snowflake syntax
		assert.Contains(t, got, "SECURE FUNCTION")
		assert.NotContains(t, got, "LANGUAGE SQL SECURE")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		comment := "my function"
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Comment:  &comment,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'my function'")
	})

	t.Run("CommentWithSingleQuote", func(t *testing.T) {
		t.Parallel()
		body := "return 1"
		comment := "it's a function"
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Comment:  &comment,
		}
		got, err := buildCreateFunctionSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'it''s a function'")
	})
}

func TestBuildAlterFunctionStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes: []string{"VARCHAR"},
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetSecure", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes: []string{"VARCHAR", "NUMBER"},
			Secure:   ptr(true),
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER FUNCTION "DB"."SCH"."F"(VARCHAR, NUMBER) SET SECURE`, stmts[0])
	})

	t.Run("UnsetSecure", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes: []string{"VARCHAR"},
			Secure:   ptr(false),
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER FUNCTION "DB"."SCH"."F"(VARCHAR) UNSET SECURE`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes: []string{},
			Comment:  ptr("updated comment"),
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes:    []string{"NUMBER"},
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("SecureAndComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			ArgTypes: []string{},
			Secure:   ptr(true),
			Comment:  ptr("secure func"),
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2) // Separate: SET SECURE + SET COMMENT
	})

	t.Run("ArgSignatureInFQN", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "OVERLOADED"),
			ArgTypes: []string{"VARCHAR", "NUMBER", "BOOLEAN"},
			Comment:  ptr("test"),
		}
		stmts, err := buildAlterFunctionStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `"DB"."SCH"."OVERLOADED"(VARCHAR, NUMBER, BOOLEAN)`)
	})
}

func TestBuildShowFunctionByIDSQL(t *testing.T) {
	t.Parallel()

	name := NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_FUNC")
	got := buildShowFunctionByIDSQL(name)
	assert.Contains(t, got, "SHOW USER FUNCTIONS")
	assert.Contains(t, got, `IN SCHEMA "MY_DB"."MY_SCHEMA"`)
}

func TestMatchProcedureArgTypes(t *testing.T) {
	t.Parallel()

	t.Run("ExactMatch", func(t *testing.T) {
		t.Parallel()
		assert.True(t, matchProcedureArgTypes("MY_PROC(VARCHAR, NUMBER) RETURN VARCHAR", "MY_PROC", []string{"VARCHAR", "NUMBER"}))
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()
		assert.True(t, matchProcedureArgTypes("my_proc(varchar, number) RETURN VARCHAR", "MY_PROC", []string{"VARCHAR", "NUMBER"}))
	})

	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()
		assert.True(t, matchProcedureArgTypes("MY_PROC() RETURN VARCHAR", "MY_PROC", []string{}))
	})

	t.Run("Mismatch", func(t *testing.T) {
		t.Parallel()
		assert.False(t, matchProcedureArgTypes("MY_PROC(VARCHAR, NUMBER) RETURN VARCHAR", "MY_PROC", []string{"VARCHAR"}))
	})

	t.Run("DifferentTypes", func(t *testing.T) {
		t.Parallel()
		assert.False(t, matchProcedureArgTypes("MY_PROC(VARCHAR) RETURN VARCHAR", "MY_PROC", []string{"NUMBER"}))
	})

	t.Run("NameNotFound", func(t *testing.T) {
		t.Parallel()
		assert.False(t, matchProcedureArgTypes("OTHER_PROC(VARCHAR) RETURN VARCHAR", "MY_PROC", []string{"VARCHAR"}))
	})

	t.Run("NoParens", func(t *testing.T) {
		t.Parallel()
		assert.True(t, matchProcedureArgTypes("MY_PROC RETURN VARCHAR", "MY_PROC", []string{}))
	})
}

func TestCreateFunctionOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:  "NUMBER",
			Language: "SQL",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateFunctionOptions{
			Returns:  "NUMBER",
			Language: "SQL",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingReturns", func(t *testing.T) {
		t.Parallel()
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Language: "SQL",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "return type is required")
	})

	t.Run("MissingLanguage", func(t *testing.T) {
		t.Parallel()
		opts := CreateFunctionOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns: "NUMBER",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "language is required")
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()
		body := "contains $$ in body"
		opts := CreateFunctionOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "body")
	})
}

func TestAlterFunctionOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "F")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("HasComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Comment: ptr("c"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasSecure", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "F"),
			Secure: ptr(true),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterFunctionOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "F"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
