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

// OAuthIntegrationForPartnerApplicationsObservation holds the result of observing a Snowflake OAuth integration for partner applications.
type OAuthIntegrationForPartnerApplicationsObservation struct {
	Exists         bool
	ShowOutput     *v1alpha1.OAuthIntegrationForPartnerApplicationsShowOutput
	DescribeOutput map[string]string
}

// CreateOAuthIntegrationForPartnerApplicationsOptions holds the parameters for CREATE.
type CreateOAuthIntegrationForPartnerApplicationsOptions struct {
	Name                      AccountObjectIdentifier
	Enabled                   *bool
	OAuthClient               string
	OAuthRedirectURI          *string
	OAuthUseSecondaryRoles    *string
	OAuthIssueRefreshTokens   *bool
	OAuthRefreshTokenValidity *int64
	BlockedRolesList          []string
	Comment                   *string
}

// Validate checks that required fields are populated.
func (o *CreateOAuthIntegrationForPartnerApplicationsOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.OAuthClient == "" {
		errs = append(errs, errors.New("oauth_client is required"))
	} else if !validOAuthPartnerClients[o.OAuthClient] {
		errs = append(errs, fmt.Errorf("invalid oauth_client %q", o.OAuthClient))
	}

	if o.OAuthUseSecondaryRoles != nil && !validOAuthSecondaryRoleModes[*o.OAuthUseSecondaryRoles] {
		errs = append(errs, fmt.Errorf("invalid oauth_use_secondary_roles %q", *o.OAuthUseSecondaryRoles))
	}

	return errors.Join(errs...)
}

var validOAuthPartnerClients = map[string]bool{
	"TABLEAU_DESKTOP": true,
	"TABLEAU_SERVER":  true,
	"LOOKER":          true,
}

// AlterOAuthIntegrationForPartnerApplicationsOptions holds the parameters for ALTER.
type AlterOAuthIntegrationForPartnerApplicationsOptions struct {
	Name                      AccountObjectIdentifier
	Enabled                   *bool
	OAuthRedirectURI          *string
	OAuthUseSecondaryRoles    *string
	OAuthIssueRefreshTokens   *bool
	OAuthRefreshTokenValidity *int64
	BlockedRolesList          *[]string
	Comment                   *string
	UnsetFields               []string
}

// HasChanges returns true if there are any SET or UNSET operations.
func (o *AlterOAuthIntegrationForPartnerApplicationsOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.OAuthRedirectURI != nil ||
		o.OAuthUseSecondaryRoles != nil ||
		o.OAuthIssueRefreshTokens != nil ||
		o.OAuthRefreshTokenValidity != nil ||
		o.BlockedRolesList != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity.
func (o *AlterOAuthIntegrationForPartnerApplicationsOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	if o.OAuthUseSecondaryRoles != nil && !validOAuthSecondaryRoleModes[*o.OAuthUseSecondaryRoles] {
		return fmt.Errorf("invalid oauth_use_secondary_roles %q", *o.OAuthUseSecondaryRoles)
	}

	return nil
}

// OAuthIntegrationForPartnerApplicationsClient provides operations.
type OAuthIntegrationForPartnerApplicationsClient struct {
	client SQLExecutor
}

// NewOAuthIntegrationForPartnerApplicationsClient creates a new client.
func NewOAuthIntegrationForPartnerApplicationsClient(c SQLExecutor) *OAuthIntegrationForPartnerApplicationsClient {
	return &OAuthIntegrationForPartnerApplicationsClient{client: c}
}

func buildCreateOAuthPartnerAppsSQL(opts CreateOAuthIntegrationForPartnerApplicationsOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = OAUTH")
	fmt.Fprintf(&b.Builder, " OAUTH_CLIENT = %s", opts.OAuthClient)

	if opts.OAuthRedirectURI != nil {
		b.SetString("OAUTH_REDIRECT_URI", opts.OAuthRedirectURI)
	}

	if opts.OAuthUseSecondaryRoles != nil {
		fmt.Fprintf(&b.Builder, " OAUTH_USE_SECONDARY_ROLES = %s", *opts.OAuthUseSecondaryRoles)
	}

	b.SetBool("OAUTH_ISSUE_REFRESH_TOKENS", opts.OAuthIssueRefreshTokens)

	if opts.OAuthRefreshTokenValidity != nil {
		fmt.Fprintf(&b.Builder, " OAUTH_REFRESH_TOKEN_VALIDITY = %d", *opts.OAuthRefreshTokenValidity)
	}

	if len(opts.BlockedRolesList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("BLOCKED_ROLES_LIST", opts.BlockedRolesList))
	}

	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an OAuth integration for partner applications.
func (c *OAuthIntegrationForPartnerApplicationsClient) Create(ctx context.Context, opts CreateOAuthIntegrationForPartnerApplicationsOptions) error {
	stmt, err := buildCreateOAuthPartnerAppsSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create OAuth partner apps integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating OAuth partner apps integration %s: %w", opts.Name, err)
	}

	return nil
}

func buildAlterOAuthPartnerAppsStatements(opts AlterOAuthIntegrationForPartnerApplicationsOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.OAuthRedirectURI != nil {
		sc.String("OAUTH_REDIRECT_URI", opts.OAuthRedirectURI)
	}

	if opts.OAuthUseSecondaryRoles != nil {
		sc.UnsafeRaw(fmt.Sprintf("OAUTH_USE_SECONDARY_ROLES = %s", *opts.OAuthUseSecondaryRoles)) //nolint:forbidigo // Snowflake keyword validated by CRD enum
	}

	sc.Bool("OAUTH_ISSUE_REFRESH_TOKENS", opts.OAuthIssueRefreshTokens)

	if opts.OAuthRefreshTokenValidity != nil {
		sc.UnsafeRaw(fmt.Sprintf("OAUTH_REFRESH_TOKEN_VALIDITY = %d", *opts.OAuthRefreshTokenValidity)) //nolint:forbidigo // integer value
	}

	if opts.BlockedRolesList != nil {
		sc.UnsafeRaw(buildStringListClause("BLOCKED_ROLES_LIST", *opts.BlockedRolesList)) //nolint:forbidigo // values escaped via EscapeString
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters an OAuth integration for partner applications.
func (c *OAuthIntegrationForPartnerApplicationsClient) Alter(ctx context.Context, opts AlterOAuthIntegrationForPartnerApplicationsOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter OAuth partner apps integration options: %w", err))
	}

	stmts, err := buildAlterOAuthPartnerAppsStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter OAuth partner apps integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering OAuth partner apps integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an OAuth integration for partner applications.
func (c *OAuthIntegrationForPartnerApplicationsClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping OAuth partner apps integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves an OAuth partner apps integration.
func (c *OAuthIntegrationForPartnerApplicationsClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.OAuthIntegrationForPartnerApplicationsShowOutput, error) {
	stmt := sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing OAuth partner apps integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanOAuthPartnerAppsShowOutput(rows, name.Name())
}

func scanOAuthPartnerAppsShowOutput(rows *sql.Rows, name string) (*v1alpha1.OAuthIntegrationForPartnerApplicationsShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.OAuthIntegrationForPartnerApplicationsShowOutput, error) {
		return &v1alpha1.OAuthIntegrationForPartnerApplicationsShowOutput{
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
func (c *OAuthIntegrationForPartnerApplicationsClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing OAuth partner apps integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *OAuthIntegrationForPartnerApplicationsClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*OAuthIntegrationForPartnerApplicationsObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &OAuthIntegrationForPartnerApplicationsObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &OAuthIntegrationForPartnerApplicationsObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
