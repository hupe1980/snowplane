package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateStreamSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicTableStream", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "MY_STREAM"),
			SourceType: StreamSourceTable,
			SourceName: `"DB"."SCH"."MY_TABLE"`,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."MY_STREAM" ON TABLE "DB"."SCH"."MY_TABLE"`, got)
	})

	t.Run("ExternalTableStream", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceExternalTable,
			SourceName: `"DB"."SCH"."EXT_TBL"`,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON EXTERNAL TABLE "DB"."SCH"."EXT_TBL"`, got)
	})

	t.Run("DynamicTableStream", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceDynamicTable,
			SourceName: `"DB"."SCH"."DYN_TBL"`,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON DYNAMIC TABLE "DB"."SCH"."DYN_TBL"`, got)
	})

	t.Run("ViewStream", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceView,
			SourceName: `"DB"."SCH"."MY_VIEW"`,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON VIEW "DB"."SCH"."MY_VIEW"`, got)
	})

	t.Run("StageStream", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceStage,
			SourceName: `"DB"."SCH"."MY_STAGE"`,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON STAGE "DB"."SCH"."MY_STAGE"`, got)
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		comment := "my stream"
		opts := CreateStreamOptions{
			Name:            NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType:      StreamSourceTable,
			SourceName:      `"DB"."SCH"."T"`,
			AppendOnly:      ptr(true),
			ShowInitialRows: ptr(true),
			Comment:         &comment,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		// COMMENT must come AFTER ON <source> and mode options per Snowflake syntax.
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON TABLE "DB"."SCH"."T" APPEND_ONLY = TRUE SHOW_INITIAL_ROWS = TRUE COMMENT = 'my stream'`, got)
	})

	t.Run("InsertOnly", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceExternalTable,
			SourceName: `"DB"."SCH"."E"`,
			InsertOnly: ptr(true),
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON EXTERNAL TABLE "DB"."SCH"."E" INSERT_ONLY = TRUE`, got)
	})

	t.Run("FalseFlags", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceTable,
			SourceName: `"DB"."SCH"."T"`,
			AppendOnly: ptr(false),
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON TABLE "DB"."SCH"."T"`, got)
	})

	t.Run("CommentOrderingAfterModeOptions", func(t *testing.T) {
		t.Parallel()
		comment := "test"
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceTable,
			SourceName: `"DB"."SCH"."T"`,
			Comment:    &comment,
		}
		got, err := buildCreateStreamSQL(opts)
		require.NoError(t, err)
		// Verify COMMENT appears after ON TABLE, not before.
		assert.Equal(t, `CREATE STREAM IF NOT EXISTS "DB"."SCH"."S" ON TABLE "DB"."SCH"."T" COMMENT = 'test'`, got)
	})
}

func TestBuildAlterStreamStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "S"),
		}
		stmts, err := buildAlterStreamStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterStreamOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "S"),
			Comment: &comment,
		}
		stmts, err := buildAlterStreamStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "S"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterStreamStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestBuildDropStreamSQL(t *testing.T) {
	t.Parallel()

	got := buildDropStreamSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_STREAM"))
	assert.Equal(t, `DROP STREAM IF EXISTS "DB"."SCH"."MY_STREAM"`, got)
}

func TestBuildShowStreamByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowStreamByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_STREAM"))
	assert.Contains(t, got, "SHOW STREAMS LIKE")
	assert.Contains(t, got, "MY\\_STREAM")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestSourceTypeKeyword(t *testing.T) {
	t.Parallel()

	t.Run("Table", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "TABLE", sourceTypeKeyword(StreamSourceTable))
	})

	t.Run("ExternalTable", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "EXTERNAL TABLE", sourceTypeKeyword(StreamSourceExternalTable))
	})

	t.Run("DynamicTable", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "DYNAMIC TABLE", sourceTypeKeyword(StreamSourceDynamicTable))
	})

	t.Run("View", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "VIEW", sourceTypeKeyword(StreamSourceView))
	})

	t.Run("Stage", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "STAGE", sourceTypeKeyword(StreamSourceStage))
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateStreamOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "S"),
			SourceType: StreamSourceTable,
			SourceName: "src",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{SourceName: "src"}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingSourceName", func(t *testing.T) {
		t.Parallel()
		opts := CreateStreamOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "S"),
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterStreamOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterStreamOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "S")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterStreamOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "S"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterStreamOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "S"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
