package task

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ptr[T any](v T) *T { return &v }

func newTask() *snowplanev1alpha1.Task {
	return &snowplanev1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Spec: snowplanev1alpha1.TaskSpec{
			Name: "TEST_TASK",
		},
	}
}

func TestLateInitialize(t *testing.T) {
	a := &adapter{}

	t.Run("fills all nil fields from observation", func(t *testing.T) {
		obj := newTask()
		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: &snowflake.TaskObservation{
				ShowOutput: &snowflake.TaskShowOutput{
					Schedule:                  "USING CRON 0 9 * * * UTC",
					Comment:                   "daily etl",
					AllowOverlappingExecution: true,
					ErrorIntegration:          "MY_NOTIFICATION",
					Condition:                 "SYSTEM$STREAM_HAS_DATA('MY_STREAM')",
					Warehouse:                 "COMPUTE_WH",
					Config:                    `{"key":"val"}`,
					State:                     "suspended",
				},
				Parameters: &snowflake.TaskParameters{
					UserTaskTimeoutMs:                       ptr(int32(3600000)),
					SuspendTaskAfterNumFailures:             ptr(int32(10)),
					TaskAutoRetryAttempts:                   ptr(int32(3)),
					LogLevel:                                "INFO",
					UserTaskMinimumTriggerIntervalInSeconds: ptr(int32(30)),
					TargetCompletionInterval:                ptr("5 MINUTE"),
					UserTaskManagedInitialWarehouseSize:     ptr("MEDIUM"),
					ServerlessTaskMinStatementSize:          ptr("SMALL"),
					ServerlessTaskMaxStatementSize:          ptr("XLARGE"),
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)

		assert.Equal(t, "USING CRON 0 9 * * * UTC", *obj.Spec.Schedule)
		assert.Equal(t, "daily etl", *obj.Spec.Comment)
		assert.Equal(t, true, *obj.Spec.AllowOverlappingExecution)
		assert.Equal(t, "MY_NOTIFICATION", *obj.Spec.ErrorIntegrationName)
		assert.Equal(t, "SYSTEM$STREAM_HAS_DATA('MY_STREAM')", *obj.Spec.When)
		assert.Equal(t, "COMPUTE_WH", *obj.Spec.WarehouseName)
		assert.Equal(t, `{"key":"val"}`, *obj.Spec.Config)
		assert.Equal(t, true, *obj.Spec.Suspend) // State="suspended" → true

		assert.Equal(t, int32(3600000), *obj.Spec.UserTaskTimeoutMs)
		assert.Equal(t, int32(10), *obj.Spec.SuspendTaskAfterNumFailures)
		assert.Equal(t, int32(3), *obj.Spec.TaskAutoRetryAttempts)
		assert.Equal(t, snowplanev1alpha1.LogLevel("INFO"), *obj.Spec.LogLevel)
		assert.Equal(t, int32(30), *obj.Spec.UserTaskMinimumTriggerIntervalInSeconds)
		assert.Equal(t, "5 MINUTE", *obj.Spec.TargetCompletionInterval)
		assert.Equal(t, "MEDIUM", *obj.Spec.UserTaskManagedInitialWarehouseSize)
		assert.Equal(t, "SMALL", *obj.Spec.ServerlessTaskMinStatementSize)
		assert.Equal(t, "XLARGE", *obj.Spec.ServerlessTaskMaxStatementSize)
	})

	t.Run("does not overwrite existing spec fields", func(t *testing.T) {
		obj := newTask()
		obj.Spec.Comment = ptr("user comment")
		obj.Spec.Suspend = ptr(false)

		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: &snowflake.TaskObservation{
				ShowOutput: &snowflake.TaskShowOutput{
					Comment:  "snowflake comment",
					State:    "suspended",
					Schedule: "USING CRON 0 * * * * UTC",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified) // Schedule was set

		// Existing fields preserved
		assert.Equal(t, "user comment", *obj.Spec.Comment)
		assert.Equal(t, false, *obj.Spec.Suspend)

		// New field populated
		assert.Equal(t, "USING CRON 0 * * * * UTC", *obj.Spec.Schedule)
	})

	t.Run("returns false when all fields already set", func(t *testing.T) {
		obj := newTask()
		obj.Spec.Schedule = ptr("USING CRON 0 * * * * UTC")
		obj.Spec.Comment = ptr("c")
		obj.Spec.AllowOverlappingExecution = ptr(false)
		obj.Spec.ErrorIntegrationName = ptr("e")
		obj.Spec.When = ptr("w")
		obj.Spec.WarehouseName = ptr("wh")
		obj.Spec.Config = ptr("{}")
		obj.Spec.Suspend = ptr(false)
		obj.Spec.UserTaskTimeoutMs = ptr(int32(1))
		obj.Spec.SuspendTaskAfterNumFailures = ptr(int32(1))
		obj.Spec.TaskAutoRetryAttempts = ptr(int32(1))
		ll := snowplanev1alpha1.LogLevel("OFF")
		obj.Spec.LogLevel = &ll
		obj.Spec.UserTaskMinimumTriggerIntervalInSeconds = ptr(int32(1))
		obj.Spec.TargetCompletionInterval = ptr("1 MINUTE")
		obj.Spec.UserTaskManagedInitialWarehouseSize = ptr("SMALL")
		obj.Spec.ServerlessTaskMinStatementSize = ptr("SMALL")
		obj.Spec.ServerlessTaskMaxStatementSize = ptr("LARGE")

		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: &snowflake.TaskObservation{
				ShowOutput: &snowflake.TaskShowOutput{Comment: "other", State: "started"},
				Parameters: &snowflake.TaskParameters{
					UserTaskTimeoutMs: ptr(int32(99)),
					LogLevel:          "DEBUG",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("returns false when detail is nil", func(t *testing.T) {
		obj := newTask()
		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: nil,
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("handles nil show output and parameters", func(t *testing.T) {
		obj := newTask()
		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: &snowflake.TaskObservation{
				ShowOutput: nil,
				Parameters: nil,
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.False(t, modified)
	})

	t.Run("suspend is false when state is started", func(t *testing.T) {
		obj := newTask()
		obs := &reconciler.Observation[*snowflake.TaskObservation]{
			Exists: true,
			Detail: &snowflake.TaskObservation{
				ShowOutput: &snowflake.TaskShowOutput{
					State: "started",
				},
			},
		}

		modified := a.LateInitialize(obj, obs)
		assert.True(t, modified)
		assert.Equal(t, false, *obj.Spec.Suspend)
	})
}
