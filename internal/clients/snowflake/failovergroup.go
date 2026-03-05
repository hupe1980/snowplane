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

// FailoverGroupObservation holds the result of observing a Snowflake Failover Group.
type FailoverGroupObservation struct {
	// Exists indicates whether the failover group was found.
	Exists bool

	// ShowOutput contains the SHOW FAILOVER GROUPS row.
	ShowOutput *v1alpha1.FailoverGroupShowOutput
}

// --- Valid values (exported for test assertions) ---

var validObjectTypes = map[string]bool{
	"ACCOUNT PARAMETERS": true, "DATABASES": true, "INTEGRATIONS": true,
	"NETWORK POLICIES": true, "RESOURCE MONITORS": true, "ROLES": true,
	"SHARES": true, "USERS": true, "WAREHOUSES": true,
}

var validIntegrationTypes = map[string]bool{
	"SECURITY INTEGRATIONS": true, "API INTEGRATIONS": true,
	"STORAGE INTEGRATIONS": true, "NOTIFICATION INTEGRATIONS": true,
}

// CreateFailoverGroupOptions holds the parameters for creating a failover group.
type CreateFailoverGroupOptions struct {
	Name                    AccountObjectIdentifier
	ObjectTypes             []string
	AllowedAccounts         []string
	AllowedDatabases        []string
	AllowedShares           []string
	AllowedIntegrationTypes []string
	IgnoreEditionCheck      *bool
	ReplicationSchedule     *string
	ErrorIntegration        *string
	Comment                 *string
}

// Validate checks that required fields are populated and values are valid.
func (o *CreateFailoverGroupOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if len(o.ObjectTypes) == 0 {
		errs = append(errs, errors.New("at least one object type is required"))
	}

	for _, ot := range o.ObjectTypes {
		if !validObjectTypes[strings.ToUpper(ot)] {
			errs = append(errs, fmt.Errorf("invalid object type: %q", ot))
		}
	}

	if len(o.AllowedAccounts) == 0 {
		errs = append(errs, errors.New("at least one allowed account is required"))
	}

	for _, it := range o.AllowedIntegrationTypes {
		if !validIntegrationTypes[strings.ToUpper(it)] {
			errs = append(errs, fmt.Errorf("invalid integration type: %q", it))
		}
	}

	return errors.Join(errs...)
}

func buildCreateFailoverGroupSQL(opts CreateFailoverGroupOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", err
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var b sqlbuilder.Builder
	fmt.Fprintf(&b.Builder, "CREATE FAILOVER GROUP %s", fqn)
	fmt.Fprintf(&b.Builder, "\n  OBJECT_TYPES = %s", strings.Join(opts.ObjectTypes, ", "))

	if len(opts.AllowedDatabases) > 0 {
		fmt.Fprintf(&b.Builder, "\n  ALLOWED_DATABASES = %s", strings.Join(opts.AllowedDatabases, ", "))
	}

	if len(opts.AllowedShares) > 0 {
		fmt.Fprintf(&b.Builder, "\n  ALLOWED_SHARES = %s", strings.Join(opts.AllowedShares, ", "))
	}

	if len(opts.AllowedIntegrationTypes) > 0 {
		fmt.Fprintf(&b.Builder, "\n  ALLOWED_INTEGRATION_TYPES = %s", strings.Join(opts.AllowedIntegrationTypes, ", "))
	}

	fmt.Fprintf(&b.Builder, "\n  ALLOWED_ACCOUNTS = %s", strings.Join(opts.AllowedAccounts, ", "))

	if opts.IgnoreEditionCheck != nil && *opts.IgnoreEditionCheck {
		fmt.Fprintf(&b.Builder, "\n  IGNORE EDITION CHECK")
	}

	b.SetString("REPLICATION_SCHEDULE", opts.ReplicationSchedule)

	if opts.ErrorIntegration != nil {
		fmt.Fprintf(&b.Builder, " ERROR_INTEGRATION = %s", *opts.ErrorIntegration)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// AlterFailoverGroupOptions holds the parameters for altering a failover group.
type AlterFailoverGroupOptions struct {
	Name                    AccountObjectIdentifier
	ObjectTypes             *[]string
	AllowedAccounts         *[]string
	AllowedDatabases        *[]string
	AllowedShares           *[]string
	AllowedIntegrationTypes *[]string
	ReplicationSchedule     *string
	ErrorIntegration        *string
	Comment                 *string

	// UnsetFields lists parameters to UNSET (e.g. "COMMENT", "REPLICATION_SCHEDULE").
	UnsetFields []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterFailoverGroupOptions) HasChanges() bool {
	return o.ObjectTypes != nil ||
		o.AllowedAccounts != nil ||
		o.AllowedDatabases != nil ||
		o.AllowedShares != nil ||
		o.AllowedIntegrationTypes != nil ||
		o.ReplicationSchedule != nil ||
		o.ErrorIntegration != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks that the alter options are valid.
func (o *AlterFailoverGroupOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.ObjectTypes != nil {
		for _, ot := range *o.ObjectTypes {
			if !validObjectTypes[strings.ToUpper(ot)] {
				errs = append(errs, fmt.Errorf("invalid object type: %q", ot))
			}
		}
	}

	if o.AllowedIntegrationTypes != nil {
		for _, it := range *o.AllowedIntegrationTypes {
			if !validIntegrationTypes[strings.ToUpper(it)] {
				errs = append(errs, fmt.Errorf("invalid integration type: %q", it))
			}
		}
	}

	return errors.Join(errs...)
}

func buildAlterFailoverGroupStatements(opts AlterFailoverGroupOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())
	var stmts []string

	// Each list-type field needs its own ALTER statement because Snowflake
	// processes them as separate SET operations.
	if opts.ObjectTypes != nil {
		stmt := fmt.Sprintf("ALTER FAILOVER GROUP %s SET\n  OBJECT_TYPES = %s", fqn, strings.Join(*opts.ObjectTypes, ", "))
		stmts = append(stmts, stmt)
	}

	if opts.AllowedDatabases != nil {
		stmt := fmt.Sprintf("ALTER FAILOVER GROUP %s SET\n  ALLOWED_DATABASES = %s", fqn, strings.Join(*opts.AllowedDatabases, ", "))
		stmts = append(stmts, stmt)
	}

	if opts.AllowedShares != nil {
		stmt := fmt.Sprintf("ALTER FAILOVER GROUP %s SET\n  ALLOWED_SHARES = %s", fqn, strings.Join(*opts.AllowedShares, ", "))
		stmts = append(stmts, stmt)
	}

	if opts.AllowedIntegrationTypes != nil {
		stmt := fmt.Sprintf("ALTER FAILOVER GROUP %s SET\n  ALLOWED_INTEGRATION_TYPES = %s", fqn, strings.Join(*opts.AllowedIntegrationTypes, ", "))
		stmts = append(stmts, stmt)
	}

	if opts.AllowedAccounts != nil {
		stmt := fmt.Sprintf("ALTER FAILOVER GROUP %s SET\n  ALLOWED_ACCOUNTS = %s", fqn, strings.Join(*opts.AllowedAccounts, ", "))
		stmts = append(stmts, stmt)
	}

	// Standard SET clauses (COMMENT, REPLICATION_SCHEDULE, ERROR_INTEGRATION)
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)
	sc.String("REPLICATION_SCHEDULE", opts.ReplicationSchedule)
	sc.Keyword("ERROR_INTEGRATION", opts.ErrorIntegration)

	setAlterStmts, err := sqlbuilder.BuildAlterStatements("FAILOVER GROUP", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, setAlterStmts...)

	return stmts, nil
}

// FailoverGroupClient operates on Snowflake failover groups.
type FailoverGroupClient struct {
	client SQLExecutor
}

// NewFailoverGroupClient creates a new FailoverGroupClient.
func NewFailoverGroupClient(client SQLExecutor) *FailoverGroupClient {
	return &FailoverGroupClient{client: client}
}

// Create creates a new failover group.
func (c *FailoverGroupClient) Create(ctx context.Context, opts CreateFailoverGroupOptions) error {
	stmt, err := buildCreateFailoverGroupSQL(opts)
	if err != nil {
		return fmt.Errorf("build CREATE FAILOVER GROUP: %w", err)
	}

	_, err = c.client.Exec(ctx, stmt)

	return err
}

// Alter modifies an existing failover group.
func (c *FailoverGroupClient) Alter(ctx context.Context, opts AlterFailoverGroupOptions) error {
	stmts, err := buildAlterFailoverGroupStatements(opts)
	if err != nil {
		return fmt.Errorf("build ALTER FAILOVER GROUP: %w", err)
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}

// Drop drops a failover group.
func (c *FailoverGroupClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("FAILOVER GROUP", sqlbuilder.QuoteIdentifier(name.Name()))

	_, err := c.client.Exec(ctx, stmt)

	return err
}

// ShowByID returns the SHOW FAILOVER GROUPS row for the given group name.
func (c *FailoverGroupClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.FailoverGroupShowOutput, error) {
	// SHOW FAILOVER GROUPS does not support LIKE, so we must filter client-side.
	rows, err := c.client.Query(ctx, "SHOW FAILOVER GROUPS")
	if err != nil {
		return nil, fmt.Errorf("SHOW FAILOVER GROUPS: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		out, err := scanFailoverGroupShowOutput(rows)
		if err != nil {
			return nil, err
		}

		if strings.EqualFold(out.Name, name.Name()) {
			return out, nil
		}
	}

	return nil, rows.Err()
}

func scanFailoverGroupShowOutput(rows *sql.Rows) (*v1alpha1.FailoverGroupShowOutput, error) {
	var (
		out                          v1alpha1.FailoverGroupShowOutput
		regionGroup, snowflakeRegion sql.NullString
		accountName                  sql.NullString
		isPrimary                    sql.NullBool
		allowedIntTypes              sql.NullString
		allowedAccounts              sql.NullString
		orgName, accountLocator      sql.NullString
		replSchedule                 sql.NullString
		secondaryState               sql.NullString
		nextRefresh                  sql.NullString
		isListingAutoFulfillment     sql.NullBool
	)

	err := rows.Scan(
		&regionGroup, &snowflakeRegion, &out.CreatedOn, &accountName,
		&out.Name, &out.Type, &out.Comment,
		&isPrimary, &out.Primary, &out.ObjectTypes,
		&allowedIntTypes, &allowedAccounts,
		&orgName, &accountLocator,
		&replSchedule, &secondaryState,
		&nextRefresh, &out.Owner,
		&isListingAutoFulfillment,
	)
	if err != nil {
		return nil, fmt.Errorf("scan SHOW FAILOVER GROUPS row: %w", err)
	}

	out.IsPrimary = isPrimary.Valid && isPrimary.Bool

	if allowedAccounts.Valid {
		out.AllowedAccounts = allowedAccounts.String
	}

	if replSchedule.Valid {
		out.ReplicationSchedule = replSchedule.String
	}

	return &out, nil
}

// Observe returns the current state of the failover group.
func (c *FailoverGroupClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*FailoverGroupObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		return nil, err
	}

	if show == nil {
		return &FailoverGroupObservation{Exists: false}, nil
	}

	return &FailoverGroupObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}
