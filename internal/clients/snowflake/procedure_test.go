package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildArgClause(t *testing.T) {
	t.Parallel()

	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()
		got := buildArgClause(nil)
		assert.Equal(t, "()", got)
	})

	t.Run("SingleArg", func(t *testing.T) {
		t.Parallel()
		got := buildArgClause([]ProcedureArgument{
			{Name: "x", Type: "VARCHAR"},
		})
		assert.Equal(t, `("x" VARCHAR)`, got)
	})

	t.Run("MultipleArgs", func(t *testing.T) {
		t.Parallel()
		got := buildArgClause([]ProcedureArgument{
			{Name: "name", Type: "VARCHAR"},
			{Name: "count", Type: "NUMBER"},
		})
		assert.Equal(t, `("name" VARCHAR, "count" NUMBER)`, got)
	})
}

func TestBuildArgSignature(t *testing.T) {
	t.Parallel()

	t.Run("NoArgs", func(t *testing.T) {
		t.Parallel()
		got := buildArgSignature(nil)
		assert.Equal(t, "()", got)
	})

	t.Run("SingleArg", func(t *testing.T) {
		t.Parallel()
		got := buildArgSignature([]string{"VARCHAR"})
		assert.Equal(t, "(VARCHAR)", got)
	})

	t.Run("MultipleArgs", func(t *testing.T) {
		t.Parallel()
		got := buildArgSignature([]string{"VARCHAR", "NUMBER", "BOOLEAN"})
		assert.Equal(t, "(VARCHAR, NUMBER, BOOLEAN)", got)
	})
}

func TestBuildCreateProcedureSQL(t *testing.T) {
	t.Parallel()

	t.Run("MinimalSQL", func(t *testing.T) {
		t.Parallel()
		body := "BEGIN RETURN 1; END"
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "MY_PROC"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Equal(t,
			`CREATE PROCEDURE IF NOT EXISTS "DB"."SCH"."MY_PROC"() RETURNS NUMBER LANGUAGE SQL AS $$BEGIN RETURN 1; END$$`,
			got)
	})

	t.Run("CreateOrAlter", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		opts := CreateProcedureOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:          "NUMBER",
			Language:         "SQL",
			Body:             &body,
			UseCreateOrAlter: true,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "CREATE OR ALTER PROCEDURE")
		assert.NotContains(t, got, "IF NOT EXISTS")
	})

	t.Run("WithArguments", func(t *testing.T) {
		t.Parallel()
		body := "RETURN x"
		opts := CreateProcedureOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Arguments: []ProcedureArgument{
				{Name: "x", Type: "VARCHAR"},
				{Name: "y", Type: "NUMBER"},
			},
			Returns:  "VARCHAR",
			Language: "JAVASCRIPT",
			Body:     &body,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `("x" VARCHAR, "y" NUMBER)`)
		assert.Contains(t, got, "RETURNS VARCHAR")
	})

	t.Run("PythonWithPackagesAndHandler", func(t *testing.T) {
		t.Parallel()
		body := "import pandas\nreturn df"
		handler := "main"
		runtime := "3.10"
		opts := CreateProcedureOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "PY_PROC"),
			Arguments: []ProcedureArgument{
				{Name: "input", Type: "VARCHAR"},
			},
			Returns:        "TABLE(col1 VARCHAR, col2 NUMBER)",
			Language:       "PYTHON",
			RuntimeVersion: &runtime,
			Handler:        &handler,
			Packages:       []string{"pandas==1.5.3", "snowflake-snowpark-python"},
			Body:           &body,
		}
		got, err := buildCreateProcedureSQL(opts)
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
		targetPath := "@~/my_proc.jar"
		opts := CreateProcedureOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "JAVA_PROC"),
			Arguments: []ProcedureArgument{
				{Name: "input", Type: "VARCHAR"},
			},
			Returns:        "VARCHAR",
			Language:       "JAVA",
			RuntimeVersion: &runtime,
			Handler:        &handler,
			TargetPath:     &targetPath,
			Imports:        []string{"@~/lib1.jar", "@~/lib2.jar"},
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "LANGUAGE JAVA")
		assert.Contains(t, got, "RUNTIME_VERSION = '17'")
		assert.Contains(t, got, "IMPORTS = ('@~/lib1.jar', '@~/lib2.jar')")
		assert.Contains(t, got, "HANDLER = 'MyClass.myMethod'")
		assert.Contains(t, got, "TARGET_PATH = '@~/my_proc.jar'")
		assert.NotContains(t, got, "AS $$") // No body for compiled languages
	})

	t.Run("WithExternalAccessAndSecrets", func(t *testing.T) {
		t.Parallel()
		body := "fetch(url)"
		opts := CreateProcedureOptions{
			Name:                       NewSchemaObjectIdentifier("DB", "SCH", "EXT_PROC"),
			Returns:                    "VARCHAR",
			Language:                   "PYTHON",
			Body:                       &body,
			ExternalAccessIntegrations: []string{"MY_EAI"},
			Secrets: map[string]string{
				"api_key": `"DB"."SCH"."MY_SECRET"`,
			},
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "EXTERNAL_ACCESS_INTEGRATIONS = (MY_EAI)")
		assert.Contains(t, got, "SECRETS = ('api_key' = \"DB\".\"SCH\".\"MY_SECRET\")")
	})

	t.Run("WithExecuteAs", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		execAs := "CALLER"
		opts := CreateProcedureOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:   "NUMBER",
			Language:  "SQL",
			Body:      &body,
			ExecuteAs: &execAs,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "EXECUTE AS CALLER")
	})

	t.Run("WithNullInputBehavior", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		nullBeh := "RETURNS NULL ON NULL INPUT"
		opts := CreateProcedureOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:           "NUMBER",
			Language:          "SQL",
			Body:              &body,
			NullInputBehavior: &nullBeh,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "RETURNS NULL ON NULL INPUT")
	})

	t.Run("Secure", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Secure:   true,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		// SECURE must come before PROCEDURE in Snowflake syntax
		assert.Contains(t, got, "SECURE PROCEDURE")
		assert.NotContains(t, got, "LANGUAGE SQL SECURE")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		comment := "my procedure"
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Comment:  &comment,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'my procedure'")
	})

	t.Run("CommentWithSingleQuote", func(t *testing.T) {
		t.Parallel()
		body := "RETURN 1"
		comment := "it's a proc"
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
			Comment:  &comment,
		}
		got, err := buildCreateProcedureSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'it''s a proc'")
	})
}

func TestBuildAlterProcedureStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes: []string{"VARCHAR"},
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("ExecuteAsCaller_NoSET", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes:  []string{"VARCHAR", "NUMBER"},
			ExecuteAs: ptr("CALLER"),
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		// Must NOT contain "SET EXECUTE AS" — Snowflake syntax is "EXECUTE AS" without SET
		assert.Equal(t, `ALTER PROCEDURE "DB"."SCH"."P"(VARCHAR, NUMBER) EXECUTE AS CALLER`, stmts[0])
		assert.NotContains(t, stmts[0], "SET EXECUTE")
	})

	t.Run("ExecuteAsOwner", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes:  []string{},
			ExecuteAs: ptr("OWNER"),
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER PROCEDURE "DB"."SCH"."P"() EXECUTE AS OWNER`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes: []string{},
			Comment:  ptr("updated comment"),
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes:    []string{"NUMBER"},
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("ExecuteAsAndComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ArgTypes:  []string{},
			ExecuteAs: ptr("CALLER"),
			Comment:   ptr("test"),
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 2) // Separate: EXECUTE AS + SET COMMENT
	})

	t.Run("ArgSignatureInFQN", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "OVERLOADED"),
			ArgTypes: []string{"VARCHAR", "NUMBER", "BOOLEAN"},
			Comment:  ptr("test"),
		}
		stmts, err := buildAlterProcedureStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `"DB"."SCH"."OVERLOADED"(VARCHAR, NUMBER, BOOLEAN)`)
	})
}

func TestBuildShowProcedureByIDSQL(t *testing.T) {
	t.Parallel()

	name := NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_PROC")
	got := buildShowProcedureByIDSQL(name)
	assert.Contains(t, got, "SHOW PROCEDURES")
	assert.Contains(t, got, `IN SCHEMA "MY_DB"."MY_SCHEMA"`)
}

func TestCreateProcedureOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:  "NUMBER",
			Language: "SQL",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateProcedureOptions{
			Returns:  "NUMBER",
			Language: "SQL",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingReturns", func(t *testing.T) {
		t.Parallel()
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Language: "SQL",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "return type is required")
	})

	t.Run("MissingLanguage", func(t *testing.T) {
		t.Parallel()
		opts := CreateProcedureOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns: "NUMBER",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "language is required")
	})

	t.Run("InvalidBody", func(t *testing.T) {
		t.Parallel()
		body := "contains $$ in body"
		opts := CreateProcedureOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Returns:  "NUMBER",
			Language: "SQL",
			Body:     &body,
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "body")
	})
}

func TestAlterProcedureOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("HasComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: ptr("c"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasExecuteAs", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ExecuteAs: ptr("CALLER"),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterProcedureOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
