package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// WarehouseObservation holds the result of observing a Snowflake warehouse.
type WarehouseObservation struct {
	Exists     bool
	ShowOutput *WarehouseShowOutput
	Parameters *WarehouseParameters
}

// WarehouseShowOutput holds the result of a SHOW WAREHOUSES query.
type WarehouseShowOutput struct {
	CreatedOn       string
	Name            string
	State           string
	Type            string
	Size            string
	Comment         string
	Owner           string
	AutoSuspend     int32
	AutoResume      bool
	MinClusterCount int32
	MaxClusterCount int32
	ScalingPolicy   string
	ResourceMonitor string
}

// WarehouseParameters holds the result of SHOW PARAMETERS IN WAREHOUSE.
type WarehouseParameters struct {
	MaxConcurrencyLevel             *int32
	StatementQueuedTimeoutInSeconds *int32
	StatementTimeoutInSeconds       *int32
	EnableQueryAcceleration         *bool
	QueryAccelerationMaxScaleFactor *int32
}

// CreateWarehouseOptions holds options for CREATE WAREHOUSE.
type CreateWarehouseOptions struct {
	Name                            AccountObjectIdentifier
	WarehouseType                   *string
	WarehouseSize                   *string
	MinClusterCount                 *int32
	MaxClusterCount                 *int32
	ScalingPolicy                   *string
	AutoSuspend                     *int32
	AutoResume                      *bool
	InitiallySuspended              bool
	ResourceMonitor                 *string
	Comment                         *string
	EnableQueryAcceleration         *bool
	QueryAccelerationMaxScaleFactor *int32
	MaxConcurrencyLevel             *int32
	StatementQueuedTimeoutInSeconds *int32
	StatementTimeoutInSeconds       *int32
	ResourceConstraint              *string

	// UseCreateOrAlter emits CREATE OR ALTER WAREHOUSE instead of
	// CREATE WAREHOUSE IF NOT EXISTS. Requires Snowflake 2024+ support.
	UseCreateOrAlter bool
}

// Validate validates the create options.
func (o *CreateWarehouseOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("%w: warehouse name is required", ErrInvalidIdentifier)
	}

	if o.QueryAccelerationMaxScaleFactor != nil {
		if *o.QueryAccelerationMaxScaleFactor < 0 || *o.QueryAccelerationMaxScaleFactor > 100 {
			return fmt.Errorf("queryAccelerationMaxScaleFactor must be between 0 and 100, got %d", *o.QueryAccelerationMaxScaleFactor)
		}
	}

	if o.MinClusterCount != nil && o.MaxClusterCount != nil {
		if *o.MinClusterCount > *o.MaxClusterCount {
			return fmt.Errorf("minClusterCount (%d) must be <= maxClusterCount (%d)", *o.MinClusterCount, *o.MaxClusterCount)
		}
	}

	return nil
}

// AlterWarehouseOptions holds options for ALTER WAREHOUSE.
type AlterWarehouseOptions struct {
	Name                            AccountObjectIdentifier
	WarehouseType                   *string
	WarehouseSize                   *string
	MinClusterCount                 *int32
	MaxClusterCount                 *int32
	ScalingPolicy                   *string
	AutoSuspend                     *int32
	AutoResume                      *bool
	ResourceMonitor                 *string
	Comment                         *string
	EnableQueryAcceleration         *bool
	QueryAccelerationMaxScaleFactor *int32
	MaxConcurrencyLevel             *int32
	StatementQueuedTimeoutInSeconds *int32
	StatementTimeoutInSeconds       *int32
	ResourceConstraint              *string
	UnsetFields                     []string
}

// Validate validates the alter options.
func (o *AlterWarehouseOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("%w: warehouse name is required", ErrInvalidIdentifier)
	}

	if o.QueryAccelerationMaxScaleFactor != nil {
		if *o.QueryAccelerationMaxScaleFactor < 0 || *o.QueryAccelerationMaxScaleFactor > 100 {
			return fmt.Errorf("queryAccelerationMaxScaleFactor must be between 0 and 100, got %d", *o.QueryAccelerationMaxScaleFactor)
		}
	}

	if o.MinClusterCount != nil && o.MaxClusterCount != nil {
		if *o.MinClusterCount > *o.MaxClusterCount {
			return fmt.Errorf("minClusterCount (%d) must be <= maxClusterCount (%d)", *o.MinClusterCount, *o.MaxClusterCount)
		}
	}

	return nil
}

// HasChanges returns true if any field is set for alteration.
func (o *AlterWarehouseOptions) HasChanges() bool {
	return o.WarehouseType != nil ||
		o.WarehouseSize != nil ||
		o.MinClusterCount != nil ||
		o.MaxClusterCount != nil ||
		o.ScalingPolicy != nil ||
		o.AutoSuspend != nil ||
		o.AutoResume != nil ||
		o.ResourceMonitor != nil ||
		o.Comment != nil ||
		o.EnableQueryAcceleration != nil ||
		o.QueryAccelerationMaxScaleFactor != nil ||
		o.MaxConcurrencyLevel != nil ||
		o.StatementQueuedTimeoutInSeconds != nil ||
		o.StatementTimeoutInSeconds != nil ||
		o.ResourceConstraint != nil ||
		len(o.UnsetFields) > 0
}

// WarehouseClient wraps a Snowflake client for warehouse operations.
type WarehouseClient struct {
	client SQLExecutor
}

// NewWarehouseClient creates a new WarehouseClient backed by the given SQLExecutor.
func NewWarehouseClient(c SQLExecutor) *WarehouseClient {
	return &WarehouseClient{client: c}
}

func buildCreateWarehouseSQL(opts CreateWarehouseOptions) string {
	var b sqlbuilder.Builder

	if opts.UseCreateOrAlter {
		b.WriteString("CREATE OR ALTER WAREHOUSE ")
	} else {
		b.WriteString("CREATE WAREHOUSE IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())

	b.SetString("WAREHOUSE_TYPE", opts.WarehouseType)
	b.SetString("WAREHOUSE_SIZE", opts.WarehouseSize)
	b.SetInt32("MIN_CLUSTER_COUNT", opts.MinClusterCount)
	b.SetInt32("MAX_CLUSTER_COUNT", opts.MaxClusterCount)
	b.SetString("SCALING_POLICY", opts.ScalingPolicy)
	b.SetInt32("AUTO_SUSPEND", opts.AutoSuspend)
	b.SetBool("AUTO_RESUME", opts.AutoResume)

	if opts.InitiallySuspended {
		b.WriteString(" INITIALLY_SUSPENDED = TRUE")
	}

	b.SetString("RESOURCE_MONITOR", opts.ResourceMonitor)
	b.SetString("COMMENT", opts.Comment)
	b.SetBool("ENABLE_QUERY_ACCELERATION", opts.EnableQueryAcceleration)
	b.SetInt32("QUERY_ACCELERATION_MAX_SCALE_FACTOR", opts.QueryAccelerationMaxScaleFactor)
	b.SetInt32("MAX_CONCURRENCY_LEVEL", opts.MaxConcurrencyLevel)
	b.SetInt32("STATEMENT_QUEUED_TIMEOUT_IN_SECONDS", opts.StatementQueuedTimeoutInSeconds)
	b.SetInt32("STATEMENT_TIMEOUT_IN_SECONDS", opts.StatementTimeoutInSeconds)
	b.SetString("RESOURCE_CONSTRAINT", opts.ResourceConstraint)

	return b.String()
}

// Create creates a warehouse in Snowflake.
func (w *WarehouseClient) Create(ctx context.Context, opts CreateWarehouseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create warehouse options: %w", err))
	}

	sql := buildCreateWarehouseSQL(opts)
	if _, err := w.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating warehouse %s: %w", opts.Name, err)
	}

	return nil
}

func buildAlterWarehouseStatements(opts AlterWarehouseOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("WAREHOUSE_TYPE", opts.WarehouseType)
	sc.String("WAREHOUSE_SIZE", opts.WarehouseSize)
	sc.Int32("MIN_CLUSTER_COUNT", opts.MinClusterCount)
	sc.Int32("MAX_CLUSTER_COUNT", opts.MaxClusterCount)
	sc.String("SCALING_POLICY", opts.ScalingPolicy)
	sc.Int32("AUTO_SUSPEND", opts.AutoSuspend)
	sc.Bool("AUTO_RESUME", opts.AutoResume)
	sc.String("RESOURCE_MONITOR", opts.ResourceMonitor)
	sc.String("COMMENT", opts.Comment)
	sc.Bool("ENABLE_QUERY_ACCELERATION", opts.EnableQueryAcceleration)
	sc.Int32("QUERY_ACCELERATION_MAX_SCALE_FACTOR", opts.QueryAccelerationMaxScaleFactor)
	sc.Int32("MAX_CONCURRENCY_LEVEL", opts.MaxConcurrencyLevel)
	sc.Int32("STATEMENT_QUEUED_TIMEOUT_IN_SECONDS", opts.StatementQueuedTimeoutInSeconds)
	sc.Int32("STATEMENT_TIMEOUT_IN_SECONDS", opts.StatementTimeoutInSeconds)
	sc.String("RESOURCE_CONSTRAINT", opts.ResourceConstraint)

	return sqlbuilder.BuildAlterStatements("WAREHOUSE", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a warehouse in Snowflake.
func (w *WarehouseClient) Alter(ctx context.Context, opts AlterWarehouseOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter warehouse options: %w", err))
	}

	stmts, err := buildAlterWarehouseStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter warehouse statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := w.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering warehouse %s: %w", opts.Name, err)
		}
	}

	return nil
}

func buildDropWarehouseSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.DropIfExists("WAREHOUSE", name.FullyQualifiedName())
}

// Drop drops a warehouse from Snowflake.
func (w *WarehouseClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("%w: warehouse name is required", ErrInvalidIdentifier))
	}

	if _, err := w.client.Exec(ctx, buildDropWarehouseSQL(name)); err != nil {
		return fmt.Errorf("dropping warehouse %s: %w", name, err)
	}

	return nil
}

func buildShowWarehouseByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("WAREHOUSES", name.Name())
}

// ShowByID retrieves a warehouse by name using SHOW WAREHOUSES.
func (w *WarehouseClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*WarehouseShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("%w: warehouse name is required", ErrInvalidIdentifier))
	}

	rows, err := w.client.Query(ctx, buildShowWarehouseByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing warehouse %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanWarehouseShowOutput(rows, name.Name())
}

func buildShowWarehouseParametersSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowParameters("WAREHOUSE", name.FullyQualifiedName())
}

// ShowParameters retrieves warehouse parameters.
func (w *WarehouseClient) ShowParameters(ctx context.Context, name AccountObjectIdentifier) (*WarehouseParameters, error) {
	rows, err := w.client.Query(ctx, buildShowWarehouseParametersSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing warehouse parameters %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanWarehouseParameters(rows)
}

// Observe observes the current state of a warehouse.
func (w *WarehouseClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*WarehouseObservation, error) {
	show, err := w.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &WarehouseObservation{Exists: false}, nil
		}

		return nil, err
	}

	params, err := w.ShowParameters(ctx, name)
	if err != nil {
		return nil, err
	}

	return &WarehouseObservation{
		Exists:     true,
		ShowOutput: show,
		Parameters: params,
	}, nil
}

func scanWarehouseShowOutput(rows *sql.Rows, name string) (*WarehouseShowOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, c := range cols {
			if values[i].Valid {
				colMap[c] = values[i].String
			}
		}

		rowName := colMap["name"]
		if !strings.EqualFold(rowName, name) {
			continue
		}

		autoSuspend, _ := parseInt32(colMap["auto_suspend"])
		minCluster, _ := parseInt32(colMap["min_cluster_count"])
		maxCluster, _ := parseInt32(colMap["max_cluster_count"])

		return &WarehouseShowOutput{
			CreatedOn:       colMap["created_on"],
			Name:            rowName,
			State:           colMap["state"],
			Type:            colMap["type"],
			Size:            colMap["size"],
			Comment:         colMap["comment"],
			Owner:           colMap["owner"],
			AutoSuspend:     autoSuspend,
			AutoResume:      strings.EqualFold(colMap["auto_resume"], "true"),
			MinClusterCount: minCluster,
			MaxClusterCount: maxCluster,
			ScalingPolicy:   colMap["scaling_policy"],
			ResourceMonitor: colMap["resource_monitor"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, fmt.Errorf("%w: warehouse %q", ErrObjectNotFound, name)
}

func scanWarehouseParameters(rows *sql.Rows) (*WarehouseParameters, error) {
	params := &WarehouseParameters{}

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting parameter columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning parameter row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, c := range cols {
			if values[i].Valid {
				colMap[c] = values[i].String
			}
		}

		key := colMap["key"]
		value := colMap["value"]

		switch key {
		case "MAX_CONCURRENCY_LEVEL":
			if v, ok := parseInt32(value); ok {
				params.MaxConcurrencyLevel = &v
			}
		case "STATEMENT_QUEUED_TIMEOUT_IN_SECONDS":
			if v, ok := parseInt32(value); ok {
				params.StatementQueuedTimeoutInSeconds = &v
			}
		case "STATEMENT_TIMEOUT_IN_SECONDS":
			if v, ok := parseInt32(value); ok {
				params.StatementTimeoutInSeconds = &v
			}
		case "ENABLE_QUERY_ACCELERATION":
			b := strings.EqualFold(value, "true")
			params.EnableQueryAcceleration = &b
		case "QUERY_ACCELERATION_MAX_SCALE_FACTOR":
			if v, ok := parseInt32(value); ok {
				params.QueryAccelerationMaxScaleFactor = &v
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parameter rows: %w", err)
	}

	return params, nil
}
