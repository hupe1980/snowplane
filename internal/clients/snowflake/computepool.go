package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ComputePoolObservation holds the result of observing a Snowflake compute pool.
type ComputePoolObservation struct {
	// Exists indicates whether the compute pool was found.
	Exists bool

	// ShowOutput contains the SHOW COMPUTE POOLS row.
	ShowOutput *v1alpha1.ComputePoolShowOutput
}

// CreateComputePoolOptions holds the parameters for creating a compute pool.
type CreateComputePoolOptions struct {
	Name            AccountObjectIdentifier
	MinNodes        int32
	MaxNodes        int32
	InstanceFamily  string
	AutoResume      *bool
	AutoSuspendSecs *int32
	Comment         *string
}

// Validate checks the CreateComputePoolOptions for validity.
func (o *CreateComputePoolOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("compute pool name is required")
	}

	if o.MinNodes < 1 {
		return fmt.Errorf("minNodes must be at least 1")
	}

	if o.MaxNodes < 1 {
		return fmt.Errorf("maxNodes must be at least 1")
	}

	if o.MinNodes > o.MaxNodes {
		return fmt.Errorf("minNodes (%d) must be <= maxNodes (%d)", o.MinNodes, o.MaxNodes)
	}

	if o.InstanceFamily == "" {
		return fmt.Errorf("instance family is required")
	}

	return nil
}

// AlterComputePoolOptions holds the parameters for altering a compute pool.
type AlterComputePoolOptions struct {
	Name            AccountObjectIdentifier
	MinNodes        *int32
	MaxNodes        *int32
	AutoResume      *bool
	AutoSuspendSecs *int32
	Comment         *string
	UnsetFields     []string
}

// Validate checks the AlterComputePoolOptions for validity.
func (o *AlterComputePoolOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("compute pool name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterComputePoolOptions) HasChanges() bool {
	return o.MinNodes != nil ||
		o.MaxNodes != nil ||
		o.AutoResume != nil ||
		o.AutoSuspendSecs != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// ComputePoolClient provides operations against Snowflake compute pools.
type ComputePoolClient struct {
	client SQLExecutor
}

// NewComputePoolClient creates a new ComputePoolClient backed by the given SQLExecutor.
func NewComputePoolClient(c SQLExecutor) *ComputePoolClient {
	return &ComputePoolClient{client: c}
}

// buildShowComputePoolByIDSQL builds a SHOW COMPUTE POOLS LIKE SQL statement.
func buildShowComputePoolByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("COMPUTE POOLS", name.Name())
}

// ShowByID queries SHOW COMPUTE POOLS for a specific compute pool.
func (c *ComputePoolClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.ComputePoolShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("compute pool name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowComputePoolByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing compute pool %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanComputePoolShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a ComputePoolObservation.
func (c *ComputePoolClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*ComputePoolObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) || IsObjectNotExistOrNotAuthorized(err) {
			return &ComputePoolObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ComputePoolObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanComputePoolShowOutput scans SHOW COMPUTE POOLS results for a matching row.
func scanComputePoolShowOutput(rows *sql.Rows, name string) (*v1alpha1.ComputePoolShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ComputePoolShowOutput, error) {
		minNodes, _ := parseInt32(m["min_nodes"])
		maxNodes, _ := parseInt32(m["max_nodes"])
		numServices, _ := parseInt32(m["num_services"])
		numJobs, _ := parseInt32(m["num_jobs"])
		autoSuspend, _ := parseInt32(m["auto_suspend_secs"])
		activeNodes, _ := parseInt32(m["active_nodes"])
		idleNodes, _ := parseInt32(m["idle_nodes"])

		return &v1alpha1.ComputePoolShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			State:          m["state"],
			MinNodes:       minNodes,
			MaxNodes:       maxNodes,
			InstanceFamily: m["instance_family"],
			NumServices:    numServices,
			NumJobs:        numJobs,
			AutoResume:     m["auto_resume"],
			AutoSuspend:    autoSuspend,
			ActiveNodes:    activeNodes,
			IdleNodes:      idleNodes,
			Owner:          m["owner"],
			Comment:        m["comment"],
		}, nil
	})
}

// Create creates a compute pool in Snowflake.
func (c *ComputePoolClient) Create(ctx context.Context, opts CreateComputePoolOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create compute pool options: %w", err))
	}

	var b strings.Builder

	fmt.Fprintf(&b, "CREATE COMPUTE POOL %s", opts.Name.FullyQualifiedName())
	fmt.Fprintf(&b, " MIN_NODES = %d", opts.MinNodes)
	fmt.Fprintf(&b, " MAX_NODES = %d", opts.MaxNodes)
	if err := sqlbuilder.ValidateKeywordValue(opts.InstanceFamily); err != nil {
		return NewTerminalError(fmt.Errorf("invalid instance family %q: %w", opts.InstanceFamily, err))
	}

	fmt.Fprintf(&b, " INSTANCE_FAMILY = %s", opts.InstanceFamily)

	if opts.AutoResume != nil {
		fmt.Fprintf(&b, " AUTO_RESUME = %t", *opts.AutoResume)
	}

	if opts.AutoSuspendSecs != nil {
		fmt.Fprintf(&b, " AUTO_SUSPEND_SECS = %d", *opts.AutoSuspendSecs)
	}

	if opts.Comment != nil {
		fmt.Fprintf(&b, " COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment))
	}

	if _, err := c.client.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("creating compute pool %s: %w", opts.Name, err)
	}

	return nil
}

// Alter modifies an existing compute pool.
func (c *ComputePoolClient) Alter(ctx context.Context, opts AlterComputePoolOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter compute pool options: %w", err))
	}

	sc := &sqlbuilder.SetClauses{}

	if opts.MinNodes != nil {
		sc.Int32("MIN_NODES", opts.MinNodes)
	}

	if opts.MaxNodes != nil {
		sc.Int32("MAX_NODES", opts.MaxNodes)
	}

	if opts.AutoResume != nil {
		sc.Bool("AUTO_RESUME", opts.AutoResume)
	}

	if opts.AutoSuspendSecs != nil {
		sc.Int32("AUTO_SUSPEND_SECS", opts.AutoSuspendSecs)
	}

	if opts.Comment != nil {
		sc.String("COMMENT", opts.Comment)
	}

	stmts, err := sqlbuilder.BuildAlterStatements("COMPUTE POOL", opts.Name.FullyQualifiedName(), sc, opts.UnsetFields)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter compute pool SQL: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering compute pool %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop removes a compute pool from Snowflake.
func (c *ComputePoolClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("compute pool name is required"))
	}

	stmt := sqlbuilder.DropIfExists("COMPUTE POOL", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping compute pool %s: %w", name, err)
	}

	return nil
}
