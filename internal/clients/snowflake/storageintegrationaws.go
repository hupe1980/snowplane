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

// StorageIntegrationAWSObservation holds the result of observing an AWS storage integration.
type StorageIntegrationAWSObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW INTEGRATIONS row.
	ShowOutput *v1alpha1.StorageIntegrationAWSShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateStorageIntegrationAWSOptions holds the parameters for creating an AWS storage integration.
type CreateStorageIntegrationAWSOptions struct {
	Name                    AccountObjectIdentifier
	Enabled                 *bool
	StorageAllowedLocations []string
	StorageBlockedLocations []string
	StorageAWSRoleARN       string
	StorageAWSExternalID    *string
	Comment                 *string
}

// Validate checks the options for validity.
func (o *CreateStorageIntegrationAWSOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("storage integration name is required"))
	}

	if len(o.StorageAllowedLocations) == 0 {
		errs = append(errs, fmt.Errorf("at least one storage allowed location is required"))
	}

	if o.StorageAWSRoleARN == "" {
		errs = append(errs, fmt.Errorf("storageAWSRoleARN is required for AWS storage integrations"))
	}

	return errors.Join(errs...)
}

// AlterStorageIntegrationAWSOptions holds the parameters for altering an AWS storage integration.
type AlterStorageIntegrationAWSOptions struct {
	Name                    AccountObjectIdentifier
	Enabled                 *bool
	StorageAllowedLocations *[]string
	StorageBlockedLocations *[]string
	StorageAWSRoleARN       *string
	StorageAWSExternalID    *string
	Comment                 *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the options for validity.
func (o *AlterStorageIntegrationAWSOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("storage integration name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStorageIntegrationAWSOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.StorageAllowedLocations != nil ||
		o.StorageBlockedLocations != nil ||
		o.StorageAWSRoleARN != nil ||
		o.StorageAWSExternalID != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// StorageIntegrationAWSClient provides operations against AWS storage integrations.
type StorageIntegrationAWSClient struct {
	client SQLExecutor
}

// NewStorageIntegrationAWSClient creates a new client.
func NewStorageIntegrationAWSClient(c SQLExecutor) *StorageIntegrationAWSClient {
	return &StorageIntegrationAWSClient{client: c}
}

// buildCreateStorageIntegrationAWSSQL builds the CREATE STORAGE INTEGRATION SQL.
func buildCreateStorageIntegrationAWSSQL(opts CreateStorageIntegrationAWSOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = 'EXTERNAL_STAGE'")
	fmt.Fprintf(&b.Builder, " STORAGE_PROVIDER = 'S3'")
	fmt.Fprintf(&b.Builder, " STORAGE_ALLOWED_LOCATIONS = %s", buildLocationList(opts.StorageAllowedLocations))

	if len(opts.StorageBlockedLocations) > 0 {
		fmt.Fprintf(&b.Builder, " STORAGE_BLOCKED_LOCATIONS = %s", buildLocationList(opts.StorageBlockedLocations))
	}

	fmt.Fprintf(&b.Builder, " STORAGE_AWS_ROLE_ARN = '%s'", sqlbuilder.EscapeString(opts.StorageAWSRoleARN))

	if opts.StorageAWSExternalID != nil {
		fmt.Fprintf(&b.Builder, " STORAGE_AWS_EXTERNAL_ID = '%s'", sqlbuilder.EscapeString(*opts.StorageAWSExternalID))
	}

	if opts.Enabled != nil {
		b.SetBool("ENABLED", opts.Enabled)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an AWS storage integration in Snowflake.
func (c *StorageIntegrationAWSClient) Create(ctx context.Context, opts CreateStorageIntegrationAWSOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create storage integration options: %w", err))
	}

	sql, err := buildCreateStorageIntegrationAWSSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create storage integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating storage integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStorageIntegrationAWSStatements builds ALTER STORAGE INTEGRATION statements.
func buildAlterStorageIntegrationAWSStatements(opts AlterStorageIntegrationAWSOptions) ([]string, error) {
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

	if opts.StorageAWSRoleARN != nil {
		sc.String("STORAGE_AWS_ROLE_ARN", opts.StorageAWSRoleARN)
	}

	if opts.StorageAWSExternalID != nil {
		sc.String("STORAGE_AWS_EXTERNAL_ID", opts.StorageAWSExternalID)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("STORAGE INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an AWS storage integration in Snowflake.
func (c *StorageIntegrationAWSClient) Alter(ctx context.Context, opts AlterStorageIntegrationAWSOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter storage integration options: %w", err))
	}

	stmts, err := buildAlterStorageIntegrationAWSStatements(opts)
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

// Drop drops an AWS storage integration from Snowflake.
func (c *StorageIntegrationAWSClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
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
func (c *StorageIntegrationAWSClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.StorageIntegrationAWSShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("STORAGE INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing storage integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStorageIntegrationAWSShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs.
func (c *StorageIntegrationAWSClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
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

// Observe combines ShowByID and Describe into a StorageIntegrationAWSObservation.
func (c *StorageIntegrationAWSClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*StorageIntegrationAWSObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StorageIntegrationAWSObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		return &StorageIntegrationAWSObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &StorageIntegrationAWSObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanStorageIntegrationAWSShowOutput scans SHOW STORAGE INTEGRATIONS results.
func scanStorageIntegrationAWSShowOutput(rows *sql.Rows, name string) (*v1alpha1.StorageIntegrationAWSShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.StorageIntegrationAWSShowOutput, error) {
		return &v1alpha1.StorageIntegrationAWSShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}
