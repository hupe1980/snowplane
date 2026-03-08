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

// ExternalOAuthIntegrationObservation holds the result of observing a Snowflake External OAuth security integration.
type ExternalOAuthIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW SECURITY INTEGRATIONS row.
	ShowOutput *v1alpha1.ExternalOAuthIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateExternalOAuthIntegrationOptions holds the parameters for creating an External OAuth security integration.
type CreateExternalOAuthIntegrationOptions struct {
	Name                          AccountObjectIdentifier
	Enabled                       *bool
	ExternalOAuthType             string
	Issuer                        string
	TokenUserMappingClaim         string
	SnowflakeUserMappingAttribute string
	JWSKeysURL                    *string
	AudienceList                  []string
	AllowedRoles                  []string
	BlockedRoles                  []string
	AnyRoleMode                   *string
	ScopeDelimiter                *string
	NetworkPolicy                 *string
	Comment                       *string
}

// Validate checks that required fields are populated.
func (o *CreateExternalOAuthIntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.ExternalOAuthType == "" {
		errs = append(errs, errors.New("external_oauth_type is required"))
	} else if !validExternalOAuthTypes[o.ExternalOAuthType] {
		errs = append(errs, fmt.Errorf("invalid external_oauth_type %q", o.ExternalOAuthType))
	}

	if o.Issuer == "" {
		errs = append(errs, errors.New("external_oauth_issuer is required"))
	}

	if o.TokenUserMappingClaim == "" {
		errs = append(errs, errors.New("external_oauth_token_user_mapping_claim is required"))
	}

	if o.SnowflakeUserMappingAttribute == "" {
		errs = append(errs, errors.New("external_oauth_snowflake_user_mapping_attribute is required"))
	}

	if o.AnyRoleMode != nil && !validExternalOAuthAnyRoleModes[*o.AnyRoleMode] {
		errs = append(errs, fmt.Errorf("invalid external_oauth_any_role_mode %q", *o.AnyRoleMode))
	}

	return errors.Join(errs...)
}

// AlterExternalOAuthIntegrationOptions holds the parameters for altering an External OAuth security integration.
type AlterExternalOAuthIntegrationOptions struct {
	Name                  AccountObjectIdentifier
	Enabled               *bool
	TokenUserMappingClaim *string
	JWSKeysURL            *string
	AudienceList          *[]string
	AllowedRoles          *[]string
	BlockedRoles          *[]string
	AnyRoleMode           *string
	ScopeDelimiter        *string
	NetworkPolicy         *string
	Comment               *string
	UnsetFields           []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterExternalOAuthIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.TokenUserMappingClaim != nil ||
		o.JWSKeysURL != nil ||
		o.AudienceList != nil ||
		o.AllowedRoles != nil ||
		o.BlockedRoles != nil ||
		o.AnyRoleMode != nil ||
		o.ScopeDelimiter != nil ||
		o.NetworkPolicy != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterExternalOAuthIntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.AnyRoleMode != nil && !validExternalOAuthAnyRoleModes[*o.AnyRoleMode] {
		errs = append(errs, fmt.Errorf("invalid external_oauth_any_role_mode %q", *o.AnyRoleMode))
	}

	return errors.Join(errs...)
}

// ExternalOAuthIntegrationClient provides operations on Snowflake External OAuth security integrations.
type ExternalOAuthIntegrationClient struct {
	client SQLExecutor
}

// NewExternalOAuthIntegrationClient creates a new ExternalOAuthIntegrationClient.
func NewExternalOAuthIntegrationClient(c SQLExecutor) *ExternalOAuthIntegrationClient {
	return &ExternalOAuthIntegrationClient{client: c}
}

// buildCreateExternalOAuthIntegrationSQL builds the CREATE SECURITY INTEGRATION SQL for External OAuth.
func buildCreateExternalOAuthIntegrationSQL(opts CreateExternalOAuthIntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = EXTERNAL_OAUTH")
	fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_TYPE = %s", opts.ExternalOAuthType)
	b.SetString("EXTERNAL_OAUTH_ISSUER", &opts.Issuer)
	fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = '%s'", sqlbuilder.EscapeString(opts.TokenUserMappingClaim))
	fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = '%s'", sqlbuilder.EscapeString(opts.SnowflakeUserMappingAttribute))

	if opts.JWSKeysURL != nil {
		b.SetString("EXTERNAL_OAUTH_JWS_KEYS_URL", opts.JWSKeysURL)
	}

	if len(opts.AudienceList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("EXTERNAL_OAUTH_AUDIENCE_LIST", opts.AudienceList))
	}

	if len(opts.AllowedRoles) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", opts.AllowedRoles))
	}

	if len(opts.BlockedRoles) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", opts.BlockedRoles))
	}

	if opts.AnyRoleMode != nil {
		fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_ANY_ROLE_MODE = %s", *opts.AnyRoleMode)
	}

	if opts.ScopeDelimiter != nil {
		b.SetString("EXTERNAL_OAUTH_SCOPE_DELIMITER", opts.ScopeDelimiter)
	}

	if opts.NetworkPolicy != nil {
		b.SetString("NETWORK_POLICY", opts.NetworkPolicy)
	}

	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an External OAuth security integration in Snowflake.
func (c *ExternalOAuthIntegrationClient) Create(ctx context.Context, opts CreateExternalOAuthIntegrationOptions) error {
	stmt, err := buildCreateExternalOAuthIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create External OAuth integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating External OAuth integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterExternalOAuthIntegrationStatements builds ALTER SECURITY INTEGRATION statements for External OAuth.
func buildAlterExternalOAuthIntegrationStatements(opts AlterExternalOAuthIntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.TokenUserMappingClaim != nil {
		sc.UnsafeRaw(fmt.Sprintf("EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = '%s'", sqlbuilder.EscapeString(*opts.TokenUserMappingClaim))) //nolint:forbidigo // value escaped via EscapeString
	}

	if opts.JWSKeysURL != nil {
		sc.String("EXTERNAL_OAUTH_JWS_KEYS_URL", opts.JWSKeysURL)
	}

	if opts.AudienceList != nil {
		sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_AUDIENCE_LIST", *opts.AudienceList)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.AllowedRoles != nil {
		sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", *opts.AllowedRoles)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.BlockedRoles != nil {
		sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", *opts.BlockedRoles)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.AnyRoleMode != nil {
		sc.UnsafeRaw(fmt.Sprintf("EXTERNAL_OAUTH_ANY_ROLE_MODE = %s", *opts.AnyRoleMode)) //nolint:forbidigo // Snowflake keyword validated by CRD enum
	}

	if opts.ScopeDelimiter != nil {
		sc.String("EXTERNAL_OAUTH_SCOPE_DELIMITER", opts.ScopeDelimiter)
	}

	if opts.NetworkPolicy != nil {
		sc.String("NETWORK_POLICY", opts.NetworkPolicy)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an External OAuth security integration in Snowflake.
func (c *ExternalOAuthIntegrationClient) Alter(ctx context.Context, opts AlterExternalOAuthIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter External OAuth integration options: %w", err))
	}

	stmts, err := buildAlterExternalOAuthIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter External OAuth integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering External OAuth integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an External OAuth security integration.
func (c *ExternalOAuthIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping External OAuth integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves an External OAuth integration from Snowflake.
func (c *ExternalOAuthIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.ExternalOAuthIntegrationShowOutput, error) {
	stmt := sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing External OAuth integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanExternalOAuthIntegrationShowOutput(rows, name.Name())
}

// scanExternalOAuthIntegrationShowOutput scans SHOW output rows into ExternalOAuthIntegrationShowOutput.
func scanExternalOAuthIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.ExternalOAuthIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ExternalOAuthIntegrationShowOutput, error) {
		return &v1alpha1.ExternalOAuthIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed External OAuth integration properties.
func (c *ExternalOAuthIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing External OAuth integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *ExternalOAuthIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*ExternalOAuthIntegrationObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &ExternalOAuthIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &ExternalOAuthIntegrationObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
