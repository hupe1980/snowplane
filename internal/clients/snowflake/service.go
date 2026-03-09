package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ServiceObservation holds the result of observing a Snowflake service.
type ServiceObservation struct {
	// Exists indicates whether the service was found.
	Exists bool

	// ShowOutput contains the SHOW SERVICES row.
	ShowOutput *v1alpha1.ServiceShowOutput
}

// CreateServiceOptions holds the parameters for creating a service.
type CreateServiceOptions struct {
	Name                       SchemaObjectIdentifier
	ComputePool                string
	Specification              *string
	SpecificationReference     *string
	MinInstances               *int32
	MaxInstances               *int32
	AutoResume                 *bool
	ExternalAccessIntegrations []string
	Comment                    *string
}

// Validate checks the CreateServiceOptions for validity.
func (o *CreateServiceOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("service name is required")
	}

	if o.ComputePool == "" {
		return fmt.Errorf("compute pool is required")
	}

	if (o.Specification == nil) == (o.SpecificationReference == nil) {
		return fmt.Errorf("exactly one of specification or specificationReference must be set")
	}

	return nil
}

// AlterServiceOptions holds the parameters for altering a service.
type AlterServiceOptions struct {
	Name         SchemaObjectIdentifier
	MinInstances *int32
	MaxInstances *int32
	AutoResume   *bool
	Comment      *string
	UnsetFields  []string
}

// Validate checks the AlterServiceOptions for validity.
func (o *AlterServiceOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("service name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterServiceOptions) HasChanges() bool {
	return o.MinInstances != nil ||
		o.MaxInstances != nil ||
		o.AutoResume != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// ServiceClient provides operations against Snowflake services (SPCS).
type ServiceClient struct {
	client SQLExecutor
}

// NewServiceClient creates a new ServiceClient backed by the given SQLExecutor.
func NewServiceClient(c SQLExecutor) *ServiceClient {
	return &ServiceClient{client: c}
}

// buildShowServiceByIDSQL builds a SHOW SERVICES LIKE query scoped to a schema.
func buildShowServiceByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("SERVICES", name.Name(), scope)
}

// ShowByID queries SHOW SERVICES for a specific service within a schema.
func (c *ServiceClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.ServiceShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("service name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowServiceByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing service %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanServiceShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a ServiceObservation.
func (c *ServiceClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ServiceObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) || IsObjectNotExistOrNotAuthorized(err) {
			return &ServiceObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ServiceObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanServiceShowOutput scans SHOW SERVICES results for a matching row.
func scanServiceShowOutput(rows *sql.Rows, name string) (*v1alpha1.ServiceShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ServiceShowOutput, error) {
		minInstances, _ := parseInt32(m["min_instances"])
		maxInstances, _ := parseInt32(m["max_instances"])

		return &v1alpha1.ServiceShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			DatabaseName:   m["database_name"],
			SchemaName:     m["schema_name"],
			Owner:          m["owner"],
			ComputePool:    m["compute_pool"],
			Status:         m["status"],
			MinInstances:   minInstances,
			MaxInstances:   maxInstances,
			AutoResume:     m["auto_resume"],
			ResumeAt:       m["resume_at"],
			QueryWarehouse: m["query_warehouse"],
			Comment:        m["comment"],
		}, nil
	})
}

// Create creates a service in Snowflake.
func (c *ServiceClient) Create(ctx context.Context, opts CreateServiceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create service options: %w", err))
	}

	var b strings.Builder

	fmt.Fprintf(&b, "CREATE SERVICE %s", opts.Name.FullyQualifiedName())
	fmt.Fprintf(&b, "\n  IN COMPUTE POOL %s", sqlbuilder.QuoteIdentifier(opts.ComputePool))

	if opts.Specification != nil {
		fmt.Fprintf(&b, "\n  FROM SPECIFICATION $$\n%s\n$$", *opts.Specification)
	} else if opts.SpecificationReference != nil {
		if err := sqlbuilder.ValidateStageLocation(*opts.SpecificationReference); err != nil {
			return NewTerminalError(fmt.Errorf("invalid specification reference %q: %w", *opts.SpecificationReference, err))
		}

		fmt.Fprintf(&b, "\n  FROM %s", *opts.SpecificationReference)
	}

	if opts.MinInstances != nil {
		fmt.Fprintf(&b, "\n  MIN_INSTANCES = %d", *opts.MinInstances)
	}

	if opts.MaxInstances != nil {
		fmt.Fprintf(&b, "\n  MAX_INSTANCES = %d", *opts.MaxInstances)
	}

	if opts.AutoResume != nil {
		fmt.Fprintf(&b, "\n  AUTO_RESUME = %s", sqlbuilder.BoolToSQL(*opts.AutoResume))
	}

	if len(opts.ExternalAccessIntegrations) > 0 {
		var quoted []string
		for _, eai := range opts.ExternalAccessIntegrations {
			quoted = append(quoted, sqlbuilder.QuoteIdentifier(eai))
		}

		fmt.Fprintf(&b, "\n  EXTERNAL_ACCESS_INTEGRATIONS = (%s)", strings.Join(quoted, ", "))
	}

	if opts.Comment != nil {
		fmt.Fprintf(&b, "\n  COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment))
	}

	if _, err := c.client.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("creating service %s: %w", opts.Name, err)
	}

	return nil
}

// Alter modifies an existing service.
func (c *ServiceClient) Alter(ctx context.Context, opts AlterServiceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter service options: %w", err))
	}

	sc := &sqlbuilder.SetClauses{}

	if opts.MinInstances != nil {
		sc.Int32("MIN_INSTANCES", opts.MinInstances)
	}

	if opts.MaxInstances != nil {
		sc.Int32("MAX_INSTANCES", opts.MaxInstances)
	}

	if opts.AutoResume != nil {
		sc.Bool("AUTO_RESUME", opts.AutoResume)
	}

	if opts.Comment != nil {
		sc.String("COMMENT", opts.Comment)
	}

	stmts, err := sqlbuilder.BuildAlterStatements("SERVICE", opts.Name.FullyQualifiedName(), sc, opts.UnsetFields)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter service SQL: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering service %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop removes a service from Snowflake.
func (c *ServiceClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("service name is required"))
	}

	stmt := sqlbuilder.DropIfExists("SERVICE", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping service %s: %w", name, err)
	}

	return nil
}
