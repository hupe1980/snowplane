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

// StorageIntegrationAzureObservation holds the result of observing an Azure storage integration.
type StorageIntegrationAzureObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW INTEGRATIONS row.
	ShowOutput *v1alpha1.StorageIntegrationAzureShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateStorageIntegrationAzureOptions holds the parameters for creating an Azure storage integration.
type CreateStorageIntegrationAzureOptions struct {
	Name                    AccountObjectIdentifier
	Enabled                 *bool
	StorageAllowedLocations []string
	StorageBlockedLocations []string
	AzureTenantID           string
	Comment                 *string
}

// Validate checks the options for validity.
func (o *CreateStorageIntegrationAzureOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("storage integration name is required"))
	}

	if len(o.StorageAllowedLocations) == 0 {
		errs = append(errs, fmt.Errorf("at least one storage allowed location is required"))
	}

	if o.AzureTenantID == "" {
		errs = append(errs, fmt.Errorf("azureTenantID is required for Azure storage integrations"))
	}

	return errors.Join(errs...)
}

// AlterStorageIntegrationAzureOptions holds the parameters for altering an Azure storage integration.
type AlterStorageIntegrationAzureOptions struct {
	Name                    AccountObjectIdentifier
	Enabled                 *bool
	StorageAllowedLocations *[]string
	StorageBlockedLocations *[]string
	AzureTenantID           *string
	Comment                 *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the options for validity.
func (o *AlterStorageIntegrationAzureOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("storage integration name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStorageIntegrationAzureOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.StorageAllowedLocations != nil ||
		o.StorageBlockedLocations != nil ||
		o.AzureTenantID != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// StorageIntegrationAzureClient provides operations against Azure storage integrations.
type StorageIntegrationAzureClient struct {
	client SQLExecutor
}

// NewStorageIntegrationAzureClient creates a new client.
func NewStorageIntegrationAzureClient(c SQLExecutor) *StorageIntegrationAzureClient {
	return &StorageIntegrationAzureClient{client: c}
}

// buildCreateStorageIntegrationAzureSQL builds the CREATE STORAGE INTEGRATION SQL.
func buildCreateStorageIntegrationAzureSQL(opts CreateStorageIntegrationAzureOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = 'EXTERNAL_STAGE'")
	fmt.Fprintf(&b.Builder, " STORAGE_PROVIDER = 'AZURE'")
	fmt.Fprintf(&b.Builder, " STORAGE_ALLOWED_LOCATIONS = %s", buildLocationList(opts.StorageAllowedLocations))

	if len(opts.StorageBlockedLocations) > 0 {
		fmt.Fprintf(&b.Builder, " STORAGE_BLOCKED_LOCATIONS = %s", buildLocationList(opts.StorageBlockedLocations))
	}

	fmt.Fprintf(&b.Builder, " AZURE_TENANT_ID = '%s'", sqlbuilder.EscapeString(opts.AzureTenantID))

	if opts.Enabled != nil {
		b.SetBool("ENABLED", opts.Enabled)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an Azure storage integration in Snowflake.
func (c *StorageIntegrationAzureClient) Create(ctx context.Context, opts CreateStorageIntegrationAzureOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create storage integration options: %w", err))
	}

	sql, err := buildCreateStorageIntegrationAzureSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create storage integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating storage integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStorageIntegrationAzureStatements builds ALTER STORAGE INTEGRATION statements.
func buildAlterStorageIntegrationAzureStatements(opts AlterStorageIntegrationAzureOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.Enabled != nil {
		sc.Bool("ENABLED", opts.Enabled)
	}

	if opts.StorageAllowedLocations != nil {
		sc.UnsafeRaw("STORAGE_ALLOWED_LOCATIONS = " + buildLocationList(*opts.StorageAllowedLocations)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.StorageBlockedLocations != nil {
		sc.UnsafeRaw("STORAGE_BLOCKED_LOCATIONS = " + buildLocationList(*opts.StorageBlockedLocations)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.AzureTenantID != nil {
		sc.String("AZURE_TENANT_ID", opts.AzureTenantID)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("STORAGE INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an Azure storage integration in Snowflake.
func (c *StorageIntegrationAzureClient) Alter(ctx context.Context, opts AlterStorageIntegrationAzureOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter storage integration options: %w", err))
	}

	stmts, err := buildAlterStorageIntegrationAzureStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter storage integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering storage integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an Azure storage integration from Snowflake.
func (c *StorageIntegrationAzureClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	stmt := sqlbuilder.DropIfExists("STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping storage integration %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW STORAGE INTEGRATIONS for a specific integration.
func (c *StorageIntegrationAzureClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.StorageIntegrationAzureShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("STORAGE INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing storage integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStorageIntegrationAzureShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs.
func (c *StorageIntegrationAzureClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing storage integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a StorageIntegrationAzureObservation.
func (c *StorageIntegrationAzureClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*StorageIntegrationAzureObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StorageIntegrationAzureObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		return &StorageIntegrationAzureObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &StorageIntegrationAzureObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanStorageIntegrationAzureShowOutput scans SHOW STORAGE INTEGRATIONS results.
func scanStorageIntegrationAzureShowOutput(rows *sql.Rows, name string) (*v1alpha1.StorageIntegrationAzureShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.StorageIntegrationAzureShowOutput, error) {
		return &v1alpha1.StorageIntegrationAzureShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}
