package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// TaskObservation holds the result of observing a Snowflake task.
type TaskObservation struct {
	// Exists indicates whether the task was found.
	Exists bool

	// ShowOutput contains the SHOW TASKS row.
	ShowOutput *v1alpha1.TaskShowOutput

	// Parameters contains the task-level parameters from SHOW PARAMETERS IN TASK.
	Parameters *TaskParameters
}

// TaskParameters contains relevant task parameters from SHOW PARAMETERS IN TASK.
type TaskParameters struct {
	UserTaskTimeoutMs                       *int32
	SuspendTaskAfterNumFailures             *int32
	TaskAutoRetryAttempts                   *int32
	LogLevel                                string
	UserTaskMinimumTriggerIntervalInSeconds *int32
	TargetCompletionInterval                *string
	UserTaskManagedInitialWarehouseSize     *string
	ServerlessTaskMinStatementSize          *string
	ServerlessTaskMaxStatementSize          *string
}

// CreateTaskOptions holds the parameters for creating a task.
type CreateTaskOptions struct {
	Name                                    SchemaObjectIdentifier
	Warehouse                               *string
	UserTaskManagedInitialWarehouseSize     *string
	Schedule                                *string
	SQLStatement                            string
	After                                   []string
	When                                    *string
	Comment                                 *string
	AllowOverlappingExecution               *bool
	UserTaskTimeoutMs                       *int32
	SuspendTaskAfterNumFailures             *int32
	ErrorIntegration                        *string
	SuccessIntegration                      *string
	TaskAutoRetryAttempts                   *int32
	Config                                  *string
	Finalize                                *string
	LogLevel                                *string
	UserTaskMinimumTriggerIntervalInSeconds *int32
	TargetCompletionInterval                *string
	ServerlessTaskMinStatementSize          *string
	ServerlessTaskMaxStatementSize          *string

	// UseCreateOrAlter emits CREATE OR ALTER TASK instead of CREATE TASK IF NOT EXISTS.
	UseCreateOrAlter bool
}

// Validate checks the CreateTaskOptions for validity.
func (o *CreateTaskOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("task name is required"))
	}

	if o.SQLStatement == "" {
		errs = append(errs, fmt.Errorf("SQL statement is required"))
	}

	if o.Warehouse != nil && o.UserTaskManagedInitialWarehouseSize != nil {
		errs = append(errs, fmt.Errorf("warehouse and userTaskManagedInitialWarehouseSize are mutually exclusive"))
	}

	if o.UserTaskTimeoutMs != nil {
		v := *o.UserTaskTimeoutMs
		if v < 0 || v > 604800000 {
			errs = append(errs, fmt.Errorf("userTaskTimeoutMs must be between 0 and 604800000 (got: %d)", v))
		}
	}

	if o.TaskAutoRetryAttempts != nil {
		v := *o.TaskAutoRetryAttempts
		if v < 0 || v > 30 {
			errs = append(errs, fmt.Errorf("taskAutoRetryAttempts must be between 0 and 30 (got: %d)", v))
		}
	}

	if o.Config != nil {
		if err := sqlbuilder.ValidateDollarQuotedValue(*o.Config); err != nil {
			errs = append(errs, fmt.Errorf("invalid config: %w", err))
		}
	}

	return errors.Join(errs...)
}

// AlterTaskOptions holds the parameters for altering a task.
type AlterTaskOptions struct {
	Name                                    SchemaObjectIdentifier
	Warehouse                               *string
	UserTaskManagedInitialWarehouseSize     *string
	Schedule                                *string
	SQLStatement                            *string
	When                                    *string
	Comment                                 *string
	AllowOverlappingExecution               *bool
	UserTaskTimeoutMs                       *int32
	SuspendTaskAfterNumFailures             *int32
	ErrorIntegration                        *string
	SuccessIntegration                      *string
	TaskAutoRetryAttempts                   *int32
	Config                                  *string
	LogLevel                                *string
	UserTaskMinimumTriggerIntervalInSeconds *int32
	TargetCompletionInterval                *string
	ServerlessTaskMinStatementSize          *string
	ServerlessTaskMaxStatementSize          *string
	Suspend                                 *bool

	// SetFinalize sets the finalizer task root.
	SetFinalize *string
	// UnsetFinalize removes the finalizer association.
	UnsetFinalize bool

	// SetAfter replaces the predecessor list.
	SetAfter []string
	// RemoveAfter removes specific predecessors.
	RemoveAfter []string

	// UnsetFields lists Snowflake parameter names to revert to defaults.
	UnsetFields []string
}

// Validate checks the AlterTaskOptions for validity.
func (o *AlterTaskOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("task name is required"))
	}

	if o.Warehouse != nil && o.UserTaskManagedInitialWarehouseSize != nil {
		errs = append(errs, fmt.Errorf("warehouse and userTaskManagedInitialWarehouseSize are mutually exclusive"))
	}

	if o.Config != nil {
		if err := sqlbuilder.ValidateDollarQuotedValue(*o.Config); err != nil {
			errs = append(errs, fmt.Errorf("invalid config: %w", err))
		}
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterTaskOptions) HasChanges() bool {
	return o.Warehouse != nil ||
		o.UserTaskManagedInitialWarehouseSize != nil ||
		o.Schedule != nil ||
		o.SQLStatement != nil ||
		o.When != nil ||
		o.Comment != nil ||
		o.AllowOverlappingExecution != nil ||
		o.UserTaskTimeoutMs != nil ||
		o.SuspendTaskAfterNumFailures != nil ||
		o.ErrorIntegration != nil ||
		o.SuccessIntegration != nil ||
		o.TaskAutoRetryAttempts != nil ||
		o.Config != nil ||
		o.LogLevel != nil ||
		o.UserTaskMinimumTriggerIntervalInSeconds != nil ||
		o.TargetCompletionInterval != nil ||
		o.ServerlessTaskMinStatementSize != nil ||
		o.ServerlessTaskMaxStatementSize != nil ||
		o.SetFinalize != nil ||
		o.UnsetFinalize ||
		o.Suspend != nil ||
		len(o.SetAfter) > 0 ||
		len(o.RemoveAfter) > 0 ||
		len(o.UnsetFields) > 0
}

// TaskClient provides operations against Snowflake tasks.
type TaskClient struct {
	client SQLExecutor
}

// NewTaskClient creates a new TaskClient backed by the given SQLExecutor.
func NewTaskClient(c SQLExecutor) *TaskClient {
	return &TaskClient{client: c}
}

// buildCreateTaskSQL builds the CREATE TASK SQL statement.
func buildCreateTaskSQL(opts CreateTaskOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "TASK", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

	if opts.Warehouse != nil {
		fmt.Fprintf(&b.Builder, " WAREHOUSE = %s", sqlbuilder.QuoteIdentifier(*opts.Warehouse))
	}

	if opts.UserTaskManagedInitialWarehouseSize != nil {
		fmt.Fprintf(&b.Builder, " USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE = '%s'", *opts.UserTaskManagedInitialWarehouseSize)
	}

	if opts.Schedule != nil {
		fmt.Fprintf(&b.Builder, " SCHEDULE = '%s'", sqlbuilder.EscapeString(*opts.Schedule))
	}

	b.SetBool("ALLOW_OVERLAPPING_EXECUTION", opts.AllowOverlappingExecution)
	b.SetInt32("USER_TASK_TIMEOUT_MS", opts.UserTaskTimeoutMs)
	b.SetInt32("SUSPEND_TASK_AFTER_NUM_FAILURES", opts.SuspendTaskAfterNumFailures)
	b.SetInt32("TASK_AUTO_RETRY_ATTEMPTS", opts.TaskAutoRetryAttempts)

	if opts.ErrorIntegration != nil {
		fmt.Fprintf(&b.Builder, " ERROR_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.ErrorIntegration))
	}

	if opts.SuccessIntegration != nil {
		fmt.Fprintf(&b.Builder, " SUCCESS_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.SuccessIntegration))
	}

	if opts.Config != nil {
		fmt.Fprintf(&b.Builder, " CONFIG = $$%s$$", *opts.Config)
	}

	if opts.Finalize != nil {
		fmt.Fprintf(&b.Builder, " FINALIZE = %s", sqlbuilder.QuoteIdentifier(*opts.Finalize))
	}

	b.SetString("LOG_LEVEL", opts.LogLevel)
	b.SetInt32("USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS", opts.UserTaskMinimumTriggerIntervalInSeconds)
	b.SetString("TARGET_COMPLETION_INTERVAL", opts.TargetCompletionInterval)
	b.SetString("SERVERLESS_TASK_MIN_STATEMENT_SIZE", opts.ServerlessTaskMinStatementSize)
	b.SetString("SERVERLESS_TASK_MAX_STATEMENT_SIZE", opts.ServerlessTaskMaxStatementSize)

	b.SetString("COMMENT", opts.Comment)

	if len(opts.After) > 0 {
		quoted := make([]string, len(opts.After))
		for i, a := range opts.After {
			quoted[i] = sqlbuilder.QuoteIdentifier(a)
		}

		fmt.Fprintf(&b.Builder, " AFTER %s", strings.Join(quoted, ", "))
	}

	if opts.When != nil {
		fmt.Fprintf(&b.Builder, " WHEN %s", *opts.When)
	}

	fmt.Fprintf(&b.Builder, " AS %s", opts.SQLStatement)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a task in Snowflake.
func (t *TaskClient) Create(ctx context.Context, opts CreateTaskOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create task options: %w", err))
	}

	sql, err := buildCreateTaskSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create task SQL: %w", err))
	}

	if _, err := t.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating task %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterTaskStatements builds the ALTER TASK SQL statements.
func buildAlterTaskStatements(opts AlterTaskOptions) (statements []string, err error) {

	// Suspend/resume must be separate statements.
	if opts.Suspend != nil {
		if *opts.Suspend {
			statements = append(statements, fmt.Sprintf("ALTER TASK %s SUSPEND", opts.Name.FullyQualifiedName()))
		} else {
			// Resume is appended last to ensure other changes are applied first.
			defer func() {
				statements = append(statements, fmt.Sprintf("ALTER TASK %s RESUME", opts.Name.FullyQualifiedName()))
			}()
		}
	}

	// Modify SQL statement.
	if opts.SQLStatement != nil {
		statements = append(statements, fmt.Sprintf("ALTER TASK %s MODIFY AS %s",
			opts.Name.FullyQualifiedName(), *opts.SQLStatement))
	}

	// Modify WHEN condition.
	if opts.When != nil {
		if *opts.When == "" {
			statements = append(statements, fmt.Sprintf("ALTER TASK %s MODIFY WHEN %s",
				opts.Name.FullyQualifiedName(), "TRUE"))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER TASK %s MODIFY WHEN %s",
				opts.Name.FullyQualifiedName(), *opts.When))
		}
	}

	// Modify predecessors.
	if len(opts.SetAfter) > 0 {
		quoted := make([]string, len(opts.SetAfter))
		for i, a := range opts.SetAfter {
			quoted[i] = sqlbuilder.QuoteIdentifier(a)
		}

		statements = append(statements, fmt.Sprintf("ALTER TASK %s ADD AFTER %s",
			opts.Name.FullyQualifiedName(), strings.Join(quoted, ", ")))
	}

	if len(opts.RemoveAfter) > 0 {
		quoted := make([]string, len(opts.RemoveAfter))
		for i, a := range opts.RemoveAfter {
			quoted[i] = sqlbuilder.QuoteIdentifier(a)
		}

		statements = append(statements, fmt.Sprintf("ALTER TASK %s REMOVE AFTER %s",
			opts.Name.FullyQualifiedName(), strings.Join(quoted, ", ")))
	}

	// Build SET clause for other parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)
	sc.Int32("USER_TASK_TIMEOUT_MS", opts.UserTaskTimeoutMs)
	sc.Int32("SUSPEND_TASK_AFTER_NUM_FAILURES", opts.SuspendTaskAfterNumFailures)
	sc.Int32("TASK_AUTO_RETRY_ATTEMPTS", opts.TaskAutoRetryAttempts)
	sc.Bool("ALLOW_OVERLAPPING_EXECUTION", opts.AllowOverlappingExecution)
	sc.String("LOG_LEVEL", opts.LogLevel)
	sc.Int32("USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS", opts.UserTaskMinimumTriggerIntervalInSeconds)
	sc.String("TARGET_COMPLETION_INTERVAL", opts.TargetCompletionInterval)
	sc.String("SERVERLESS_TASK_MIN_STATEMENT_SIZE", opts.ServerlessTaskMinStatementSize)
	sc.String("SERVERLESS_TASK_MAX_STATEMENT_SIZE", opts.ServerlessTaskMaxStatementSize)

	if opts.Warehouse != nil {
		sc.Keyword("WAREHOUSE", opts.Warehouse)
	}

	if opts.UserTaskManagedInitialWarehouseSize != nil {
		sc.String("USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE", opts.UserTaskManagedInitialWarehouseSize)
	}

	if opts.Schedule != nil {
		sc.String("SCHEDULE", opts.Schedule)
	}

	if opts.Config != nil {
		// CONFIG uses $$ delimiters, not single quotes.
		sc.UnsafeRaw(fmt.Sprintf("CONFIG = $$%s$$", *opts.Config)) //nolint:forbidigo // dollar-quoted; validated upstream
	}

	if opts.ErrorIntegration != nil {
		sc.Keyword("ERROR_INTEGRATION", opts.ErrorIntegration)
	}

	if opts.SuccessIntegration != nil {
		sc.Keyword("SUCCESS_INTEGRATION", opts.SuccessIntegration)
	}

	alterStmts, err := sqlbuilder.BuildAlterStatements("TASK", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	// FINALIZE requires separate ALTER statements.
	if opts.SetFinalize != nil {
		statements = append(statements, fmt.Sprintf("ALTER TASK %s SET FINALIZE = %s",
			opts.Name.FullyQualifiedName(), sqlbuilder.QuoteIdentifier(*opts.SetFinalize)))
	} else if opts.UnsetFinalize {
		statements = append(statements, fmt.Sprintf("ALTER TASK %s UNSET FINALIZE",
			opts.Name.FullyQualifiedName()))
	}

	return statements, nil
}

// Alter alters a task in Snowflake.
func (t *TaskClient) Alter(ctx context.Context, opts AlterTaskOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter task options: %w", err))
	}

	stmts, err := buildAlterTaskStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter task statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := t.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering task %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a task from Snowflake.
func (t *TaskClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("task name is required"))
	}

	if _, err := t.client.Exec(ctx, sqlbuilder.DropIfExists("TASK", name.FullyQualifiedName())); err != nil {
		return fmt.Errorf("dropping task %s: %w", name, err)
	}

	return nil
}

// buildShowTaskByIDSQL builds the SHOW TASKS LIKE SQL statement scoped to a schema.
func buildShowTaskByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("TASKS", name.Name(), scope)
}

// ShowByID queries SHOW TASKS for a specific task name within a schema.
func (t *TaskClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.TaskShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("task name is required"))
	}

	rows, err := t.client.Query(ctx, buildShowTaskByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing task %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanTaskShowOutput(rows, name.Name())
}

// Observe combines ShowByID and ShowParameters into a TaskObservation.
func (t *TaskClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*TaskObservation, error) {
	show, err := t.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &TaskObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := t.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &TaskObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

// scanTaskShowOutput scans SHOW TASKS results for a matching row.
func scanTaskShowOutput(rows *sql.Rows, name string) (*v1alpha1.TaskShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.TaskShowOutput, error) {
		return &v1alpha1.TaskShowOutput{
			CreatedOn:                 m["created_on"],
			Name:                      m["name"],
			DatabaseName:              m["database_name"],
			SchemaName:                m["schema_name"],
			Owner:                     m["owner"],
			Comment:                   m["comment"],
			Warehouse:                 m["warehouse"],
			Schedule:                  m["schedule"],
			State:                     m["state"],
			Definition:                m["definition"],
			Condition:                 m["condition"],
			Predecessors:              m["predecessors"],
			ErrorIntegration:          m["error_integration"],
			AllowOverlappingExecution: strings.EqualFold(m["allow_overlapping_execution"], "true"),
			Config:                    m["config"],
		}, nil
	})
}

// buildShowTaskParametersSQL builds the SHOW PARAMETERS IN TASK SQL statement.
func buildShowTaskParametersSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.ShowParameters("TASK", name.FullyQualifiedName())
}

// ShowParameters queries SHOW PARAMETERS IN TASK for a specific task.
func (t *TaskClient) ShowParameters(ctx context.Context, name SchemaObjectIdentifier) (*TaskParameters, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("task name is required"))
	}

	rows, err := t.client.Query(ctx, buildShowTaskParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing parameters for task %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanTaskParameters(rows)
}

// scanTaskParameters parses SHOW PARAMETERS results into TaskParameters.
func scanTaskParameters(rows *sql.Rows) (*TaskParameters, error) {
	return ScanParameters(rows, func(params *TaskParameters, key, val string) {
		switch key {
		case "USER_TASK_TIMEOUT_MS":
			if v, ok := parseInt32(val); ok {
				params.UserTaskTimeoutMs = &v
			}
		case "SUSPEND_TASK_AFTER_NUM_FAILURES":
			if v, ok := parseInt32(val); ok {
				params.SuspendTaskAfterNumFailures = &v
			}
		case "TASK_AUTO_RETRY_ATTEMPTS":
			if v, ok := parseInt32(val); ok {
				params.TaskAutoRetryAttempts = &v
			}
		case "LOG_LEVEL":
			params.LogLevel = val
		case "USER_TASK_MINIMUM_TRIGGER_INTERVAL_IN_SECONDS":
			if v, ok := parseInt32(val); ok {
				params.UserTaskMinimumTriggerIntervalInSeconds = &v
			}
		case "TARGET_COMPLETION_INTERVAL":
			params.TargetCompletionInterval = &val
		case "USER_TASK_MANAGED_INITIAL_WAREHOUSE_SIZE":
			params.UserTaskManagedInitialWarehouseSize = &val
		case "SERVERLESS_TASK_MIN_STATEMENT_SIZE":
			params.ServerlessTaskMinStatementSize = &val
		case "SERVERLESS_TASK_MAX_STATEMENT_SIZE":
			params.ServerlessTaskMaxStatementSize = &val
		}
	})
}
