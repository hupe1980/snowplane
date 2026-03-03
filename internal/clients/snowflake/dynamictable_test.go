package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateDynamicTableSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MY_DT"),
			Query:     "SELECT * FROM DB.SCH.T",
			TargetLag: "5 minutes",
			Warehouse: "MY_WH",
		}
		got, err := buildCreateDynamicTableSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `CREATE DYNAMIC TABLE "DB"."SCH"."MY_DT"`)
		assert.Contains(t, got, "TARGET_LAG = '5 minutes'")
		assert.Contains(t, got, `WAREHOUSE = "MY_WH"`)
		assert.Contains(t, got, "AS SELECT * FROM DB.SCH.T")
	})

	t.Run("WithRefreshMode", func(t *testing.T) {
		t.Parallel()
		rm := "FULL"
		opts := CreateDynamicTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:       "SELECT 1",
			TargetLag:   "1 hour",
			Warehouse:   "WH",
			RefreshMode: &rm,
		}
		got, err := buildCreateDynamicTableSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "REFRESH_MODE = FULL")
	})

	t.Run("WithInitialize", func(t *testing.T) {
		t.Parallel()
		init := "ON_SCHEDULE"
		opts := CreateDynamicTableOptions{
			Name:       NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:      "SELECT 1",
			TargetLag:  "DOWNSTREAM",
			Warehouse:  "WH",
			Initialize: &init,
		}
		got, err := buildCreateDynamicTableSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "INITIALIZE = ON_SCHEDULE")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		rm := "INCREMENTAL"
		init := "ON_CREATE"
		comment := "my dyn table"
		opts := CreateDynamicTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:       "SELECT a FROM T",
			TargetLag:   "10 minutes",
			Warehouse:   "COMPUTE_WH",
			RefreshMode: &rm,
			Initialize:  &init,
			Comment:     &comment,
		}
		got, err := buildCreateDynamicTableSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TARGET_LAG = '10 minutes'")
		assert.Contains(t, got, `WAREHOUSE = "COMPUTE_WH"`)
		assert.Contains(t, got, "REFRESH_MODE = INCREMENTAL")
		assert.Contains(t, got, "INITIALIZE = ON_CREATE")
		assert.Contains(t, got, "COMMENT = 'my dyn table'")
		assert.Contains(t, got, "AS SELECT a FROM T")
	})

	t.Run("TargetLagDownstream", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:     "SELECT 1",
			TargetLag: "DOWNSTREAM",
			Warehouse: "WH",
		}
		got, err := buildCreateDynamicTableSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "TARGET_LAG = 'DOWNSTREAM'")
	})
}

func TestCreateDynamicTableOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:     "SELECT 1",
			TargetLag: "1 minute",
			Warehouse: "WH",
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Query:     "SELECT 1",
			TargetLag: "1 minute",
			Warehouse: "WH",
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingQuery", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			TargetLag: "1 minute",
			Warehouse: "WH",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query")
	})

	t.Run("MissingTargetLag", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:     "SELECT 1",
			Warehouse: "WH",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target lag")
	})

	t.Run("MissingWarehouse", func(t *testing.T) {
		t.Parallel()
		opts := CreateDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Query:     "SELECT 1",
			TargetLag: "1 minute",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "warehouse")
	})
}

func TestBuildAlterDynamicTableStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "DT"),
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetTargetLag", func(t *testing.T) {
		t.Parallel()
		tl := "10 minutes"
		opts := AlterDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			TargetLag: &tl,
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALTER DYNAMIC TABLE")
		assert.Contains(t, stmts[0], "SET TARGET_LAG = '10 minutes'")
	})

	t.Run("SetWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "NEW_WH"
		opts := AlterDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Warehouse: &wh,
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `WAREHOUSE = "NEW_WH"`)
	})

	t.Run("SetBoth", func(t *testing.T) {
		t.Parallel()
		tl := "5 minutes"
		wh := "WH2"
		opts := AlterDynamicTableOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			TargetLag: &tl,
			Warehouse: &wh,
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated comment"
		opts := AlterDynamicTableOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			Comment: &comment,
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated comment'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "DT"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterDynamicTableStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALTER DYNAMIC TABLE")
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestBuildShowDynamicTableByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowDynamicTableByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_DT"))
	assert.Contains(t, got, "SHOW DYNAMIC TABLES LIKE 'MY\\_DT'")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestBuildDropDynamicTableSQL(t *testing.T) {
	t.Parallel()
	got := buildDropDynamicTableSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_DT"))
	assert.Contains(t, got, `DROP DYNAMIC TABLE IF EXISTS "DB"."SCH"."MY_DT"`)
}

func TestAlterDynamicTableOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithTargetLag", func(t *testing.T) {
		t.Parallel()
		tl := "1 hour"
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT"), TargetLag: &tl}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "WH"
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT"), Warehouse: &wh}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "hello"
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterDynamicTableOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "DT")}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterDynamicTableOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

func TestCreateDynamicTableOptions_Validate_InvalidRefreshMode(t *testing.T) {
	t.Parallel()

	badRM := "BADMODE"
	opts := CreateDynamicTableOptions{
		Name:        NewSchemaObjectIdentifier("DB", "SCH", "DT"),
		Query:       "SELECT 1",
		TargetLag:   "1 minute",
		Warehouse:   "WH",
		RefreshMode: &badRM,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid refresh mode")
}

func TestCreateDynamicTableOptions_Validate_InvalidInitialize(t *testing.T) {
	t.Parallel()

	badInit := "IMMEDIATELY"
	opts := CreateDynamicTableOptions{
		Name:       NewSchemaObjectIdentifier("DB", "SCH", "DT"),
		Query:      "SELECT 1",
		TargetLag:  "1 minute",
		Warehouse:  "WH",
		Initialize: &badInit,
	}
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid initialize value")
}

func TestCreateDynamicTableOptions_Validate_ValidRefreshModes(t *testing.T) {
	t.Parallel()

	for _, rm := range []string{"AUTO", "FULL", "INCREMENTAL"} {
		t.Run(rm, func(t *testing.T) {
			t.Parallel()
			rm := rm
			opts := CreateDynamicTableOptions{
				Name:        NewSchemaObjectIdentifier("DB", "SCH", "DT"),
				Query:       "SELECT 1",
				TargetLag:   "1 minute",
				Warehouse:   "WH",
				RefreshMode: &rm,
			}
			require.NoError(t, opts.Validate())
		})
	}
}

func TestCreateDynamicTableOptions_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	opts := CreateDynamicTableOptions{}
	err := opts.Validate()
	require.Error(t, err)
	// Should contain errors for name, query, target lag, and warehouse.
	assert.Contains(t, err.Error(), "name")
	assert.Contains(t, err.Error(), "query")
	assert.Contains(t, err.Error(), "target lag")
	assert.Contains(t, err.Error(), "warehouse")
}
