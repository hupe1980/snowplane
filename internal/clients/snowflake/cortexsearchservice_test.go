package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateCortexSearchServiceSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicCreate", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MY_SVC"),
			On:        "transcript_text",
			Warehouse: "WH",
			TargetLag: "1 hour",
			Query:     "SELECT transcript_text, region FROM support_transcripts",
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE CORTEX SEARCH SERVICE "DB"."SCH"."MY_SVC"`)
		assert.Contains(t, sql, `ON "transcript_text"`)
		assert.Contains(t, sql, `WAREHOUSE = "WH"`)
		assert.Contains(t, sql, `TARGET_LAG = '1 hour'`)
		assert.Contains(t, sql, `AS (SELECT transcript_text, region FROM support_transcripts)`)
		assert.NotContains(t, sql, "ATTRIBUTES")
		assert.NotContains(t, sql, "EMBEDDING_MODEL")
	})

	t.Run("WithAttributes", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:         "body",
			Attributes: []string{"region", "agent_id"},
			Warehouse:  "WH",
			TargetLag:  "5 minutes",
			Query:      "SELECT body, region, agent_id FROM docs",
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `ATTRIBUTES "region", "agent_id"`)
	})

	t.Run("WithEmbeddingModel", func(t *testing.T) {
		t.Parallel()

		model := "snowflake-arctic-embed-l-v2.0"
		opts := CreateCortexSearchServiceOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:             "text_col",
			Warehouse:      "WH",
			TargetLag:      "1 day",
			Query:          "SELECT text_col FROM t",
			EmbeddingModel: &model,
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `EMBEDDING_MODEL = 'snowflake-arctic-embed-l-v2.0'`)
	})

	t.Run("WithRefreshModeAndInitialize", func(t *testing.T) {
		t.Parallel()

		rm := "FULL"
		init := "ON_SCHEDULE"
		opts := CreateCortexSearchServiceOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:          "text_col",
			Warehouse:   "WH",
			TargetLag:   "1 hour",
			Query:       "SELECT * FROM t",
			RefreshMode: &rm,
			Initialize:  &init,
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "REFRESH_MODE = FULL")
		assert.Contains(t, sql, "INITIALIZE = ON_SCHEDULE")
	})

	t.Run("WithFullIndexBuildIntervalDays", func(t *testing.T) {
		t.Parallel()

		days := int32(7)
		opts := CreateCortexSearchServiceOptions{
			Name:                       NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:                         "text_col",
			Warehouse:                  "WH",
			TargetLag:                  "1 hour",
			Query:                      "SELECT * FROM t",
			FullIndexBuildIntervalDays: &days,
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "FULL_INDEX_BUILD_INTERVAL_DAYS = 7")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		comment := "Search service for support transcripts"
		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:        "text_col",
			Warehouse: "WH",
			TargetLag: "1 hour",
			Query:     "SELECT * FROM t",
			Comment:   &comment,
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "COMMENT = 'Search service for support transcripts'")
	})

	t.Run("FullOptions", func(t *testing.T) {
		t.Parallel()

		model := "snowflake-arctic-embed-l-v2.0"
		rm := "INCREMENTAL"
		init := "ON_CREATE"
		days := int32(14)
		comment := "Full options test"
		opts := CreateCortexSearchServiceOptions{
			Name:                       NewSchemaObjectIdentifier("ANALYTICS", "SEARCH", "TRANSCRIPT_SVC"),
			On:                         "transcript_text",
			Attributes:                 []string{"region", "agent_id", "date"},
			Warehouse:                  "CORTEX_WH",
			TargetLag:                  "1 day",
			Query:                      "SELECT transcript_text, region, agent_id, date FROM transcripts",
			EmbeddingModel:             &model,
			RefreshMode:                &rm,
			Initialize:                 &init,
			FullIndexBuildIntervalDays: &days,
			Comment:                    &comment,
		}

		sql, err := buildCreateCortexSearchServiceSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `"ANALYTICS"."SEARCH"."TRANSCRIPT_SVC"`)
		assert.Contains(t, sql, `ON "transcript_text"`)
		assert.Contains(t, sql, `ATTRIBUTES "region", "agent_id", "date"`)
		assert.Contains(t, sql, `WAREHOUSE = "CORTEX_WH"`)
		assert.Contains(t, sql, `TARGET_LAG = '1 day'`)
		assert.Contains(t, sql, `EMBEDDING_MODEL = 'snowflake-arctic-embed-l-v2.0'`)
		assert.Contains(t, sql, "REFRESH_MODE = INCREMENTAL")
		assert.Contains(t, sql, "INITIALIZE = ON_CREATE")
		assert.Contains(t, sql, "FULL_INDEX_BUILD_INTERVAL_DAYS = 14")
		assert.Contains(t, sql, "COMMENT = 'Full options test'")
		assert.Contains(t, sql, "AS (SELECT transcript_text, region, agent_id, date FROM transcripts)")
	})
}

func TestCreateCortexSearchServiceOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:        "text_col",
			Warehouse: "WH",
			TargetLag: "1 hour",
			Query:     "SELECT * FROM t",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			On:        "text_col",
			Warehouse: "WH",
			TargetLag: "1 hour",
			Query:     "SELECT * FROM t",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cortex search service name is required")
	})

	t.Run("MissingOn", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			Warehouse: "WH",
			TargetLag: "1 hour",
			Query:     "SELECT * FROM t",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "search column (ON) is required")
	})

	t.Run("MissingWarehouse", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:        "text_col",
			TargetLag: "1 hour",
			Query:     "SELECT * FROM t",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "warehouse is required")
	})

	t.Run("MissingTargetLag", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:        "text_col",
			Warehouse: "WH",
			Query:     "SELECT * FROM t",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target lag is required")
	})

	t.Run("MissingQuery", func(t *testing.T) {
		t.Parallel()

		opts := CreateCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:        "text_col",
			Warehouse: "WH",
			TargetLag: "1 hour",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query is required")
	})

	t.Run("InvalidRefreshMode", func(t *testing.T) {
		t.Parallel()

		rm := "INVALID"
		opts := CreateCortexSearchServiceOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:          "text_col",
			Warehouse:   "WH",
			TargetLag:   "1 hour",
			Query:       "SELECT * FROM t",
			RefreshMode: &rm,
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid refresh mode")
	})

	t.Run("InvalidInitialize", func(t *testing.T) {
		t.Parallel()

		init := "INVALID"
		opts := CreateCortexSearchServiceOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			On:         "text_col",
			Warehouse:  "WH",
			TargetLag:  "1 hour",
			Query:      "SELECT * FROM t",
			Initialize: &init,
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid initialize value")
	})
}

func TestBuildAlterCortexSearchServiceStatements(t *testing.T) {
	t.Parallel()

	fqn := NewSchemaObjectIdentifier("DB", "SCH", "SVC")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{Name: fqn}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetTargetLag", func(t *testing.T) {
		t.Parallel()

		tl := "2 hours"
		opts := AlterCortexSearchServiceOptions{Name: fqn, TargetLag: &tl}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET TARGET_LAG = '2 hours'")
	})

	t.Run("SetWarehouse", func(t *testing.T) {
		t.Parallel()

		wh := "NEW_WH"
		opts := AlterCortexSearchServiceOptions{Name: fqn, Warehouse: &wh}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `SET WAREHOUSE = "NEW_WH"`)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		c := "Updated comment"
		opts := AlterCortexSearchServiceOptions{Name: fqn, Comment: &c}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'Updated comment'")
	})

	t.Run("SetFullIndexBuildIntervalDays", func(t *testing.T) {
		t.Parallel()

		days := int32(30)
		opts := AlterCortexSearchServiceOptions{Name: fqn, FullIndexBuildIntervalDays: &days}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET FULL_INDEX_BUILD_INTERVAL_DAYS = 30")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{
			Name:        fqn,
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("MixedOperations", func(t *testing.T) {
		t.Parallel()

		tl := "30 minutes"
		c := "new comment"
		days := int32(7)
		opts := AlterCortexSearchServiceOptions{
			Name:                       fqn,
			TargetLag:                  &tl,
			Comment:                    &c,
			FullIndexBuildIntervalDays: &days,
		}
		stmts, err := buildAlterCortexSearchServiceStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 3)
	})
}

func TestAlterCortexSearchServiceOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "SVC")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithTargetLag", func(t *testing.T) {
		t.Parallel()

		tl := "1 hour"
		opts := AlterCortexSearchServiceOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			TargetLag: &tl,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "SVC"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterCortexSearchServiceOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "SVC")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := AlterCortexSearchServiceOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cortex search service name is required")
	})
}
