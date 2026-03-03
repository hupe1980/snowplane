package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ResourceMonitorObservation holds the result of observing a Snowflake resource monitor.
type ResourceMonitorObservation struct {
	// Exists indicates whether the resource monitor was found.
	Exists bool

	// ShowOutput contains the SHOW RESOURCE MONITORS row.
	ShowOutput *ResourceMonitorShowOutput
}

// ResourceMonitorShowOutput contains the fields from SHOW RESOURCE MONITORS.
type ResourceMonitorShowOutput struct {
	CreatedOn            string
	Name                 string
	CreditQuota          string
	UsedCredits          string
	RemainingCredits     string
	Level                string
	Frequency            string
	StartTime            string
	EndTime              string
	NotifyAt             string
	SuspendAt            string
	SuspendImmediatelyAt string
	NotifyUsers          string
}

// ResourceMonitorTrigger defines a trigger for the resource monitor.
type ResourceMonitorTrigger struct {
	// Threshold is the percentage of the credit quota.
	Threshold int32

	// Action is the action: SUSPEND, SUSPEND_IMMEDIATE, or NOTIFY.
	Action string
}

// CreateResourceMonitorOptions holds the parameters for creating a resource monitor.
type CreateResourceMonitorOptions struct {
	Name           AccountObjectIdentifier
	CreditQuota    *int32
	Frequency      *string
	StartTimestamp *string
	EndTimestamp   *string
	NotifyUsers    []string
	Triggers       []ResourceMonitorTrigger
}

// Validate checks the CreateResourceMonitorOptions for validity.
func (o *CreateResourceMonitorOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("resource monitor name is required")
	}

	return nil
}

// AlterResourceMonitorOptions holds the parameters for altering a resource monitor.
type AlterResourceMonitorOptions struct {
	Name           AccountObjectIdentifier
	CreditQuota    *int32
	Frequency      *string
	StartTimestamp *string
	EndTimestamp   *string
	NotifyUsers    *[]string
	Triggers       *[]ResourceMonitorTrigger

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterResourceMonitorOptions for validity.
func (o *AlterResourceMonitorOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("resource monitor name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterResourceMonitorOptions) HasChanges() bool {
	return o.CreditQuota != nil ||
		o.Frequency != nil ||
		o.StartTimestamp != nil ||
		o.EndTimestamp != nil ||
		o.NotifyUsers != nil ||
		o.Triggers != nil ||
		len(o.UnsetFields) > 0
}

// ResourceMonitorClient provides operations against Snowflake resource monitors.
type ResourceMonitorClient struct {
	client SQLExecutor
}

// NewResourceMonitorClient creates a new ResourceMonitorClient backed by the given SQLExecutor.
func NewResourceMonitorClient(c SQLExecutor) *ResourceMonitorClient {
	return &ResourceMonitorClient{client: c}
}

// buildTriggersClause builds a TRIGGERS clause for CREATE or ALTER.
func buildTriggersClause(triggers []ResourceMonitorTrigger) string {
	if len(triggers) == 0 {
		return ""
	}

	parts := make([]string, len(triggers))
	for i, t := range triggers {
		parts[i] = fmt.Sprintf("ON %d PERCENT DO %s", t.Threshold, t.Action)
	}

	return "TRIGGERS " + strings.Join(parts, " ")
}

// buildNotifyUsersClause builds a NOTIFY_USERS clause.
func buildNotifyUsersClause(users []string) string {
	if len(users) == 0 {
		return ""
	}

	return fmt.Sprintf("NOTIFY_USERS = (%s)", strings.Join(users, ", "))
}

// buildCreateResourceMonitorSQL builds the CREATE RESOURCE MONITOR SQL statement.
func buildCreateResourceMonitorSQL(opts CreateResourceMonitorOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "RESOURCE MONITOR", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	b.WriteString(" WITH")

	if opts.CreditQuota != nil {
		b.SetInt32("CREDIT_QUOTA", opts.CreditQuota)
	}

	if opts.Frequency != nil {
		b.SetKeyword("FREQUENCY", opts.Frequency)
	}

	if opts.StartTimestamp != nil {
		v := *opts.StartTimestamp
		if strings.EqualFold(v, "IMMEDIATELY") {
			b.WriteString(" START_TIMESTAMP = IMMEDIATELY")
		} else {
			fmt.Fprintf(&b, " START_TIMESTAMP = '%s'", sqlbuilder.EscapeString(v)) //nolint:errcheck // strings.Builder.Write never returns an error
		}
	}

	if opts.EndTimestamp != nil {
		fmt.Fprintf(&b, " END_TIMESTAMP = '%s'", sqlbuilder.EscapeString(*opts.EndTimestamp)) //nolint:errcheck // strings.Builder.Write never returns an error
	}

	if len(opts.NotifyUsers) > 0 {
		b.WriteString(" ")
		b.WriteString(buildNotifyUsersClause(opts.NotifyUsers))
	}

	if len(opts.Triggers) > 0 {
		b.WriteString(" ")
		b.WriteString(buildTriggersClause(opts.Triggers))
	}

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a resource monitor in Snowflake.
func (rm *ResourceMonitorClient) Create(ctx context.Context, opts CreateResourceMonitorOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create resource monitor options: %w", err))
	}

	sql, err := buildCreateResourceMonitorSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create resource monitor SQL: %w", err))
	}

	if _, err := rm.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating resource monitor %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterResourceMonitorSQL builds the ALTER RESOURCE MONITOR SQL statement.
func buildAlterResourceMonitorSQL(opts AlterResourceMonitorOptions) string {
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var parts []string

	// Build SET clause for scalar fields.
	var setClauses []string

	if opts.CreditQuota != nil {
		setClauses = append(setClauses, fmt.Sprintf("CREDIT_QUOTA = %d", *opts.CreditQuota))
	}

	if opts.Frequency != nil {
		setClauses = append(setClauses, fmt.Sprintf("FREQUENCY = %s", *opts.Frequency))
	}

	if opts.StartTimestamp != nil {
		v := *opts.StartTimestamp
		if strings.EqualFold(v, "IMMEDIATELY") {
			setClauses = append(setClauses, "START_TIMESTAMP = IMMEDIATELY")
		} else {
			setClauses = append(setClauses, fmt.Sprintf("START_TIMESTAMP = '%s'", sqlbuilder.EscapeString(v)))
		}
	}

	if opts.EndTimestamp != nil {
		setClauses = append(setClauses, fmt.Sprintf("END_TIMESTAMP = '%s'", sqlbuilder.EscapeString(*opts.EndTimestamp)))
	}

	if opts.NotifyUsers != nil {
		setClauses = append(setClauses, buildNotifyUsersClause(*opts.NotifyUsers))
	}

	if len(setClauses) > 0 {
		parts = append(parts, "SET "+strings.Join(setClauses, " "))
	}

	// TRIGGERS is a separate clause (not part of SET).
	if opts.Triggers != nil {
		tc := buildTriggersClause(*opts.Triggers)
		if tc != "" {
			parts = append(parts, tc)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("ALTER RESOURCE MONITOR %s %s", fqn, strings.Join(parts, " "))
}

// Alter alters a resource monitor in Snowflake.
func (rm *ResourceMonitorClient) Alter(ctx context.Context, opts AlterResourceMonitorOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter resource monitor options: %w", err))
	}

	stmt := buildAlterResourceMonitorSQL(opts)
	if stmt == "" {
		return nil
	}

	if _, err := rm.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("altering resource monitor %s: %w", opts.Name, err)
	}

	return nil
}

// Drop drops a resource monitor from Snowflake.
func (rm *ResourceMonitorClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("resource monitor name is required"))
	}

	stmt := sqlbuilder.DropIfExists("RESOURCE MONITOR", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := rm.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping resource monitor %s: %w", name, err)
	}

	return nil
}

// buildShowResourceMonitorByIDSQL builds a SHOW RESOURCE MONITORS LIKE SQL statement.
func buildShowResourceMonitorByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("RESOURCE MONITORS", name.Name())
}

// ShowByID queries SHOW RESOURCE MONITORS for a specific monitor.
func (rm *ResourceMonitorClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*ResourceMonitorShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("resource monitor name is required"))
	}

	rows, err := rm.client.Query(ctx, buildShowResourceMonitorByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing resource monitor %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanResourceMonitorShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a ResourceMonitorObservation.
func (rm *ResourceMonitorClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*ResourceMonitorObservation, error) {
	show, err := rm.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ResourceMonitorObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ResourceMonitorObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanResourceMonitorShowOutput scans SHOW RESOURCE MONITORS results for a matching row.
func scanResourceMonitorShowOutput(rows *sql.Rows, name string) (*ResourceMonitorShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*ResourceMonitorShowOutput, error) {
		return &ResourceMonitorShowOutput{
			CreatedOn:            m["created_on"],
			Name:                 m["name"],
			CreditQuota:          m["credit_quota"],
			UsedCredits:          m["used_credits"],
			RemainingCredits:     m["remaining_credits"],
			Level:                m["level"],
			Frequency:            m["frequency"],
			StartTime:            m["start_time"],
			EndTime:              m["end_time"],
			NotifyAt:             m["notify_at"],
			SuspendAt:            m["suspend_at"],
			SuspendImmediatelyAt: m["suspend_immediately_at"],
			NotifyUsers:          m["notify_users"],
		}, nil
	})
}
