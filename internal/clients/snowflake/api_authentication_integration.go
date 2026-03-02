package snowflake

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// OAuthGrantType identifies the OAuth grant flow for API authentication integrations.
type OAuthGrantType string

// OAuth grant types for API authentication integrations.
//
//nolint:gosec // G101: constants are not hardcoded credentials.
const (
	OAuthGrantTypeClientCredentials OAuthGrantType = "CLIENT_CREDENTIALS"
	OAuthGrantTypeAuthorizationCode OAuthGrantType = "AUTHORIZATION_CODE"
	OAuthGrantTypeJWTBearer         OAuthGrantType = "JWT_BEARER"
)

// APIAuthenticationIntegrationObservation holds the result of observing
// a Snowflake API Authentication security integration.
type APIAuthenticationIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW SECURITY INTEGRATIONS row.
	ShowOutput *SecurityIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateAPIAuthenticationIntegrationOptions holds the parameters for creating
// an API authentication security integration.
type CreateAPIAuthenticationIntegrationOptions struct {
	Name           AccountObjectIdentifier
	OAuthGrantType OAuthGrantType
	Enabled        *bool

	// Required for all grant types.
	OAuthClientID     string
	OAuthClientSecret string

	// Common optional parameters.
	OAuthTokenEndpoint       *string
	OAuthClientAuthMethod    *string
	OAuthAccessTokenValidity *int32
	OAuthAllowedScopes       []string
	Comment                  *string

	// Authorization code grant specific.
	OAuthAuthorizationEndpoint *string
	OAuthRefreshTokenValidity  *int32

	// JWT bearer specific.
	OAuthAssertionIssuer *string
}

// Validate checks the CreateAPIAuthenticationIntegrationOptions for validity.
func (o *CreateAPIAuthenticationIntegrationOptions) Validate() error {
	var errs []string

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, "integration name is required")
	}

	if o.OAuthGrantType == "" {
		errs = append(errs, "oauth_grant type is required")
	}

	if o.OAuthClientID == "" {
		errs = append(errs, "oauth_client_id is required")
	}

	if o.OAuthClientSecret == "" {
		errs = append(errs, "oauth_client_secret is required")
	}

	if o.OAuthClientAuthMethod != nil {
		switch *o.OAuthClientAuthMethod {
		case "CLIENT_SECRET_POST", "CLIENT_SECRET_BASIC":
			// OK
		default:
			errs = append(errs, fmt.Sprintf("invalid oauth_client_auth_method %q", *o.OAuthClientAuthMethod))
		}
	}

	if o.OAuthGrantType == OAuthGrantTypeJWTBearer {
		if o.OAuthAssertionIssuer == nil || *o.OAuthAssertionIssuer == "" {
			errs = append(errs, "oauth_assertion_issuer is required for JWT_BEARER grant type")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

// AlterAPIAuthenticationIntegrationOptions holds the parameters for altering
// an API authentication security integration.
type AlterAPIAuthenticationIntegrationOptions struct {
	Name           AccountObjectIdentifier
	OAuthGrantType OAuthGrantType
	Enabled        *bool

	// Alterable common parameters.
	OAuthTokenEndpoint       *string
	OAuthClientAuthMethod    *string
	OAuthAccessTokenValidity *int32
	OAuthAllowedScopes       *[]string
	Comment                  *string

	// Authorization code grant specific.
	OAuthAuthorizationEndpoint *string
	OAuthRefreshTokenValidity  *int32

	// JWT bearer specific.
	OAuthAssertionIssuer *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterAPIAuthenticationIntegrationOptions for validity.
func (o *AlterAPIAuthenticationIntegrationOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("integration name is required")
	}

	if o.OAuthClientAuthMethod != nil {
		switch *o.OAuthClientAuthMethod {
		case "CLIENT_SECRET_POST", "CLIENT_SECRET_BASIC":
			// OK
		default:
			return fmt.Errorf("invalid oauth_client_auth_method %q", *o.OAuthClientAuthMethod)
		}
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterAPIAuthenticationIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.OAuthTokenEndpoint != nil ||
		o.OAuthClientAuthMethod != nil ||
		o.OAuthAccessTokenValidity != nil ||
		o.OAuthAllowedScopes != nil ||
		o.OAuthAuthorizationEndpoint != nil ||
		o.OAuthRefreshTokenValidity != nil ||
		o.OAuthAssertionIssuer != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// APIAuthenticationIntegrationClient provides operations against Snowflake
// API authentication security integrations.
type APIAuthenticationIntegrationClient struct {
	client SQLExecutor
}

// NewAPIAuthenticationIntegrationClient creates a new APIAuthenticationIntegrationClient.
func NewAPIAuthenticationIntegrationClient(c SQLExecutor) *APIAuthenticationIntegrationClient {
	return &APIAuthenticationIntegrationClient{client: c}
}

// buildCreateAPIAuthIntegrationSQL builds the CREATE SECURITY INTEGRATION SQL
// for an API authentication integration.
func buildCreateAPIAuthIntegrationSQL(opts CreateAPIAuthenticationIntegrationOptions) string {
	var b sqlbuilder.Builder

	b.WriteString("CREATE SECURITY INTEGRATION IF NOT EXISTS ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.Name.Name()))
	b.WriteString(" TYPE = API_AUTHENTICATION AUTH_TYPE = OAUTH2")
	fmt.Fprintf(&b.Builder, " OAUTH_GRANT = %s", string(opts.OAuthGrantType))

	if opts.Enabled != nil {
		b.SetBool("ENABLED", opts.Enabled)
	}

	clientID := opts.OAuthClientID
	b.SetString("OAUTH_CLIENT_ID", &clientID)

	clientSecret := opts.OAuthClientSecret
	b.SetString("OAUTH_CLIENT_SECRET", &clientSecret)

	b.SetString("OAUTH_TOKEN_ENDPOINT", opts.OAuthTokenEndpoint)

	if opts.OAuthClientAuthMethod != nil {
		// OAUTH_CLIENT_AUTH_METHOD is an unquoted keyword.
		b.SetKeyword("OAUTH_CLIENT_AUTH_METHOD", opts.OAuthClientAuthMethod)
	}

	b.SetInt32("OAUTH_ACCESS_TOKEN_VALIDITY", opts.OAuthAccessTokenValidity)

	if len(opts.OAuthAllowedScopes) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("OAUTH_ALLOWED_SCOPES", opts.OAuthAllowedScopes))
	}

	// Grant-type specific parameters.
	switch opts.OAuthGrantType {
	case OAuthGrantTypeAuthorizationCode:
		b.SetString("OAUTH_AUTHORIZATION_ENDPOINT", opts.OAuthAuthorizationEndpoint)
		b.SetInt32("OAUTH_REFRESH_TOKEN_VALIDITY", opts.OAuthRefreshTokenValidity)
	case OAuthGrantTypeJWTBearer:
		b.SetString("OAUTH_ASSERTION_ISSUER", opts.OAuthAssertionIssuer)
		b.SetString("OAUTH_AUTHORIZATION_ENDPOINT", opts.OAuthAuthorizationEndpoint)
		b.SetInt32("OAUTH_REFRESH_TOKEN_VALIDITY", opts.OAuthRefreshTokenValidity)
	case OAuthGrantTypeClientCredentials:
		// Client credentials grant has no additional grant-specific parameters.
	}

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates an API authentication security integration in Snowflake.
func (c *APIAuthenticationIntegrationClient) Create(ctx context.Context, opts CreateAPIAuthenticationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create API authentication integration options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildCreateAPIAuthIntegrationSQL(opts)); err != nil {
		return fmt.Errorf("creating API authentication integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAPIAuthIntegrationStatements builds ALTER SECURITY INTEGRATION
// statements for an API authentication integration.
func buildAlterAPIAuthIntegrationStatements(opts AlterAPIAuthenticationIntegrationOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.Enabled != nil {
		sc.Bool("ENABLED", opts.Enabled)
	}

	sc.String("OAUTH_TOKEN_ENDPOINT", opts.OAuthTokenEndpoint)

	if opts.OAuthClientAuthMethod != nil {
		sc.Keyword("OAUTH_CLIENT_AUTH_METHOD", opts.OAuthClientAuthMethod)
	}

	sc.Int32("OAUTH_ACCESS_TOKEN_VALIDITY", opts.OAuthAccessTokenValidity)

	if opts.OAuthAllowedScopes != nil {
		sc.UnsafeRaw(buildStringListClause("OAUTH_ALLOWED_SCOPES", *opts.OAuthAllowedScopes))
	}

	// Grant-type specific parameters.
	switch opts.OAuthGrantType {
	case OAuthGrantTypeAuthorizationCode:
		sc.String("OAUTH_AUTHORIZATION_ENDPOINT", opts.OAuthAuthorizationEndpoint)
		sc.Int32("OAUTH_REFRESH_TOKEN_VALIDITY", opts.OAuthRefreshTokenValidity)
	case OAuthGrantTypeJWTBearer:
		sc.String("OAUTH_ASSERTION_ISSUER", opts.OAuthAssertionIssuer)
		sc.String("OAUTH_AUTHORIZATION_ENDPOINT", opts.OAuthAuthorizationEndpoint)
		sc.Int32("OAUTH_REFRESH_TOKEN_VALIDITY", opts.OAuthRefreshTokenValidity)
	case OAuthGrantTypeClientCredentials:
		// Client credentials grant has no additional grant-specific parameters.
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an API authentication security integration in Snowflake.
func (c *APIAuthenticationIntegrationClient) Alter(ctx context.Context, opts AlterAPIAuthenticationIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter API authentication integration options: %w", err))
	}

	stmts, err := buildAlterAPIAuthIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter API authentication integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering API authentication integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an API authentication security integration from Snowflake.
func (c *APIAuthenticationIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("API authentication integration name is required"))
	}

	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping API authentication integration %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW SECURITY INTEGRATIONS for a specific API authentication integration.
func (c *APIAuthenticationIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*SecurityIntegrationShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("API authentication integration name is required"))
	}

	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing API authentication integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSecurityIntegrationShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs of properties.
func (c *APIAuthenticationIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("API authentication integration name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing API authentication integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into an APIAuthenticationIntegrationObservation.
func (c *APIAuthenticationIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*APIAuthenticationIntegrationObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &APIAuthenticationIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &APIAuthenticationIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &APIAuthenticationIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}
