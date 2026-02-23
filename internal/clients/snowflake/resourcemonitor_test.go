package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildTriggersClause(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := buildTriggersClause(nil)
		assert.Equal(t, "", got)
	})

	t.Run("SingleTrigger", func(t *testing.T) {
		t.Parallel()
		triggers := []ResourceMonitorTrigger{
			{Threshold: 80, Action: "SUSPEND"},
		}
		got := buildTriggersClause(triggers)
		assert.Equal(t, "TRIGGERS ON 80 PERCENT DO SUSPEND", got)
	})

	t.Run("MultiTriggers", func(t *testing.T) {
		t.Parallel()
		triggers := []ResourceMonitorTrigger{
			{Threshold: 80, Action: "NOTIFY"},
			{Threshold: 100, Action: "SUSPEND"},
			{Threshold: 110, Action: "SUSPEND_IMMEDIATE"},
		}
		got := buildTriggersClause(triggers)
		assert.Equal(t, "TRIGGERS ON 80 PERCENT DO NOTIFY ON 100 PERCENT DO SUSPEND ON 110 PERCENT DO SUSPEND_IMMEDIATE", got)
	})
}

func TestBuildNotifyUsersClause(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		got := buildNotifyUsersClause(nil)
		assert.Equal(t, "", got)
	})

	t.Run("SingleUser", func(t *testing.T) {
		t.Parallel()
		got := buildNotifyUsersClause([]string{"ADMIN"})
		assert.Equal(t, "NOTIFY_USERS = (ADMIN)", got)
	})

	t.Run("MultiUsers", func(t *testing.T) {
		t.Parallel()
		got := buildNotifyUsersClause([]string{"USER1", "USER2"})
		assert.Equal(t, "NOTIFY_USERS = (USER1, USER2)", got)
	})
}

func TestBuildCreateResourceMonitorSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicMonitor", func(t *testing.T) {
		t.Parallel()
		opts := CreateResourceMonitorOptions{
			Name: NewAccountObjectIdentifier("MY_MONITOR"),
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Equal(t, `CREATE RESOURCE MONITOR IF NOT EXISTS "MY_MONITOR" WITH`, got)
	})

	t.Run("WithCreditQuota", func(t *testing.T) {
		t.Parallel()
		opts := CreateResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("MON"),
			CreditQuota: int32Ptr(100),
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Contains(t, got, "CREDIT_QUOTA = 100")
	})

	t.Run("WithFrequencyAndTimestamp", func(t *testing.T) {
		t.Parallel()
		freq := "MONTHLY"
		ts := "2024-01-01 00:00"
		opts := CreateResourceMonitorOptions{
			Name:           NewAccountObjectIdentifier("MON"),
			Frequency:      &freq,
			StartTimestamp: &ts,
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Contains(t, got, "FREQUENCY = MONTHLY")
		assert.Contains(t, got, "START_TIMESTAMP = '2024-01-01 00:00'")
	})

	t.Run("WithImmediateStart", func(t *testing.T) {
		t.Parallel()
		ts := "IMMEDIATELY"
		opts := CreateResourceMonitorOptions{
			Name:           NewAccountObjectIdentifier("MON"),
			StartTimestamp: &ts,
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Contains(t, got, "START_TIMESTAMP = IMMEDIATELY")
		assert.NotContains(t, got, "'IMMEDIATELY'")
	})

	t.Run("WithEndTimestamp", func(t *testing.T) {
		t.Parallel()
		endTs := "2024-12-31 23:59"
		opts := CreateResourceMonitorOptions{
			Name:         NewAccountObjectIdentifier("MON"),
			EndTimestamp: &endTs,
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Contains(t, got, "END_TIMESTAMP = '2024-12-31 23:59'")
	})

	t.Run("WithNotifyUsersAndTriggers", func(t *testing.T) {
		t.Parallel()
		opts := CreateResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("MON"),
			NotifyUsers: []string{"ADMIN", "DBA"},
			Triggers: []ResourceMonitorTrigger{
				{Threshold: 90, Action: "NOTIFY"},
				{Threshold: 100, Action: "SUSPEND"},
			},
		}
		got := buildCreateResourceMonitorSQL(opts)
		assert.Contains(t, got, "NOTIFY_USERS = (ADMIN, DBA)")
		assert.Contains(t, got, "TRIGGERS ON 90 PERCENT DO NOTIFY ON 100 PERCENT DO SUSPEND")
	})
}

func TestBuildAlterResourceMonitorSQL(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{
			Name: NewAccountObjectIdentifier("MON"),
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Equal(t, "", got)
	})

	t.Run("SetCreditQuota", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("MON"),
			CreditQuota: int32Ptr(200),
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, `ALTER RESOURCE MONITOR "MON"`)
		assert.Contains(t, got, "SET CREDIT_QUOTA = 200")
	})

	t.Run("SetFrequency", func(t *testing.T) {
		t.Parallel()
		freq := "WEEKLY"
		opts := AlterResourceMonitorOptions{
			Name:      NewAccountObjectIdentifier("MON"),
			Frequency: &freq,
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, "SET FREQUENCY = WEEKLY")
	})

	t.Run("SetStartTimestampImmediately", func(t *testing.T) {
		t.Parallel()
		ts := "immediately"
		opts := AlterResourceMonitorOptions{
			Name:           NewAccountObjectIdentifier("MON"),
			StartTimestamp: &ts,
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, "START_TIMESTAMP = IMMEDIATELY")
	})

	t.Run("SetEndTimestamp", func(t *testing.T) {
		t.Parallel()
		endTs := "2025-01-01 00:00"
		opts := AlterResourceMonitorOptions{
			Name:         NewAccountObjectIdentifier("MON"),
			EndTimestamp: &endTs,
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, "END_TIMESTAMP = '2025-01-01 00:00'")
	})

	t.Run("SetNotifyUsers", func(t *testing.T) {
		t.Parallel()
		users := []string{"USER1"}
		opts := AlterResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("MON"),
			NotifyUsers: &users,
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, "SET NOTIFY_USERS = (USER1)")
	})

	t.Run("SetTriggers", func(t *testing.T) {
		t.Parallel()
		triggers := []ResourceMonitorTrigger{
			{Threshold: 50, Action: "NOTIFY"},
		}
		opts := AlterResourceMonitorOptions{
			Name:     NewAccountObjectIdentifier("MON"),
			Triggers: &triggers,
		}
		got := buildAlterResourceMonitorSQL(opts)
		assert.Contains(t, got, "TRIGGERS ON 50 PERCENT DO NOTIFY")
	})
}

func TestBuildShowResourceMonitorByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowResourceMonitorByIDSQL(NewAccountObjectIdentifier("MY_MON"))
	assert.Contains(t, got, "SHOW RESOURCE MONITORS LIKE")
	assert.Contains(t, got, "MY\\_MON")
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateResourceMonitorOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateResourceMonitorOptions{Name: NewAccountObjectIdentifier("M")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateResourceMonitorOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterResourceMonitorOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{Name: NewAccountObjectIdentifier("M")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterResourceMonitorOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{Name: NewAccountObjectIdentifier("M")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("CreditQuota", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("M"),
			CreditQuota: int32Ptr(100),
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("Frequency", func(t *testing.T) {
		t.Parallel()
		f := "DAILY"
		opts := AlterResourceMonitorOptions{
			Name:      NewAccountObjectIdentifier("M"),
			Frequency: &f,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("Triggers", func(t *testing.T) {
		t.Parallel()
		triggers := []ResourceMonitorTrigger{{Threshold: 80, Action: "NOTIFY"}}
		opts := AlterResourceMonitorOptions{
			Name:     NewAccountObjectIdentifier("M"),
			Triggers: &triggers,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterResourceMonitorOptions{
			Name:        NewAccountObjectIdentifier("M"),
			UnsetFields: []string{"SOMETHING"},
		}
		assert.True(t, opts.HasChanges())
	})
}
