package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SecurityIntegrationObservation holds the result of observing a Snowflake security integration.
type SecurityIntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW SECURITY INTEGRATIONS row.
	ShowOutput *SecurityIntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// SecurityIntegrationShowOutput contains the fields from SHOW SECURITY INTEGRATIONS.
type SecurityIntegrationShowOutput struct {
	CreatedOn string
	Name      string
	Type      string
	Category  string
	Enabled   bool
	Comment   string
}

// CreateSecurityIntegrationOptions holds the parameters for creating a security integration.
type CreateSecurityIntegrationOptions struct {
	Name    AccountObjectIdentifier
	Type    string // EXTERNAL_OAUTH, SAML2, SCIM, API_AUTHENTICATION
	Enabled *bool

	// ExternalOAuth config.
	ExternalOAuthType                     *string
	ExternalOAuthIssuer                   *string
	ExternalOAuthTokenUserMappingClaim    *string
	ExternalOAuthSnowflakeUserMappingAttr *string
	ExternalOAuthJWSKeysURL               *string
	ExternalOAuthAudienceList             []string
	ExternalOAuthAllowedRoles             []string
	ExternalOAuthBlockedRoles             []string
	ExternalOAuthAnyRoleMode              *string
	ExternalOAuthScopeDelimiter           *string
	ExternalOAuthNetworkPolicy            *string

	// SAML2 config.
	SAML2Issuer                *string
	SAML2SSOURL                *string
	SAML2Provider              *string
	SAML2X509Cert              *string
	SAML2AllowedEmailPatterns  []string
	SAML2AllowedUserDomains    []string
	SAML2SPInitiatedLoginLabel *string
	SAML2EnableSPInitiated     *bool
	SAML2ForceAuthn            *bool
	SAML2RequestedNameIDFormat *string
	SAML2PostLogoutRedirectURL *string

	// SCIM config.
	SCIMClient        *string
	SCIMRunAsRole     *string
	SCIMNetworkPolicy *string
	SCIMSyncPassword  *bool

	// API Authentication config.
	OAuthClientID      *string
	OAuthClientSecret  *string
	OAuthTokenEndpoint *string
	OAuthAllowedScopes []string
	OAuthGrantType     *string

	Comment *string
}

// validSecurityIntegrationTypes is the allowlist of security integration types.
var validSecurityIntegrationTypes = map[string]bool{
	"EXTERNAL_OAUTH":     true,
	"SAML2":              true,
	"SCIM":               true,
	"API_AUTHENTICATION": true,
}

// validExternalOAuthTypes is the allowlist of EXTERNAL_OAUTH_TYPE values.
var validExternalOAuthTypes = map[string]bool{
	"OKTA":          true,
	"AZURE":         true,
	"PING_FEDERATE": true,
	"CUSTOM":        true,
}

// validExternalOAuthAnyRoleModes is the allowlist of EXTERNAL_OAUTH_ANY_ROLE_MODE values.
var validExternalOAuthAnyRoleModes = map[string]bool{
	"DISABLE":              true,
	"ENABLE":               true,
	"ENABLE_FOR_PRIVILEGE": true,
}

// validOAuthGrantTypes is the allowlist of OAUTH_GRANT values.
var validOAuthGrantTypes = map[string]bool{
	"CLIENT_CREDENTIALS": true,
	"AUTHORIZATION_CODE": true,
	"JWT_BEARER":         true,
}

// Validate checks the CreateSecurityIntegrationOptions for validity.
func (o *CreateSecurityIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("security integration name is required"))
	}

	if o.Type == "" {
		errs = append(errs, fmt.Errorf("security integration type is required"))
	} else if !validSecurityIntegrationTypes[o.Type] {
		errs = append(errs, fmt.Errorf("invalid security integration type %q", o.Type))
	}

	switch o.Type {
	case "EXTERNAL_OAUTH":
		if o.ExternalOAuthType == nil || *o.ExternalOAuthType == "" {
			errs = append(errs, fmt.Errorf("external_oauth_type is required for EXTERNAL_OAUTH"))
		} else if !validExternalOAuthTypes[*o.ExternalOAuthType] {
			errs = append(errs, fmt.Errorf("invalid external_oauth_type %q", *o.ExternalOAuthType))
		}
		if o.ExternalOAuthIssuer == nil || *o.ExternalOAuthIssuer == "" {
			errs = append(errs, fmt.Errorf("external_oauth_issuer is required for EXTERNAL_OAUTH"))
		}
		if o.ExternalOAuthTokenUserMappingClaim == nil || *o.ExternalOAuthTokenUserMappingClaim == "" {
			errs = append(errs, fmt.Errorf("external_oauth_token_user_mapping_claim is required for EXTERNAL_OAUTH"))
		}
		if o.ExternalOAuthSnowflakeUserMappingAttr == nil || *o.ExternalOAuthSnowflakeUserMappingAttr == "" {
			errs = append(errs, fmt.Errorf("external_oauth_snowflake_user_mapping_attribute is required for EXTERNAL_OAUTH"))
		}
		if o.ExternalOAuthAnyRoleMode != nil && !validExternalOAuthAnyRoleModes[*o.ExternalOAuthAnyRoleMode] {
			errs = append(errs, fmt.Errorf("invalid external_oauth_any_role_mode %q", *o.ExternalOAuthAnyRoleMode))
		}
	case "SAML2":
		if o.SAML2Issuer == nil || *o.SAML2Issuer == "" {
			errs = append(errs, fmt.Errorf("saml2_issuer is required for SAML2"))
		}
		if o.SAML2SSOURL == nil || *o.SAML2SSOURL == "" {
			errs = append(errs, fmt.Errorf("saml2_sso_url is required for SAML2"))
		}
		if o.SAML2Provider == nil || *o.SAML2Provider == "" {
			errs = append(errs, fmt.Errorf("saml2_provider is required for SAML2"))
		}
		if o.SAML2X509Cert == nil || *o.SAML2X509Cert == "" {
			errs = append(errs, fmt.Errorf("saml2_x509_cert is required for SAML2"))
		}
	case "SCIM":
		if o.SCIMClient == nil || *o.SCIMClient == "" {
			errs = append(errs, fmt.Errorf("scim_client is required for SCIM"))
		}
		if o.SCIMRunAsRole == nil || *o.SCIMRunAsRole == "" {
			errs = append(errs, fmt.Errorf("run_as_role is required for SCIM"))
		}
	case "API_AUTHENTICATION":
		if o.OAuthClientID == nil || *o.OAuthClientID == "" {
			errs = append(errs, fmt.Errorf("oauth_client_id is required for API_AUTHENTICATION"))
		}
		if o.OAuthClientSecret == nil || *o.OAuthClientSecret == "" {
			errs = append(errs, fmt.Errorf("oauth_client_secret is required for API_AUTHENTICATION"))
		}
		if o.OAuthGrantType != nil && !validOAuthGrantTypes[*o.OAuthGrantType] {
			errs = append(errs, fmt.Errorf("invalid oauth_grant type %q", *o.OAuthGrantType))
		}
	}

	return errors.Join(errs...)
}

// AlterSecurityIntegrationOptions holds the parameters for altering a security integration.
type AlterSecurityIntegrationOptions struct {
	Name    AccountObjectIdentifier
	Type    string
	Enabled *bool

	// ExternalOAuth config (alterable fields).
	ExternalOAuthTokenUserMappingClaim *string
	ExternalOAuthJWSKeysURL            *string
	ExternalOAuthAudienceList          *[]string
	ExternalOAuthAllowedRoles          *[]string
	ExternalOAuthBlockedRoles          *[]string
	ExternalOAuthAnyRoleMode           *string
	ExternalOAuthScopeDelimiter        *string
	ExternalOAuthNetworkPolicy         *string

	// SAML2 config (alterable fields).
	SAML2X509Cert              *string
	SAML2AllowedEmailPatterns  *[]string
	SAML2AllowedUserDomains    *[]string
	SAML2SPInitiatedLoginLabel *string
	SAML2EnableSPInitiated     *bool
	SAML2ForceAuthn            *bool
	SAML2RequestedNameIDFormat *string
	SAML2PostLogoutRedirectURL *string

	// SCIM config (alterable fields).
	SCIMNetworkPolicy *string
	SCIMSyncPassword  *bool

	// API Authentication (alterable fields).
	OAuthAllowedScopes *[]string

	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterSecurityIntegrationOptions for validity.
func (o *AlterSecurityIntegrationOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("security integration name is required"))
	}

	if o.ExternalOAuthAnyRoleMode != nil && !validExternalOAuthAnyRoleModes[*o.ExternalOAuthAnyRoleMode] {
		errs = append(errs, fmt.Errorf("invalid external_oauth_any_role_mode %q", *o.ExternalOAuthAnyRoleMode))
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterSecurityIntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.Comment != nil ||
		o.ExternalOAuthTokenUserMappingClaim != nil ||
		o.ExternalOAuthJWSKeysURL != nil ||
		o.ExternalOAuthAudienceList != nil ||
		o.ExternalOAuthAllowedRoles != nil ||
		o.ExternalOAuthBlockedRoles != nil ||
		o.ExternalOAuthAnyRoleMode != nil ||
		o.ExternalOAuthScopeDelimiter != nil ||
		o.ExternalOAuthNetworkPolicy != nil ||
		o.SAML2X509Cert != nil ||
		o.SAML2AllowedEmailPatterns != nil ||
		o.SAML2AllowedUserDomains != nil ||
		o.SAML2SPInitiatedLoginLabel != nil ||
		o.SAML2EnableSPInitiated != nil ||
		o.SAML2ForceAuthn != nil ||
		o.SAML2RequestedNameIDFormat != nil ||
		o.SAML2PostLogoutRedirectURL != nil ||
		o.SCIMNetworkPolicy != nil ||
		o.SCIMSyncPassword != nil ||
		o.OAuthAllowedScopes != nil ||
		len(o.UnsetFields) > 0
}

// SecurityIntegrationClient provides operations against Snowflake security integrations.
type SecurityIntegrationClient struct {
	client SQLExecutor
}

// NewSecurityIntegrationClient creates a new SecurityIntegrationClient.
func NewSecurityIntegrationClient(c SQLExecutor) *SecurityIntegrationClient {
	return &SecurityIntegrationClient{client: c}
}

// buildStringListClause formats a list of strings for Snowflake SQL, e.g. ('val1', 'val2').
func buildStringListClause(keyword string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
	}

	return fmt.Sprintf("%s = (%s)", keyword, strings.Join(quoted, ", "))
}

// buildCreateSecurityIntegrationSQL builds the CREATE SECURITY INTEGRATION SQL statement.
func buildCreateSecurityIntegrationSQL(opts CreateSecurityIntegrationOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = %s", opts.Type)

	switch opts.Type {
	case "EXTERNAL_OAUTH":
		if opts.ExternalOAuthType != nil {
			fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_TYPE = %s", *opts.ExternalOAuthType)
		}
		if opts.ExternalOAuthIssuer != nil {
			b.SetString("EXTERNAL_OAUTH_ISSUER", opts.ExternalOAuthIssuer)
		}
		if opts.ExternalOAuthTokenUserMappingClaim != nil {
			fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM = '%s'", sqlbuilder.EscapeString(*opts.ExternalOAuthTokenUserMappingClaim))
		}
		if opts.ExternalOAuthSnowflakeUserMappingAttr != nil {
			fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_SNOWFLAKE_USER_MAPPING_ATTRIBUTE = '%s'", sqlbuilder.EscapeString(*opts.ExternalOAuthSnowflakeUserMappingAttr))
		}
		if opts.ExternalOAuthJWSKeysURL != nil {
			b.SetString("EXTERNAL_OAUTH_JWS_KEYS_URL", opts.ExternalOAuthJWSKeysURL)
		}
		if len(opts.ExternalOAuthAudienceList) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("EXTERNAL_OAUTH_AUDIENCE_LIST", opts.ExternalOAuthAudienceList))
		}
		if len(opts.ExternalOAuthAllowedRoles) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", opts.ExternalOAuthAllowedRoles))
		}
		if len(opts.ExternalOAuthBlockedRoles) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", opts.ExternalOAuthBlockedRoles))
		}
		if opts.ExternalOAuthAnyRoleMode != nil {
			fmt.Fprintf(&b.Builder, " EXTERNAL_OAUTH_ANY_ROLE_MODE = %s", *opts.ExternalOAuthAnyRoleMode)
		}
		if opts.ExternalOAuthScopeDelimiter != nil {
			b.SetString("EXTERNAL_OAUTH_SCOPE_DELIMITER", opts.ExternalOAuthScopeDelimiter)
		}
		if opts.ExternalOAuthNetworkPolicy != nil {
			b.SetString("NETWORK_POLICY", opts.ExternalOAuthNetworkPolicy)
		}
	case "SAML2":
		if opts.SAML2Issuer != nil {
			b.SetString("SAML2_ISSUER", opts.SAML2Issuer)
		}
		if opts.SAML2SSOURL != nil {
			b.SetString("SAML2_SSO_URL", opts.SAML2SSOURL)
		}
		if opts.SAML2Provider != nil {
			b.SetString("SAML2_PROVIDER", opts.SAML2Provider)
		}
		if opts.SAML2X509Cert != nil {
			b.SetString("SAML2_X509_CERT", opts.SAML2X509Cert)
		}
		if len(opts.SAML2AllowedEmailPatterns) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("ALLOWED_EMAIL_PATTERNS", opts.SAML2AllowedEmailPatterns))
		}
		if len(opts.SAML2AllowedUserDomains) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("ALLOWED_USER_DOMAINS", opts.SAML2AllowedUserDomains))
		}
		if opts.SAML2SPInitiatedLoginLabel != nil {
			b.SetString("SAML2_SP_INITIATED_LOGIN_PAGE_LABEL", opts.SAML2SPInitiatedLoginLabel)
		}
		if opts.SAML2EnableSPInitiated != nil {
			b.SetBool("SAML2_ENABLE_SP_INITIATED", opts.SAML2EnableSPInitiated)
		}
		if opts.SAML2ForceAuthn != nil {
			b.SetBool("SAML2_FORCE_AUTHN", opts.SAML2ForceAuthn)
		}
		if opts.SAML2RequestedNameIDFormat != nil {
			b.SetString("SAML2_REQUESTED_NAMEID_FORMAT", opts.SAML2RequestedNameIDFormat)
		}
		if opts.SAML2PostLogoutRedirectURL != nil {
			b.SetString("SAML2_POST_LOGOUT_REDIRECT_URL", opts.SAML2PostLogoutRedirectURL)
		}
	case "SCIM":
		if opts.SCIMClient != nil {
			fmt.Fprintf(&b.Builder, " SCIM_CLIENT = '%s'", sqlbuilder.EscapeString(*opts.SCIMClient))
		}
		if opts.SCIMRunAsRole != nil {
			fmt.Fprintf(&b.Builder, " RUN_AS_ROLE = '%s'", sqlbuilder.EscapeString(*opts.SCIMRunAsRole))
		}
		if opts.SCIMNetworkPolicy != nil {
			b.SetString("NETWORK_POLICY", opts.SCIMNetworkPolicy)
		}
		if opts.SCIMSyncPassword != nil {
			b.SetBool("SYNC_PASSWORD", opts.SCIMSyncPassword)
		}
	case "API_AUTHENTICATION":
		fmt.Fprintf(&b.Builder, " AUTH_TYPE = OAUTH2")
		if opts.OAuthClientID != nil {
			b.SetString("OAUTH_CLIENT_ID", opts.OAuthClientID)
		}
		if opts.OAuthClientSecret != nil {
			b.SetString("OAUTH_CLIENT_SECRET", opts.OAuthClientSecret)
		}
		if opts.OAuthTokenEndpoint != nil {
			b.SetString("OAUTH_TOKEN_ENDPOINT", opts.OAuthTokenEndpoint)
		}
		if len(opts.OAuthAllowedScopes) > 0 {
			b.WriteString(" ")
			b.WriteString(buildStringListClause("OAUTH_ALLOWED_SCOPES", opts.OAuthAllowedScopes))
		}
		if opts.OAuthGrantType != nil {
			fmt.Fprintf(&b.Builder, " OAUTH_GRANT = '%s'", *opts.OAuthGrantType)
		}
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

// Create creates a security integration in Snowflake.
func (si *SecurityIntegrationClient) Create(ctx context.Context, opts CreateSecurityIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create security integration options: %w", err))
	}

	sql, err := buildCreateSecurityIntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create security integration SQL: %w", err))
	}

	if _, err := si.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating security integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSecurityIntegrationStatements builds ALTER SECURITY INTEGRATION statements.
func buildAlterSecurityIntegrationStatements(opts AlterSecurityIntegrationOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.Enabled != nil {
		sc.Bool("ENABLED", opts.Enabled)
	}

	// Type-specific alterable fields.
	switch opts.Type {
	case "EXTERNAL_OAUTH":
		if opts.ExternalOAuthTokenUserMappingClaim != nil {
			sc.String("EXTERNAL_OAUTH_TOKEN_USER_MAPPING_CLAIM", opts.ExternalOAuthTokenUserMappingClaim)
		}
		if opts.ExternalOAuthJWSKeysURL != nil {
			sc.String("EXTERNAL_OAUTH_JWS_KEYS_URL", opts.ExternalOAuthJWSKeysURL)
		}
		if opts.ExternalOAuthAudienceList != nil {
			sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_AUDIENCE_LIST", *opts.ExternalOAuthAudienceList))
		}
		if opts.ExternalOAuthAllowedRoles != nil {
			sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_ALLOWED_ROLES_LIST", *opts.ExternalOAuthAllowedRoles))
		}
		if opts.ExternalOAuthBlockedRoles != nil {
			sc.UnsafeRaw(buildStringListClause("EXTERNAL_OAUTH_BLOCKED_ROLES_LIST", *opts.ExternalOAuthBlockedRoles))
		}
		if opts.ExternalOAuthAnyRoleMode != nil {
			sc.UnsafeRaw(fmt.Sprintf("EXTERNAL_OAUTH_ANY_ROLE_MODE = %s", *opts.ExternalOAuthAnyRoleMode))
		}
		if opts.ExternalOAuthScopeDelimiter != nil {
			sc.String("EXTERNAL_OAUTH_SCOPE_DELIMITER", opts.ExternalOAuthScopeDelimiter)
		}
		if opts.ExternalOAuthNetworkPolicy != nil {
			sc.String("NETWORK_POLICY", opts.ExternalOAuthNetworkPolicy)
		}
	case "SAML2":
		if opts.SAML2X509Cert != nil {
			sc.String("SAML2_X509_CERT", opts.SAML2X509Cert)
		}
		if opts.SAML2AllowedEmailPatterns != nil {
			sc.UnsafeRaw(buildStringListClause("ALLOWED_EMAIL_PATTERNS", *opts.SAML2AllowedEmailPatterns))
		}
		if opts.SAML2AllowedUserDomains != nil {
			sc.UnsafeRaw(buildStringListClause("ALLOWED_USER_DOMAINS", *opts.SAML2AllowedUserDomains))
		}
		if opts.SAML2SPInitiatedLoginLabel != nil {
			sc.String("SAML2_SP_INITIATED_LOGIN_PAGE_LABEL", opts.SAML2SPInitiatedLoginLabel)
		}
		if opts.SAML2EnableSPInitiated != nil {
			sc.Bool("SAML2_ENABLE_SP_INITIATED", opts.SAML2EnableSPInitiated)
		}
		if opts.SAML2ForceAuthn != nil {
			sc.Bool("SAML2_FORCE_AUTHN", opts.SAML2ForceAuthn)
		}
		if opts.SAML2RequestedNameIDFormat != nil {
			sc.String("SAML2_REQUESTED_NAMEID_FORMAT", opts.SAML2RequestedNameIDFormat)
		}
		if opts.SAML2PostLogoutRedirectURL != nil {
			sc.String("SAML2_POST_LOGOUT_REDIRECT_URL", opts.SAML2PostLogoutRedirectURL)
		}
	case "SCIM":
		if opts.SCIMNetworkPolicy != nil {
			sc.String("NETWORK_POLICY", opts.SCIMNetworkPolicy)
		}
		if opts.SCIMSyncPassword != nil {
			sc.Bool("SYNC_PASSWORD", opts.SCIMSyncPassword)
		}
	case "API_AUTHENTICATION":
		if opts.OAuthAllowedScopes != nil {
			sc.UnsafeRaw(buildStringListClause("OAUTH_ALLOWED_SCOPES", *opts.OAuthAllowedScopes))
		}
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a security integration in Snowflake.
func (si *SecurityIntegrationClient) Alter(ctx context.Context, opts AlterSecurityIntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter security integration options: %w", err))
	}

	stmts, err := buildAlterSecurityIntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter security integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := si.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering security integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a security integration from Snowflake.
func (si *SecurityIntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("security integration name is required"))
	}

	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := si.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping security integration %s: %w", name, err)
	}

	return nil
}

// buildShowSecurityIntegrationByIDSQL builds the SHOW SQL for a specific security integration.
func buildShowSecurityIntegrationByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())
}

// ShowByID queries SHOW SECURITY INTEGRATIONS for a specific integration.
func (si *SecurityIntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*SecurityIntegrationShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("security integration name is required"))
	}

	rows, err := si.client.Query(ctx, buildShowSecurityIntegrationByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing security integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSecurityIntegrationShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE INTEGRATION and returns key-value pairs of properties.
func (si *SecurityIntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("security integration name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := si.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing security integration %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a SecurityIntegrationObservation.
func (si *SecurityIntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SecurityIntegrationObservation, error) {
	show, err := si.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SecurityIntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := si.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &SecurityIntegrationObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &SecurityIntegrationObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanSecurityIntegrationShowOutput scans SHOW SECURITY INTEGRATIONS results.
func scanSecurityIntegrationShowOutput(rows *sql.Rows, name string) (*SecurityIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*SecurityIntegrationShowOutput, error) {
		return &SecurityIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}
