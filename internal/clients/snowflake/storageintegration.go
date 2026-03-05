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

// StorageIntegrationObservation holds the result of observing a Snowflake storage integration.
type StorageIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW INTEGRATIONS row.
	ShowOutput *v1alpha1.StorageIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateStorageIntegrationOptions holds the parameters for creating a storage integration.
type CreateStorageIntegrationOptions struct {
	Name                    AccountObjectIdentifier
	Type                    string // EXTERNAL_STAGE
	Enabled                 *bool
	StorageProvider         string // S3, GCS, AZURE
	StorageAllowedLocations []string
	StorageBlockedLocations []string
	StorageAWSRoleARN       *string
	StorageAWSExternalID    *string
	AzureTenantID           *string
	Comment                 *string
}

// Validate checks the CreateStorageIntegrationOptions for validity.
func (o *CreateStorageIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("storage integration name is required"))
	}

	if o.StorageProvider == "" {
		errs = append(errs, fmt.Errorf("storage provider is required"))
	}

	if len(o.StorageAllowedLocations) == 0 {
		errs = append(errs, fmt.Errorf("at least one storage allowed location is required"))
	}

	if o.StorageProvider == "S3" && (o.StorageAWSRoleARN == nil || *o.StorageAWSRoleARN == "") {
		errs = append(errs, fmt.Errorf("storageAWSRoleARN is required for S3 provider"))
	}

	if o.StorageProvider == "AZURE" && (o.AzureTenantID == nil || *o.AzureTenantID == "") {
		errs = append(errs, fmt.Errorf("azureTenantID is required for AZURE provider"))
	}

	return errors.Join(errs...)
}

// AlterStorageIntegrationOptions holds the parameters for altering a storage integration.
type AlterStorageIntegrationOptions struct {
	Name                    AccountObjectIdentifier
	Enabled                 *bool
	StorageAllowedLocations *[]string
	StorageBlockedLocations *[]string
	StorageAWSRoleARN       *string
	StorageAWSExternalID    *string
	AzureTenantID           *string
	Comment                 *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterStorageIntegrationOptions for validity.
func (o *AlterStorageIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("storage integration name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStorageIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.StorageAllowedLocations != nil ||
		o.StorageBlockedLocations != nil ||
		o.StorageAWSRoleARN != nil ||
		o.StorageAWSExternalID != nil ||
		o.AzureTenantID != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// StorageIntegrationClient provides operations against Snowflake storage integrations.
type StorageIntegrationClient struct {
	client SQLExecutor
}

// NewStorageIntegrationClient creates a new StorageIntegrationClient.
func NewStorageIntegrationClient(c SQLExecutor) *StorageIntegrationClient {
	return &StorageIntegrationClient{client: c}
}

// buildLocationList formats a list of URIs for Snowflake SQL, e.g. ('s3://bucket/path/', 's3://bucket/other/').
func buildLocationList(locs []string) string {
	quoted := make([]string, len(locs))
	for i, loc := range locs {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(loc))
	}

	return fmt.Sprintf("(%s)", strings.Join(quoted, ", "))
}

// buildCreateStorageIntegrationSQL builds the CREATE STORAGE INTEGRATION SQL statement.
func buildCreateStorageIntegrationSQL(opts CreateStorageIntegrationOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = '%s'", opts.Type)
	fmt.Fprintf(&b.Builder, " STORAGE_PROVIDER = '%s'", opts.StorageProvider)
	fmt.Fprintf(&b.Builder, " STORAGE_ALLOWED_LOCATIONS = %s", buildLocationList(opts.StorageAllowedLocations))

	if len(opts.StorageBlockedLocations) > 0 {
		fmt.Fprintf(&b.Builder, " STORAGE_BLOCKED_LOCATIONS = %s", buildLocationList(opts.StorageBlockedLocations))
	}

	if opts.StorageAWSRoleARN != nil {
		fmt.Fprintf(&b.Builder, " STORAGE_AWS_ROLE_ARN = '%s'", sqlbuilder.EscapeString(*opts.StorageAWSRoleARN))
	}

	if opts.StorageAWSExternalID != nil {
		fmt.Fprintf(&b.Builder, " STORAGE_AWS_EXTERNAL_ID = '%s'", sqlbuilder.EscapeString(*opts.StorageAWSExternalID))
	}

	if opts.AzureTenantID != nil {
		fmt.Fprintf(&b.Builder, " AZURE_TENANT_ID = '%s'", sqlbuilder.EscapeString(*opts.AzureTenantID))
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

// Create creates a storage integration in Snowflake.
func (si *StorageIntegrationClient) Create(ctx context.Context, opts CreateStorageIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create storage integration options: %w", err))
	}

	sql, err := buildCreateStorageIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create storage integration SQL: %w", err))
	}

	if _, err := si.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating storage integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStorageIntegrationStatements builds ALTER STORAGE INTEGRATION statements.
func buildAlterStorageIntegrationStatements(opts AlterStorageIntegrationOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.Enabled != nil {
		sc.Bool("ENABLED", opts.Enabled)
	}

	if opts.StorageAllowedLocations != nil {
		sc.UnsafeRaw("STORAGE_ALLOWED_LOCATIONS = " + buildLocationList(*opts.StorageAllowedLocations))
	}

	if opts.StorageBlockedLocations != nil {
		sc.UnsafeRaw("STORAGE_BLOCKED_LOCATIONS = " + buildLocationList(*opts.StorageBlockedLocations))
	}

	if opts.StorageAWSRoleARN != nil {
		sc.String("STORAGE_AWS_ROLE_ARN", opts.StorageAWSRoleARN)
	}

	if opts.StorageAWSExternalID != nil {
		sc.String("STORAGE_AWS_EXTERNAL_ID", opts.StorageAWSExternalID)
	}

	if opts.AzureTenantID != nil {
		sc.String("AZURE_TENANT_ID", opts.AzureTenantID)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("STORAGE INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a storage integration in Snowflake.
func (si *StorageIntegrationClient) Alter(ctx context.Context, opts AlterStorageIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter storage integration options: %w", err))
	}

	stmts, err := buildAlterStorageIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter storage integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := si.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering storage integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a storage integration from Snowflake.
func (si *StorageIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	stmt := sqlbuilder.DropIfExists("STORAGE INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := si.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping storage integration %s: %w", name, err)
	}

	return nil
}

// buildShowStorageIntegrationByIDSQL builds the SHOW SQL for a specific integration.
func buildShowStorageIntegrationByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("STORAGE INTEGRATIONS", name.Name())
}

// ShowByID queries SHOW STORAGE INTEGRATIONS for a specific integration.
func (si *StorageIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.StorageIntegrationShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	rows, err := si.client.Query(ctx, buildShowStorageIntegrationByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing storage integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStorageIntegrationShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs of properties.
func (si *StorageIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("storage integration name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := si.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing storage integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a StorageIntegrationObservation.
func (si *StorageIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*StorageIntegrationObservation, error) {
	show, err := si.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StorageIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := si.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, we still have partial info.
		// Return what we have with a nil DescribeOutput.
		return &StorageIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &StorageIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanStorageIntegrationShowOutput scans SHOW STORAGE INTEGRATIONS results.
func scanStorageIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.StorageIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.StorageIntegrationShowOutput, error) {
		return &v1alpha1.StorageIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}
