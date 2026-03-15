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

// OAuthIntegrationForCustomClientsObservation holds the result of observing a Snowflake OAuth integration for custom clients.
type OAuthIntegrationForCustomClientsObservation struct {
	Exists         bool
	ShowOutput     *v1alpha1.OAuthIntegrationForCustomClientsShowOutput
	DescribeOutput map[string]string
}

// CreateOAuthIntegrationForCustomClientsOptions holds the parameters for CREATE.
type CreateOAuthIntegrationForCustomClientsOptions struct {
	Name                        AccountObjectIdentifier
	Enabled                     *bool
	OAuthClientType             string
	OAuthRedirectURI            string
	OAuthAllowNonTLSRedirectURI *bool
	OAuthEnforcePKCE            *bool
	OAuthUseSecondaryRoles      *string
	PreAuthorizedRolesList      []string
	BlockedRolesList            []string
	OAuthIssueRefreshTokens     *bool
	OAuthRefreshTokenValidity   *int64
	NetworkPolicy               *string
	OAuthClientRSAPublicKey     *string
	OAuthClientRSAPublicKey2    *string
	Comment                     *string
}

// Validate checks that required fields are populated.
func (o *CreateOAuthIntegrationForCustomClientsOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.OAuthClientType == "" {
		errs = append(errs, errors.New("oauth_client_type is required"))
	} else if o.OAuthClientType != "PUBLIC" && o.OAuthClientType != "CONFIDENTIAL" {
		errs = append(errs, fmt.Errorf("invalid oauth_client_type %q", o.OAuthClientType))
	}

	if o.OAuthRedirectURI == "" {
		errs = append(errs, errors.New("oauth_redirect_uri is required"))
	}

	if o.OAuthUseSecondaryRoles != nil && !validOAuthSecondaryRoleModes[*o.OAuthUseSecondaryRoles] {
		errs = append(errs, fmt.Errorf("invalid oauth_use_secondary_roles %q", *o.OAuthUseSecondaryRoles))
	}

	return errors.Join(errs...)
}

var validOAuthSecondaryRoleModes = map[string]bool{
	"IMPLICIT": true,
	"NONE":     true,
}

// AlterOAuthIntegrationForCustomClientsOptions holds the parameters for ALTER.
type AlterOAuthIntegrationForCustomClientsOptions struct {
	Name                        AccountObjectIdentifier
	Enabled                     *bool
	OAuthRedirectURI            *string
	OAuthAllowNonTLSRedirectURI *bool
	OAuthEnforcePKCE            *bool
	OAuthUseSecondaryRoles      *string
	PreAuthorizedRolesList      *[]string
	BlockedRolesList            *[]string
	OAuthIssueRefreshTokens     *bool
	OAuthRefreshTokenValidity   *int64
	NetworkPolicy               *string
	OAuthClientRSAPublicKey     *string
	OAuthClientRSAPublicKey2    *string
	Comment                     *string
	UnsetFields                 []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterOAuthIntegrationForCustomClientsOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.OAuthRedirectURI != nil ||
		o.OAuthAllowNonTLSRedirectURI != nil ||
		o.OAuthEnforcePKCE != nil ||
		o.OAuthUseSecondaryRoles != nil ||
		o.PreAuthorizedRolesList != nil ||
		o.BlockedRolesList != nil ||
		o.OAuthIssueRefreshTokens != nil ||
		o.OAuthRefreshTokenValidity != nil ||
		o.NetworkPolicy != nil ||
		o.OAuthClientRSAPublicKey != nil ||
		o.OAuthClientRSAPublicKey2 != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterOAuthIntegrationForCustomClientsOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	if o.OAuthUseSecondaryRoles != nil && !validOAuthSecondaryRoleModes[*o.OAuthUseSecondaryRoles] {
		return fmt.Errorf("invalid oauth_use_secondary_roles %q", *o.OAuthUseSecondaryRoles)
	}

	return nil
}

// OAuthIntegrationForCustomClientsClient provides operations on Snowflake OAuth integrations for custom clients.
type OAuthIntegrationForCustomClientsClient struct {
	client SQLExecutor
}

// NewOAuthIntegrationForCustomClientsClient creates a new client.
func NewOAuthIntegrationForCustomClientsClient(c SQLExecutor) *OAuthIntegrationForCustomClientsClient {
	return &OAuthIntegrationForCustomClientsClient{client: c}
}

func buildCreateOAuthCustomClientsSQL(opts CreateOAuthIntegrationForCustomClientsOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = OAUTH")
	fmt.Fprintf(&b.Builder, " OAUTH_CLIENT = CUSTOM")
	fmt.Fprintf(&b.Builder, " OAUTH_CLIENT_TYPE = '%s'", sqlbuilder.EscapeString(opts.OAuthClientType))
	b.SetString("OAUTH_REDIRECT_URI", &opts.OAuthRedirectURI)
	b.SetBool("OAUTH_ALLOW_NON_TLS_REDIRECT_URI", opts.OAuthAllowNonTLSRedirectURI)
	b.SetBool("OAUTH_ENFORCE_PKCE", opts.OAuthEnforcePKCE)

	if opts.OAuthUseSecondaryRoles != nil {
		fmt.Fprintf(&b.Builder, " OAUTH_USE_SECONDARY_ROLES = %s", *opts.OAuthUseSecondaryRoles)
	}

	if len(opts.PreAuthorizedRolesList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("PRE_AUTHORIZED_ROLES_LIST", opts.PreAuthorizedRolesList))
	}

	if len(opts.BlockedRolesList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("BLOCKED_ROLES_LIST", opts.BlockedRolesList))
	}

	b.SetBool("OAUTH_ISSUE_REFRESH_TOKENS", opts.OAuthIssueRefreshTokens)

	if opts.OAuthRefreshTokenValidity != nil {
		fmt.Fprintf(&b.Builder, " OAUTH_REFRESH_TOKEN_VALIDITY = %d", *opts.OAuthRefreshTokenValidity)
	}

	if opts.NetworkPolicy != nil {
		b.SetString("NETWORK_POLICY", opts.NetworkPolicy)
	}

	if opts.OAuthClientRSAPublicKey != nil {
		b.SetString("OAUTH_CLIENT_RSA_PUBLIC_KEY", opts.OAuthClientRSAPublicKey)
	}

	if opts.OAuthClientRSAPublicKey2 != nil {
		b.SetString("OAUTH_CLIENT_RSA_PUBLIC_KEY_2", opts.OAuthClientRSAPublicKey2)
	}

	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an OAuth integration for custom clients in Snowflake.
func (c *OAuthIntegrationForCustomClientsClient) Create(ctx context.Context, opts CreateOAuthIntegrationForCustomClientsOptions) error {
	stmt, err := buildCreateOAuthCustomClientsSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create OAuth custom clients integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating OAuth custom clients integration %s: %w", opts.Name, err)
	}

	return nil
}

func buildAlterOAuthCustomClientsStatements(opts AlterOAuthIntegrationForCustomClientsOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.OAuthRedirectURI != nil {
		sc.String("OAUTH_REDIRECT_URI", opts.OAuthRedirectURI)
	}

	sc.Bool("OAUTH_ALLOW_NON_TLS_REDIRECT_URI", opts.OAuthAllowNonTLSRedirectURI)
	sc.Bool("OAUTH_ENFORCE_PKCE", opts.OAuthEnforcePKCE)

	if opts.OAuthUseSecondaryRoles != nil {
		sc.UnsafeRaw(fmt.Sprintf("OAUTH_USE_SECONDARY_ROLES = %s", *opts.OAuthUseSecondaryRoles)) //nolint:forbidigo // Snowflake keyword validated by CRD enum
	}

	if opts.PreAuthorizedRolesList != nil {
		sc.UnsafeRaw(buildStringListClause("PRE_AUTHORIZED_ROLES_LIST", *opts.PreAuthorizedRolesList)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.BlockedRolesList != nil {
		sc.UnsafeRaw(buildStringListClause("BLOCKED_ROLES_LIST", *opts.BlockedRolesList)) //nolint:forbidigo // values escaped via EscapeString
	}

	sc.Bool("OAUTH_ISSUE_REFRESH_TOKENS", opts.OAuthIssueRefreshTokens)

	if opts.OAuthRefreshTokenValidity != nil {
		sc.UnsafeRaw(fmt.Sprintf("OAUTH_REFRESH_TOKEN_VALIDITY = %d", *opts.OAuthRefreshTokenValidity)) //nolint:forbidigo // integer value
	}

	if opts.NetworkPolicy != nil {
		sc.String("NETWORK_POLICY", opts.NetworkPolicy)
	}

	if opts.OAuthClientRSAPublicKey != nil {
		sc.String("OAUTH_CLIENT_RSA_PUBLIC_KEY", opts.OAuthClientRSAPublicKey)
	}

	if opts.OAuthClientRSAPublicKey2 != nil {
		sc.String("OAUTH_CLIENT_RSA_PUBLIC_KEY_2", opts.OAuthClientRSAPublicKey2)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an OAuth integration for custom clients in Snowflake.
func (c *OAuthIntegrationForCustomClientsClient) Alter(ctx context.Context, opts AlterOAuthIntegrationForCustomClientsOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter OAuth custom clients integration options: %w", err))
	}

	stmts, err := buildAlterOAuthCustomClientsStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter OAuth custom clients integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering OAuth custom clients integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an OAuth integration for custom clients.
func (c *OAuthIntegrationForCustomClientsClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping OAuth custom clients integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves an OAuth custom clients integration.
func (c *OAuthIntegrationForCustomClientsClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.OAuthIntegrationForCustomClientsShowOutput, error) {
	stmt := sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing OAuth custom clients integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanOAuthCustomClientsShowOutput(rows, name.Name())
}

func scanOAuthCustomClientsShowOutput(rows *sql.Rows, name string) (*v1alpha1.OAuthIntegrationForCustomClientsShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.OAuthIntegrationForCustomClientsShowOutput, error) {
		return &v1alpha1.OAuthIntegrationForCustomClientsShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed integration properties.
func (c *OAuthIntegrationForCustomClientsClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing OAuth custom clients integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *OAuthIntegrationForCustomClientsClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*OAuthIntegrationForCustomClientsObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &OAuthIntegrationForCustomClientsObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &OAuthIntegrationForCustomClientsObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
