package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// AlertObservation holds the result of observing a Snowflake alert.
type AlertObservation struct {
	// Exists indicates whether the alert was found.
	Exists bool

	// ShowOutput contains the SHOW ALERTS row.
	ShowOutput *AlertShowOutput
}

// AlertShowOutput contains the fields from SHOW ALERTS.
type AlertShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
	Warehouse    string
	Schedule     string
	State        string // started, suspended
	Condition    string
	Action       string
}

// CreateAlertOptions holds the parameters for creating an alert.
type CreateAlertOptions struct {
	Name      SchemaObjectIdentifier
	Warehouse *string
	Schedule  *string
	Comment   *string
	Condition string
	Action    string
}

// Validate checks the CreateAlertOptions for validity.
func (o *CreateAlertOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("alert name is required"))
	}

	if o.Condition == "" {
		errs = append(errs, fmt.Errorf("condition is required"))
	}

	if o.Action == "" {
		errs = append(errs, fmt.Errorf("action is required"))
	}

	return errors.Join(errs...)
}

// AlterAlertOptions holds the parameters for altering an alert.
type AlterAlertOptions struct {
	Name      SchemaObjectIdentifier
	Warehouse *string
	Schedule  *string
	Comment   *string
	Condition *string
	Action    *string
	Suspend   *bool

	// CurrentState is the observed state of the alert (e.g. "started", "suspended").
	// When the alert is running and condition/action/schedule/warehouse are being
	// modified, the alter logic will auto-suspend before modification and
	// auto-resume afterward (unless Suspend is explicitly set to true).
	CurrentState string

	// UnsetFields lists Snowflake parameter names to revert to defaults.
	UnsetFields []string
}

// Validate checks the AlterAlertOptions for validity.
func (o *AlterAlertOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("alert name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterAlertOptions) HasChanges() bool {
	return o.Warehouse != nil ||
		o.Schedule != nil ||
		o.Comment != nil ||
		o.Condition != nil ||
		o.Action != nil ||
		o.Suspend != nil ||
		len(o.UnsetFields) > 0
}

// AlertClient provides operations against Snowflake alerts.
type AlertClient struct {
	client SQLExecutor
}

// NewAlertClient creates a new AlertClient backed by the given SQLExecutor.
func NewAlertClient(c SQLExecutor) *AlertClient {
	return &AlertClient{client: c}
}

// buildCreateAlertSQL builds the CREATE ALERT SQL statement.
func buildCreateAlertSQL(opts CreateAlertOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "ALERT", opts.Name.FullyQualifiedName(), false, false)

	if opts.Warehouse != nil {
		fmt.Fprintf(&b.Builder, " WAREHOUSE = %s", sqlbuilder.QuoteIdentifier(*opts.Warehouse))
	}

	if opts.Schedule != nil {
		fmt.Fprintf(&b.Builder, " SCHEDULE = '%s'", sqlbuilder.EscapeString(*opts.Schedule))
	}

	b.SetString("COMMENT", opts.Comment)

	fmt.Fprintf(&b.Builder, " IF( EXISTS( %s )) THEN %s", opts.Condition, opts.Action)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an alert in Snowflake.
func (a *AlertClient) Create(ctx context.Context, opts CreateAlertOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create alert options: %w", err))
	}

	sql, err := buildCreateAlertSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create alert SQL: %w", err))
	}

	if _, err := a.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating alert %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAlertStatements builds the ALTER ALERT SQL statements.
// Implements the suspend-before-modify pattern: if the alert is currently
// running ("started") and any of condition/action/schedule/warehouse are
// being modified, the alert is suspended first, then modified, then resumed
// (unless the user explicitly requests suspension via Suspend=true).
func buildAlterAlertStatements(opts AlterAlertOptions) (statements []string, _ error) {
	fqn := opts.Name.FullyQualifiedName()

	// Determine if we need to auto-suspend for safe modification.
	isRunning := strings.EqualFold(opts.CurrentState, "started")
	needsModify := opts.Condition != nil || opts.Action != nil || opts.Schedule != nil || opts.Warehouse != nil
	userWantsSuspend := opts.Suspend != nil && *opts.Suspend
	userWantsResume := opts.Suspend != nil && !*opts.Suspend
	autoSuspend := isRunning && needsModify && !userWantsSuspend

	// Step 1: Suspend if explicitly requested or needed for safe modification.
	if userWantsSuspend || autoSuspend {
		statements = append(statements, fmt.Sprintf("ALTER ALERT %s SUSPEND", fqn))
	}

	// Step 2: Apply modifications (condition, action, SET/UNSET).
	if opts.Condition != nil {
		statements = append(statements, fmt.Sprintf("ALTER ALERT %s MODIFY CONDITION EXISTS (%s)",
			fqn, *opts.Condition))
	}

	if opts.Action != nil {
		statements = append(statements, fmt.Sprintf("ALTER ALERT %s MODIFY ACTION %s",
			fqn, *opts.Action))
	}

	// Build SET clause for other parameters.
	var sc sqlbuilder.SetClauses

	sc.String("COMMENT", opts.Comment)

	if opts.Warehouse != nil {
		sc.Keyword("WAREHOUSE", opts.Warehouse)
	}

	if opts.Schedule != nil {
		sc.String("SCHEDULE", opts.Schedule)
	}

	alterStmts, err := sqlbuilder.BuildAlterStatements("ALERT", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	// Step 3: Resume if explicitly requested or if we auto-suspended.
	if userWantsResume || (autoSuspend && !userWantsSuspend) {
		statements = append(statements, fmt.Sprintf("ALTER ALERT %s RESUME", fqn))
	}

	return statements, nil
}

// Alter alters an alert in Snowflake.
func (a *AlertClient) Alter(ctx context.Context, opts AlterAlertOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter alert options: %w", err))
	}

	stmts, err := buildAlterAlertStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter alert statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := a.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering alert %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an alert from Snowflake.
func (a *AlertClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("alert name is required"))
	}

	if _, err := a.client.Exec(ctx, sqlbuilder.DropIfExists("ALERT", name.FullyQualifiedName())); err != nil {
		return fmt.Errorf("dropping alert %s: %w", name, err)
	}

	return nil
}

// buildShowAlertByIDSQL builds the SHOW ALERTS LIKE SQL statement scoped to a schema.
func buildShowAlertByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("ALERTS", name.Name(), scope)
}

// ShowByID queries SHOW ALERTS for a specific alert name within a schema.
func (a *AlertClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*AlertShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("alert name is required"))
	}

	rows, err := a.client.Query(ctx, buildShowAlertByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing alert %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanAlertShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an AlertObservation.
func (a *AlertClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*AlertObservation, error) {
	show, err := a.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &AlertObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &AlertObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanAlertShowOutput scans SHOW ALERTS results for a matching row.
func scanAlertShowOutput(rows *sql.Rows, name string) (*AlertShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*AlertShowOutput, error) {
		return &AlertShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Comment:      m["comment"],
			Warehouse:    m["warehouse"],
			Schedule:     m["schedule"],
			State:        m["state"],
			Condition:    m["condition"],
			Action:       m["action"],
		}, nil
	})
}
