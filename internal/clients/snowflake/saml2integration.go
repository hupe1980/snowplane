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

// SAML2IntegrationObservation holds the result of observing a Snowflake SAML2 security integration.
type SAML2IntegrationObservation struct {
	// Exists indicates whether the integration was found.
	Exists bool

	// ShowOutput contains the SHOW SECURITY INTEGRATIONS row.
	ShowOutput *v1alpha1.SAML2IntegrationShowOutput

	// DescribeOutput contains the DESCRIBE INTEGRATION output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateSAML2IntegrationOptions holds the parameters for creating a SAML2 security integration.
type CreateSAML2IntegrationOptions struct {
	Name                  AccountObjectIdentifier
	Enabled               *bool
	Issuer                string
	SSOURL                string
	Provider              string
	X509Cert              string
	AllowedEmailPatterns  []string
	AllowedUserDomains    []string
	SPInitiatedLoginLabel *string
	EnableSPInitiated     *bool
	ForceAuthn            *bool
	RequestedNameIDFormat *string
	PostLogoutRedirectURL *string
	Comment               *string
}

// Validate checks that required fields are populated.
func (o *CreateSAML2IntegrationOptions) Validate() error {
	var errs []error

	if o.Name.Name() == "" {
		errs = append(errs, errors.New("name is required"))
	}

	if o.Issuer == "" {
		errs = append(errs, errors.New("saml2_issuer is required"))
	}

	if o.SSOURL == "" {
		errs = append(errs, errors.New("saml2_sso_url is required"))
	}

	if o.Provider == "" {
		errs = append(errs, errors.New("saml2_provider is required"))
	}

	if o.X509Cert == "" {
		errs = append(errs, errors.New("saml2_x509_cert is required"))
	}

	return errors.Join(errs...)
}

// AlterSAML2IntegrationOptions holds the parameters for altering a SAML2 security integration.
type AlterSAML2IntegrationOptions struct {
	Name                  AccountObjectIdentifier
	Enabled               *bool
	X509Cert              *string
	AllowedEmailPatterns  *[]string
	AllowedUserDomains    *[]string
	SPInitiatedLoginLabel *string
	EnableSPInitiated     *bool
	ForceAuthn            *bool
	RequestedNameIDFormat *string
	PostLogoutRedirectURL *string
	Comment               *string
	UnsetFields           []string
}

// HasChanges returns true if there are any SET or UNSET operations to apply.
func (o *AlterSAML2IntegrationOptions) HasChanges() bool {
	return o.Enabled != nil ||
		o.X509Cert != nil ||
		o.AllowedEmailPatterns != nil ||
		o.AllowedUserDomains != nil ||
		o.SPInitiatedLoginLabel != nil ||
		o.EnableSPInitiated != nil ||
		o.ForceAuthn != nil ||
		o.RequestedNameIDFormat != nil ||
		o.PostLogoutRedirectURL != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// Validate checks validity of the alter options.
func (o *AlterSAML2IntegrationOptions) Validate() error {
	if o.Name.Name() == "" {
		return errors.New("name is required")
	}

	return nil
}

// SAML2IntegrationClient provides operations on Snowflake SAML2 security integrations.
type SAML2IntegrationClient struct {
	client SQLExecutor
}

// NewSAML2IntegrationClient creates a new SAML2IntegrationClient.
func NewSAML2IntegrationClient(c SQLExecutor) *SAML2IntegrationClient {
	return &SAML2IntegrationClient{client: c}
}

// buildCreateSAML2IntegrationSQL builds the CREATE SECURITY INTEGRATION SQL for SAML2.
func buildCreateSAML2IntegrationSQL(opts CreateSAML2IntegrationOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", fmt.Errorf("invalid create options: %w", err)
	}

	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)
	fmt.Fprintf(&b.Builder, " TYPE = SAML2")
	b.SetString("SAML2_ISSUER", &opts.Issuer)
	b.SetString("SAML2_SSO_URL", &opts.SSOURL)
	b.SetString("SAML2_PROVIDER", &opts.Provider)
	b.SetString("SAML2_X509_CERT", &opts.X509Cert)

	if len(opts.AllowedEmailPatterns) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("ALLOWED_EMAIL_PATTERNS", opts.AllowedEmailPatterns))
	}

	if len(opts.AllowedUserDomains) > 0 {
		b.WriteString(" ")
		b.WriteString(buildStringListClause("ALLOWED_USER_DOMAINS", opts.AllowedUserDomains))
	}

	if opts.SPInitiatedLoginLabel != nil {
		b.SetString("SAML2_SP_INITIATED_LOGIN_PAGE_LABEL", opts.SPInitiatedLoginLabel)
	}

	if opts.EnableSPInitiated != nil {
		b.SetBool("SAML2_ENABLE_SP_INITIATED", opts.EnableSPInitiated)
	}

	if opts.ForceAuthn != nil {
		b.SetBool("SAML2_FORCE_AUTHN", opts.ForceAuthn)
	}

	if opts.RequestedNameIDFormat != nil {
		b.SetString("SAML2_REQUESTED_NAMEID_FORMAT", opts.RequestedNameIDFormat)
	}

	if opts.PostLogoutRedirectURL != nil {
		b.SetString("SAML2_POST_LOGOUT_REDIRECT_URL", opts.PostLogoutRedirectURL)
	}

	b.SetBool("ENABLED", opts.Enabled)
	b.SetString("COMMENT", opts.Comment)

	return b.String(), nil
}

// Create creates a SAML2 security integration in Snowflake.
func (c *SAML2IntegrationClient) Create(ctx context.Context, opts CreateSAML2IntegrationOptions) error {
	stmt, err := buildCreateSAML2IntegrationSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create SAML2 integration SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating SAML2 integration %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSAML2IntegrationStatements builds ALTER SECURITY INTEGRATION statements for SAML2.
func buildAlterSAML2IntegrationStatements(opts AlterSAML2IntegrationOptions) ([]string, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid alter options: %w", err)
	}

	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	sc.Bool("ENABLED", opts.Enabled)

	if opts.X509Cert != nil {
		sc.String("SAML2_X509_CERT", opts.X509Cert)
	}

	if opts.AllowedEmailPatterns != nil {
		sc.UnsafeRaw(buildStringListClause("ALLOWED_EMAIL_PATTERNS", *opts.AllowedEmailPatterns)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.AllowedUserDomains != nil {
		sc.UnsafeRaw(buildStringListClause("ALLOWED_USER_DOMAINS", *opts.AllowedUserDomains)) //nolint:forbidigo // values escaped via EscapeString
	}

	if opts.SPInitiatedLoginLabel != nil {
		sc.String("SAML2_SP_INITIATED_LOGIN_PAGE_LABEL", opts.SPInitiatedLoginLabel)
	}

	if opts.EnableSPInitiated != nil {
		sc.Bool("SAML2_ENABLE_SP_INITIATED", opts.EnableSPInitiated)
	}

	if opts.ForceAuthn != nil {
		sc.Bool("SAML2_FORCE_AUTHN", opts.ForceAuthn)
	}

	if opts.RequestedNameIDFormat != nil {
		sc.String("SAML2_REQUESTED_NAMEID_FORMAT", opts.RequestedNameIDFormat)
	}

	if opts.PostLogoutRedirectURL != nil {
		sc.String("SAML2_POST_LOGOUT_REDIRECT_URL", opts.PostLogoutRedirectURL)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SECURITY INTEGRATION", fqn, &sc, opts.UnsetFields)
}

// Alter alters a SAML2 security integration in Snowflake.
func (c *SAML2IntegrationClient) Alter(ctx context.Context, opts AlterSAML2IntegrationOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter SAML2 integration options: %w", err))
	}

	stmts, err := buildAlterSAML2IntegrationStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter SAML2 integration statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering SAML2 integration %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a SAML2 security integration.
func (c *SAML2IntegrationClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	stmt := sqlbuilder.DropIfExists("SECURITY INTEGRATION", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping SAML2 integration %s: %w", name, err)
	}

	return nil
}

// ShowByID retrieves a SAML2 integration from Snowflake.
func (c *SAML2IntegrationClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.SAML2IntegrationShowOutput, error) {
	stmt := sqlbuilder.ShowLike("SECURITY INTEGRATIONS", name.Name())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing SAML2 integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanSAML2IntegrationShowOutput(rows, name.Name())
}

// scanSAML2IntegrationShowOutput scans SHOW output rows into SAML2IntegrationShowOutput.
func scanSAML2IntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.SAML2IntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SAML2IntegrationShowOutput, error) {
		return &v1alpha1.SAML2IntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}

// Describe retrieves detailed SAML2 integration properties.
func (c *SAML2IntegrationClient) Describe(ctx context.Context, name AccountObjectIdentifier) (map[string]string, error) {
	stmt := fmt.Sprintf("DESCRIBE INTEGRATION %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing SAML2 integration %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a single observation.
func (c *SAML2IntegrationClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*SAML2IntegrationObservation, error) {
	showOutput, err := c.ShowByID(ctx, name)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return &SAML2IntegrationObservation{Exists: false}, nil
		}

		return nil, err
	}

	descOutput, err := c.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &SAML2IntegrationObservation{
		Exists:         true,
		ShowOutput:     showOutput,
		DescribeOutput: descOutput,
	}, nil
}
