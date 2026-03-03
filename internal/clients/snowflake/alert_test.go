package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildCreateAlertSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicAlert", func(t *testing.T) {
		t.Parallel()
		wh := "MY_WH"
		sched := "5 MINUTE"
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ALERT"),
			Warehouse: &wh,
			Schedule:  &sched,
			Condition: "SELECT COUNT(*) FROM my_table WHERE status = 'ERROR'",
			Action:    "CALL SYSTEM$SEND_EMAIL('alerts@example.com', 'Alert', 'Errors detected')",
		}
		got, err := buildCreateAlertSQL(opts)
		require.NoError(t, err)
		assert.Equal(t,
			`CREATE ALERT IF NOT EXISTS "MY_DB"."MY_SCHEMA"."MY_ALERT" WAREHOUSE = "MY_WH" SCHEDULE = '5 MINUTE' IF( EXISTS( SELECT COUNT(*) FROM my_table WHERE status = 'ERROR' )) THEN CALL SYSTEM$SEND_EMAIL('alerts@example.com', 'Alert', 'Errors detected')`,
			got)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "monitoring alert"
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Comment:   &comment,
			Condition: "SELECT 1 FROM t WHERE c > 0",
			Action:    "INSERT INTO log VALUES(CURRENT_TIMESTAMP())",
		}
		got, err := buildCreateAlertSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'monitoring alert'")
		assert.Contains(t, got, "IF( EXISTS( SELECT 1 FROM t WHERE c > 0 ))")
		assert.Contains(t, got, "THEN INSERT INTO log VALUES(CURRENT_TIMESTAMP())")
	})

	t.Run("ServerlessAlert", func(t *testing.T) {
		t.Parallel()
		sched := "USING CRON 0 * * * * UTC"
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Schedule:  &sched,
			Condition: "SELECT 1",
			Action:    "SELECT 1",
		}
		got, err := buildCreateAlertSQL(opts)
		require.NoError(t, err)
		assert.NotContains(t, got, "WAREHOUSE =")
		assert.Contains(t, got, "SCHEDULE = 'USING CRON 0 * * * * UTC'")
	})

	t.Run("MinimalAlert", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: "SELECT 1",
			Action:    "SELECT 2",
		}
		got, err := buildCreateAlertSQL(opts)
		require.NoError(t, err)
		assert.Equal(t,
			`CREATE ALERT IF NOT EXISTS "DB"."SCH"."A" IF( EXISTS( SELECT 1 )) THEN SELECT 2`,
			got)
	})
}

func TestBuildAlterAlertStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "A"),
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("Suspend", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Suspend: ptr(true),
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER ALERT "DB"."SCH"."A" SUSPEND`, stmts[0])
	})

	t.Run("Resume", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Suspend: ptr(false),
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.NotEmpty(t, stmts)
		assert.Equal(t, `ALTER ALERT "DB"."SCH"."A" RESUME`, stmts[len(stmts)-1])
	})

	t.Run("ModifyCondition", func(t *testing.T) {
		t.Parallel()
		cond := "SELECT COUNT(*) FROM errors WHERE ts > DATEADD('hour', -1, CURRENT_TIMESTAMP())"
		opts := AlterAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: &cond,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "MODIFY CONDITION EXISTS (")
		assert.Contains(t, stmts[0], cond)
	})

	t.Run("ModifyAction", func(t *testing.T) {
		t.Parallel()
		action := "CALL notify_team()"
		opts := AlterAlertOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Action: &action,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Equal(t, `ALTER ALERT "DB"."SCH"."A" MODIFY ACTION CALL notify_team()`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Comment: &comment,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'updated'")
	})

	t.Run("SetWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "NEW_WH"
		opts := AlterAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Warehouse: &wh,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `SET WAREHOUSE = NEW_WH`)
	})

	t.Run("SetSchedule", func(t *testing.T) {
		t.Parallel()
		sched := "10 MINUTE"
		opts := AlterAlertOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Schedule: &sched,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET SCHEDULE = '10 MINUTE'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "A"),
			UnsetFields: []string{"COMMENT", "WAREHOUSE"},
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT, WAREHOUSE")
	})

	t.Run("ResumeIsLast", func(t *testing.T) {
		t.Parallel()
		comment := "c"
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Suspend: ptr(false),
			Comment: &comment,
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.True(t, len(stmts) >= 2)
		assert.Contains(t, stmts[len(stmts)-1], "RESUME")
	})

	t.Run("MultipleChanges", func(t *testing.T) {
		t.Parallel()
		cond := "SELECT 1 FROM t"
		action := "INSERT INTO log VALUES(1)"
		comment := "updated"
		opts := AlterAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: &cond,
			Action:    &action,
			Comment:   &comment,
			Suspend:   ptr(false),
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		// Should have: MODIFY CONDITION, MODIFY ACTION, SET COMMENT, RESUME
		assert.True(t, len(stmts) >= 4, "expected at least 4 statements, got %d", len(stmts))
		assert.Contains(t, stmts[len(stmts)-1], "RESUME")
	})

	t.Run("AutoSuspendBeforeModifyCondition", func(t *testing.T) {
		t.Parallel()
		cond := "SELECT 1 FROM t"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition:    &cond,
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 3, "expected SUSPEND + MODIFY CONDITION + RESUME")
		assert.Contains(t, stmts[0], "SUSPEND")
		assert.Contains(t, stmts[1], "MODIFY CONDITION")
		assert.Contains(t, stmts[2], "RESUME")
	})

	t.Run("AutoSuspendBeforeModifyAction", func(t *testing.T) {
		t.Parallel()
		action := "CALL x()"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Action:       &action,
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 3, "expected SUSPEND + MODIFY ACTION + RESUME")
		assert.Contains(t, stmts[0], "SUSPEND")
		assert.Contains(t, stmts[1], "MODIFY ACTION")
		assert.Contains(t, stmts[2], "RESUME")
	})

	t.Run("AutoSuspendBeforeSetSchedule", func(t *testing.T) {
		t.Parallel()
		sched := "5 MINUTE"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Schedule:     &sched,
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 3, "expected SUSPEND + SET SCHEDULE + RESUME")
		assert.Contains(t, stmts[0], "SUSPEND")
		assert.Contains(t, stmts[1], "SCHEDULE")
		assert.Contains(t, stmts[2], "RESUME")
	})

	t.Run("AutoSuspendBeforeSetWarehouse", func(t *testing.T) {
		t.Parallel()
		wh := "WH2"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Warehouse:    &wh,
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 3, "expected SUSPEND + SET WAREHOUSE + RESUME")
		assert.Contains(t, stmts[0], "SUSPEND")
		assert.Contains(t, stmts[1], "WAREHOUSE")
		assert.Contains(t, stmts[2], "RESUME")
	})

	t.Run("NoAutoSuspendWhenAlreadySuspended", func(t *testing.T) {
		t.Parallel()
		cond := "SELECT 1"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition:    &cond,
			CurrentState: "suspended",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1, "no auto-suspend when already suspended")
		assert.Contains(t, stmts[0], "MODIFY CONDITION")
	})

	t.Run("ExplicitSuspendWithModify_NoDoubleOrResume", func(t *testing.T) {
		t.Parallel()
		cond := "SELECT 1"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition:    &cond,
			Suspend:      ptr(true),
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		// SUSPEND + MODIFY CONDITION — no RESUME because user wants it suspended.
		require.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SUSPEND")
		assert.Contains(t, stmts[1], "MODIFY CONDITION")
	})

	t.Run("NoAutoSuspendForCommentOnly", func(t *testing.T) {
		t.Parallel()
		comment := "hi"
		opts := AlterAlertOptions{
			Name:         NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Comment:      &comment,
			CurrentState: "started",
		}
		stmts, err := buildAlterAlertStatements(opts)
		require.NoError(t, err)
		// Comment changes don't require suspend.
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT")
		// No SUSPEND or RESUME.
		for _, s := range stmts {
			assert.NotContains(t, s, "SUSPEND")
			assert.NotContains(t, s, "RESUME")
		}
	})
}

func TestBuildShowAlertByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowAlertByIDSQL(NewSchemaObjectIdentifier("MY_DB", "MY_SCHEMA", "MY_ALERT"))
	assert.Contains(t, got, "SHOW ALERTS LIKE")
	assert.Contains(t, got, "MY\\_ALERT")
	assert.Contains(t, got, `IN SCHEMA "MY_DB"."MY_SCHEMA"`)
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateAlertOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: "SELECT 1",
			Action:    "SELECT 2",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{Condition: "SELECT 1", Action: "SELECT 2"}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingCondition", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Action: "SELECT 2",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "condition")
	})

	t.Run("MissingAction", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: "SELECT 1",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "action")
	})

	t.Run("AllMissing", func(t *testing.T) {
		t.Parallel()
		opts := CreateAlertOptions{}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alert name")
		assert.Contains(t, err.Error(), "condition")
		assert.Contains(t, err.Error(), "action")
	})
}

func TestAlterAlertOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "A")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterAlertOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "A")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("Suspend", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Suspend: ptr(true),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterAlertOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ConditionSet", func(t *testing.T) {
		t.Parallel()
		c := "SELECT 1"
		opts := AlterAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Condition: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ActionSet", func(t *testing.T) {
		t.Parallel()
		a := "SELECT 1"
		opts := AlterAlertOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Action: &a,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WarehouseSet", func(t *testing.T) {
		t.Parallel()
		w := "WH"
		opts := AlterAlertOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Warehouse: &w,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("ScheduleSet", func(t *testing.T) {
		t.Parallel()
		s := "5 MINUTE"
		opts := AlterAlertOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "A"),
			Schedule: &s,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterAlertOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "A"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
