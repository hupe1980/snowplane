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

// validAPIProviders is the allowlist of API_PROVIDER values.
var validAPIProviders = map[string]bool{
	"aws_api_gateway":             true,
	"aws_private_api_gateway":     true,
	"aws_gov_api_gateway":         true,
	"aws_gov_private_api_gateway": true,
	"azure_api_management":        true,
	"google_api_gateway":          true,
	"git_https_api":               true,
}

// isAWSProvider returns true if the provider is an AWS variant.
func isAWSProvider(p string) bool {
	switch p {
	case "aws_api_gateway", "aws_private_api_gateway",
		"aws_gov_api_gateway", "aws_gov_private_api_gateway":
		return true
	}
	return false
}

// APIIntegrationObservation holds the result of observing a Snowflake API integration.
type APIIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW API INTEGRATIONS row.
	ShowOutput *v1alpha1.APIIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateAPIIntegrationOptions holds the parameters for creating an API integration.
type CreateAPIIntegrationOptions struct {
	Name               AccountObjectIdentifier
	APIProvider        string
	Enabled            *bool
	APIAllowedPrefixes []string
	APIBlockedPrefixes []string
	APIAWSRoleARN      *string
	AzureTenantID      *string
	AzureADAppID       *string
	GoogleAudience     *string
	APIKey             *string //nolint:gosec // G117: API key, not a secret credential
	Comment            *string
}

// Validate checks that required fields are populated and provider-specific requirements are met.
func (o *CreateAPIIntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.APIProvider == "" {
		errs = append(errs, errors.New("api_provider is required"))
	} else if !validAPIProviders[o.APIProvider] {
		errs = append(errs, fmt.Errorf("invalid api_provider %q", o.APIProvider))
	}

	if len(o.APIAllowedPrefixes) == 0 {
		errs = append(errs, errors.New("api_allowed_prefixes is required"))
	}

	// Provider-specific validation.
	if isAWSProvider(o.APIProvider) {
		if o.APIAWSRoleARN == nil || *o.APIAWSRoleARN == "" {
			errs = append(errs, fmt.Errorf("api_aws_role_arn is required for provider %s", o.APIProvider))
		}
	}

	if o.APIProvider == "azure_api_management" {
		if o.AzureTenantID == nil || *o.AzureTenantID == "" {
			errs = append(errs, errors.New("azure_tenant_id is required for provider azure_api_management"))
		}

		if o.AzureADAppID == nil || *o.AzureADAppID == "" {
			errs = append(errs, errors.New("azure_ad_application_id is required for provider azure_api_management"))
		}
	}

	if o.APIProvider == "google_api_gateway" {
		if o.GoogleAudience == nil || *o.GoogleAudience == "" {
			errs = append(errs, errors.New("google_audience is required for provider google_api_gateway"))
		}
	}

	return errors.Join(errs...)
}

// AlterAPIIntegrationOptions holds the parameters for altering an API integration.
type AlterAPIIntegrationOptions struct {
	Name               AccountObjectIdentifier
	Enabled            *bool
	APIAllowedPrefixes *[]string
	APIBlockedPrefixes *[]string
	APIAWSRoleARN      *string
	AzureADAppID       *string
	APIKey             *string //nolint:gosec // G117: API key, not a secret credential
	Comment            *string
	UnsetFields        []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterAPIIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.APIAllowedPrefixes != nil ||
		o.APIBlockedPrefixes != nil ||
		o.APIAWSRoleARN != nil ||
		o.AzureADAppID != nil ||
		o.APIKey != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterAPIIntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.APIAllowedPrefixes != nil && len(*o.APIAllowedPrefixes) == 0 {
		errs = append(errs, errors.New("api_allowed_prefixes cannot be set to empty"))
	}

	return errors.Join(errs...)
}

// APIIntegrationClient provides operations on Snowflake API integrations.
type APIIntegrationClient struct {
	client SQLExecutor
}

// NewAPIIntegrationClient creates a new APIIntegrationClient.
func NewAPIIntegrationClient(c SQLExecutor) *APIIntegrationClient {
	return &APIIntegrationClient{client: c}
}

// buildCreateAPIIntegrationSQL builds the CREATE API INTEGRATION SQL statement.
func buildCreateAPIIntegrationSQL(opts CreateAPIIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "API INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " API_PROVIDER = %s", opts.APIProvider)

	// Provider-specific fields.
	if opts.APIAWSRoleARN != nil {
		b.SetString("API_AWS_ROLE_ARN", opts.APIAWSRoleARN)
	}

	if opts.AzureTenantID != nil {
		b.SetString("AZURE_TENANT_ID", opts.AzureTenantID)
	}

	if opts.AzureADAppID != nil {
		b.SetString("AZURE_AD_APPLICATION_ID", opts.AzureADAppID)
	}

	if opts.GoogleAudience != nil {
		b.SetString("GOOGLE_AUDIENCE", opts.GoogleAudience)
	}

	// Required list field.
	b.WriteString(" ")
	b.WriteString(buildStringListClause("API_ALLOWED_PREFIXES", opts.APIAllowedPrefixes))

	// Optional list field.
	if len(opts.APIBlockedPrefixes) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("API_BLOCKED_PREFIXES", opts.APIBlockedPrefixes))
	}

	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("API_KEY", opts.APIKey)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an API integration in Snowflake.
func (c *APIIntegrationClient) Create(ctx context.Context, opts CreateAPIIntegrationOptions) error {
	stmt, err := buildCreateAPIIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create API integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating API integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAPIIntegrationStatements builds ALTER API INTEGRATION statements.
func buildAlterAPIIntegrationStatements(opts AlterAPIIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.APIAllowedPrefixes != nil {
		sc.UnsafeRaw(buildStringListClause("API_ALLOWED_PREFIXES", *opts.APIAllowedPrefixes))
	}

	if opts.APIBlockedPrefixes != nil {
		sc.UnsafeRaw(buildStringListClause("API_BLOCKED_PREFIXES", *opts.APIBlockedPrefixes))
	}

	sc.String("API_AWS_ROLE_ARN", opts.APIAWSRoleARN)
	sc.String("AZURE_AD_APPLICATION_ID", opts.AzureADAppID)
	sc.String("API_KEY", opts.APIKey)
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("API INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an API integration in Snowflake.
func (c *APIIntegrationClient) Alter(ctx context.Context, opts AlterAPIIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter API integration options: %w", err))
	}

	stmts, err := buildAlterAPIIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter API integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering API integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an API integration.
func (c *APIIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("API INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping API integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves an API integration from Snowflake.
func (c *APIIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.APIIntegrationShowOutput, error) {
	stmt := sqlbuilder.ShowLike("API INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing API integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanAPIIntegrationShowOutput(rows, name.Name())
}

// scanAPIIntegrationShowOutput scans SHOW output rows into APIIntegrationShowOutput.
func scanAPIIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.APIIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.APIIntegrationShowOutput, error) {
		return &v1alpha1.APIIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed API integration properties.
func (c *APIIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing API integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *APIIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*APIIntegrationObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &APIIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &APIIntegrationObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
