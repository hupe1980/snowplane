package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreatePipeSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "MY_PIPE"),
			CopyStatement: "COPY INTO DB.SCH.T FROM @DB.SCH.S",
		}
		got, err := buildCreatePipeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE PIPE IF NOT EXISTS "DB"."SCH"."MY_PIPE"`)
		assert.Contains(t, got, "AS COPY INTO DB.SCH.T FROM @DB.SCH.S")
	})

	t.Run("WithAutoIngest", func(t *testing.T) {
		t.Parallel()
		integration := "MY_NOTIF_INT"
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement: "COPY INTO T FROM @S",
			AutoIngest:    ptr(true),
			Integration:   &integration,
		}
		got, err := buildCreatePipeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "AUTO_INGEST = TRUE")
		assert.Contains(t, got, "INTEGRATION = 'MY_NOTIF_INT'")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		integration := "INT"
		errIntegration := "ERR_INT"
		comment := "my pipe"
		opts := CreatePipeOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement:    "COPY INTO T FROM @S",
			AutoIngest:       ptr(true),
			Integration:      &integration,
			ErrorIntegration: &errIntegration,
			Comment:          &comment,
		}
		got, err := buildCreatePipeSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "AUTO_INGEST = TRUE")
		assert.Contains(t, got, "INTEGRATION = 'INT'")
		assert.Contains(t, got, "ERROR_INTEGRATION = 'ERR_INT'")
		assert.Contains(t, got, "COMMENT = 'my pipe'")
		assert.Contains(t, got, "AS COPY INTO")
	})

	t.Run("NoAutoIngest", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement: "COPY INTO T FROM @S",
			AutoIngest:    ptr(false),
		}
		got, err := buildCreatePipeSQL(opts)
		require.NoError(t, err)
		assert.NotContains(t, got, "AUTO_INGEST")
	})
}

func TestCreatePipeOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement: "COPY INTO T FROM @S",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{CopyStatement: "COPY INTO T FROM @S"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingCopyStatement", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "copy statement")
	})

	t.Run("AutoIngestWithoutIntegration", func(t *testing.T) {
		t.Parallel()
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement: "COPY INTO T FROM @S",
			AutoIngest:    ptr(true),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integration")
	})

	t.Run("AutoIngestWithIntegration", func(t *testing.T) {
		t.Parallel()
		integration := "INT"
		opts := CreatePipeOptions{
			Name:          NewSchemaObjectIdentifier("DB", "SCH", "P"),
			CopyStatement: "COPY INTO T FROM @S",
			AutoIngest:    ptr(true),
			Integration:   &integration,
		}
		require.NoError(t, opts.Validate())
	})
}

func TestBuildAlterPipeStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "P"),
		}
		stmts, err := buildAlterPipeStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		c := "updated"
		opts := AlterPipeOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "P"),
			Comment: &c,
		}
		stmts, err := buildAlterPipeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("SetErrorIntegration", func(t *testing.T) {
		t.Parallel()
		ei := "ERR_INT"
		opts := AlterPipeOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "P"),
			ErrorIntegration: &ei,
		}
		stmts, err := buildAlterPipeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ERROR_INTEGRATION = 'ERR_INT'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "P"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterPipeStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestBuildShowPipeByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowPipeByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_PIPE"))
	assert.Contains(t, got, "SHOW PIPES LIKE 'MY\\_PIPE'")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestBuildDropPipeSQL(t *testing.T) {
	t.Parallel()
	got := buildDropPipeSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_PIPE"))
	assert.Contains(t, got, `DROP PIPE IF EXISTS "DB"."SCH"."MY_PIPE"`)
}

func TestAlterPipeOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterPipeOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithErrorIntegration", func(t *testing.T) {
		t.Parallel()
		ei := "ERR"
		opts := AlterPipeOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), ErrorIntegration: &ei}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterPipeOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "P")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterPipeOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}
