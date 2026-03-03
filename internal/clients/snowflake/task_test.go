package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateTaskSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicTask", func(t *testing.T) {
		t.Parallel()
		wh := "MY_WH"
		sched := "5 MINUTE"
		opts := CreateTaskOptions{
			Name:         NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK"),
			Warehouse:    &wh,
			Schedule:     &sched,
			SQLStatement: "SELECT 1",
		}
		got, err := buildCreateTaskSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE TASK IF NOT EXISTS "MY_DB"."MY_SCHEMA"."MY_TASK" WAREHOUSE = "MY_WH" SCHEDULE = '5 MINUTE' AS SELECT 1`, got)
	})

	t.Run("CreateOrAlter", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{
			Name:             NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement:     "CALL my_proc()",
			UseCreateOrAlter: true,
		}
		got, err := buildCreateTaskSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "CREATE OR ALTER TASK ")
		assert.Contains(t, got, `AS CALL my_proc()`)
	})

	t.Run("WithUserTaskManagedInitialWarehouseSize", func(t *testing.T) {
		t.Parallel()
		size := "SMALL"
		opts := CreateTaskOptions{
			Name:                                NewSchemaObjectIdentifier("DB", "SCH", "T"),
			UserTaskManagedInitialWarehouseSize: &size,
			SQLStatement:                        "SELECT 1",
		}
		got, err := buildCreateTaskSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE = 'SMALL'")
		assert.NotContains(t, got, "WAREHOUSE =")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()
		wh := "WH"
		sched := "USING CRON 0 * * * * UTC"
		when := "SYSTEM$STREAM_HAS_DATA('MY_STREAM')"
		comment := "test task"
		errInt := "MY_ERR_INT"
		succInt := "MY_SUCC_INT"
		opts := CreateTaskOptions{
			Name:                        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Warehouse:                   &wh,
			Schedule:                    &sched,
			SQLStatement:                "INSERT INTO t SELECT * FROM s",
			After:                       []string{"PARENT_TASK1", "PARENT_TASK2"},
			When:                        &when,
			Comment:                     &comment,
			AllowOverlappingExecution:   ptr(true),
			UserTaskTimeoutMs:           ptr(int32(60000)),
			SuspendTaskAfterNumFailures: ptr(int32(3)),
			ErrorIntegration:            &errInt,
			SuccessIntegration:          &succInt,
			TaskAutoRetryAttempts:       ptr(int32(2)),
		}
		got, err := buildCreateTaskSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `WAREHOUSE = "WH"`)
		assert.Contains(t, got, `SCHEDULE = 'USING CRON 0 * * * * UTC'`)
		assert.Contains(t, got, "ALLOW_OVERLAPPING_EXECUTION = TRUE")
		assert.Contains(t, got, "USER_TASK_TIMEOUT_MS = 60000")
		assert.Contains(t, got, "SUSPEND_TASK_AFTER_NUM_FAILURES = 3")
		assert.Contains(t, got, "TASK_AUTO_RETRY_ATTEMPTS = 2")
		assert.Contains(t, got, `ERROR_INTEGRATION = "MY_ERR_INT"`)
		assert.Contains(t, got, `SUCCESS_INTEGRATION = "MY_SUCC_INT"`)
		assert.Contains(t, got, "COMMENT = 'test task'")
		assert.Contains(t, got, `AFTER "PARENT_TASK1", "PARENT_TASK2"`)
		assert.Contains(t, got, "WHEN SYSTEM$STREAM_HAS_DATA('MY_STREAM')")
		assert.Contains(t, got, "AS INSERT INTO t SELECT * FROM s")
	})
}

func TestBuildAlterTaskStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "T"),
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("Suspend", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Suspend: ptr(true),
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER TASK "DB"."SCH"."T" SUSPEND`, stmts[0])
	})

	t.Run("Resume", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Suspend: ptr(false),
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		// RESUME should be the last statement
		assert.Equal(t, `ALTER TASK "DB"."SCH"."T" RESUME`, stmts[len(stmts)-1])
	})

	t.Run("ModifySQL", func(t *testing.T) {
		t.Parallel()
		sql := "SELECT 2"
		opts := AlterTaskOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement: &sql,
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER TASK "DB"."SCH"."T" MODIFY AS SELECT 2`, stmts[0])
	})

	t.Run("ModifyWhen", func(t *testing.T) {
		t.Parallel()
		when := "SYSTEM$STREAM_HAS_DATA('S')"
		opts := AlterTaskOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "T"),
			When: &when,
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "MODIFY WHEN SYSTEM$STREAM_HAS_DATA('S')")
	})

	t.Run("ModifyWhenEmpty", func(t *testing.T) {
		t.Parallel()
		when := ""
		opts := AlterTaskOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "T"),
			When: &when,
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "MODIFY WHEN TRUE")
	})

	t.Run("SetAfter", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SetAfter: []string{"PARENT1"},
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `ADD AFTER "PARENT1"`)
	})

	t.Run("RemoveAfter", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			RemoveAfter: []string{"PARENT1", "PARENT2"},
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `REMOVE AFTER "PARENT1", "PARENT2"`)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Comment: &comment,
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("ResumeIsLast", func(t *testing.T) {
		t.Parallel()
		comment := "c"
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Suspend: ptr(false),
			Comment: &comment,
		}
		stmts, err := buildAlterTaskStatements(opts)
		require.NoError(t, err)
		require.True(t, len(stmts) >= 2)
		assert.Contains(t, stmts[len(stmts)-1], "RESUME")
	})
}

func TestBuildShowTaskByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowTaskByIDSQL(NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK"))
	assert.Contains(t, got, "SHOW TASKS LIKE")
	assert.Contains(t, got, "MY\\_TASK")
	assert.Contains(t, got, `IN SCHEMA "MY_DB"."MY_SCHEMA"`)
}

func TestBuildShowTaskParametersSQL(t *testing.T) {
	t.Parallel()

	got := buildShowTaskParametersSQL(NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_TASK"))
	assert.Equal(t, `SHOW PARAMETERS IN TASK "MY_DB"."MY_SCHEMA"."MY_TASK"`, got)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateTaskOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement: "SELECT 1",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{SQLStatement: "SELECT 1"}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingSQLStatement", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "T"),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MutuallyExclusiveWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "WH"
		size := "SMALL"
		opts := CreateTaskOptions{
			Name:                                NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement:                        "SELECT 1",
			Warehouse:                           &wh,
			UserTaskManagedInitialWarehouseSize: &size,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidTimeoutMs", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{
			Name:              NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement:      "SELECT 1",
			UserTaskTimeoutMs: ptr(int32(-1)),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidAutoRetry", func(t *testing.T) {
		t.Parallel()
		opts := CreateTaskOptions{
			Name:                  NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement:          "SELECT 1",
			TaskAutoRetryAttempts: ptr(int32(31)),
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("ConfigWithDollarQuoting", func(t *testing.T) {
		t.Parallel()
		cfg := `{"key": "value$$injection$$"}`
		opts := CreateTaskOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement: "SELECT 1",
			Config:       &cfg,
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config")
	})

	t.Run("ConfigWithSemicolon", func(t *testing.T) {
		t.Parallel()
		cfg := `{"key": "value"}; DROP TABLE x`
		opts := CreateTaskOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement: "SELECT 1",
			Config:       &cfg,
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config")
	})

	t.Run("ValidConfig", func(t *testing.T) {
		t.Parallel()
		cfg := `{"output_dir": "/temp/test_directory/", "learning_rate": 0.1}`
		opts := CreateTaskOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SQLStatement: "SELECT 1",
			Config:       &cfg,
		}
		assert.NoError(t, opts.Validate())
	})
}

func TestAlterTaskOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "T")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{}
		assert.Error(t, opts.Validate())
	})

	t.Run("MutuallyExclusiveWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "WH"
		size := "SMALL"
		opts := AlterTaskOptions{
			Name:                                NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Warehouse:                           &wh,
			UserTaskManagedInitialWarehouseSize: &size,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("ConfigWithDollarQuoting", func(t *testing.T) {
		t.Parallel()
		cfg := `value$$injection$$`
		opts := AlterTaskOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Config: &cfg,
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config")
	})
}

func TestAlterTaskOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "T")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("Suspend", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Suspend: ptr(true),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterTaskOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "T"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "T"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("SetAfter", func(t *testing.T) {
		t.Parallel()
		opts := AlterTaskOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "T"),
			SetAfter: []string{"P"},
		}
		assert.True(t, opts.HasChanges())
	})
}
