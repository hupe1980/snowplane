package snowflake

import (
	"context"
	"database/sql"
	"fmt"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// AuthenticationPolicyObservation holds the result of observing a Snowflake authentication policy.
type AuthenticationPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW AUTHENTICATION POLICIES row.
	ShowOutput *v1alpha1.AuthenticationPolicyShowOutput

	// DescribeOutput contains the DESCRIBE AUTHENTICATION POLICY output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateAuthenticationPolicyOptions holds the parameters for creating an authentication policy.
type CreateAuthenticationPolicyOptions struct {
	Name SchemaObjectIdentifier

	// UseCreateOrAlter emits CREATE OR ALTER AUTHENTICATION POLICY instead of
	// CREATE AUTHENTICATION POLICY IF NOT EXISTS.
	UseCreateOrAlter bool

	AuthenticationMethods []string
	ClientTypes           []string
	SecurityIntegrations  []string
	MfaEnrollment         *string

	// MFA sub-policy.
	MfaAllowedMethods           []string
	MfaEnforceMfaOnExternalAuth *string

	// PAT sub-policy.
	PatDefaultExpiryInDays     *int32
	PatMaxExpiryInDays         *int32
	PatNetworkPolicyEvaluation *string
	PatRequireRoleRestriction  *bool

	// Workload identity sub-policy.
	WorkloadIdentityAllowedProviders    []string
	WorkloadIdentityAllowedAwsAccounts  []string
	WorkloadIdentityAllowedAzureIssuers []string
	WorkloadIdentityAllowedOidcIssuers  []string

	Comment *string
}

// Validate checks the CreateAuthenticationPolicyOptions for validity.
func (o *CreateAuthenticationPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("authentication policy name is required")
	}

	return nil
}

// AlterAuthenticationPolicyOptions holds the parameters for altering an authentication policy.
type AlterAuthenticationPolicyOptions struct {
	Name SchemaObjectIdentifier

	AuthenticationMethods []string
	ClientTypes           []string
	SecurityIntegrations  []string
	MfaEnrollment         *string

	// MFA sub-policy.
	MfaAllowedMethods           []string
	MfaEnforceMfaOnExternalAuth *string

	// PAT sub-policy.
	PatDefaultExpiryInDays     *int32
	PatMaxExpiryInDays         *int32
	PatNetworkPolicyEvaluation *string
	PatRequireRoleRestriction  *bool

	// Workload identity sub-policy.
	WorkloadIdentityAllowedProviders    []string
	WorkloadIdentityAllowedAwsAccounts  []string
	WorkloadIdentityAllowedAzureIssuers []string
	WorkloadIdentityAllowedOidcIssuers  []string

	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterAuthenticationPolicyOptions for validity.
func (o *AlterAuthenticationPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("authentication policy name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterAuthenticationPolicyOptions) HasChanges() bool {
	return len(o.AuthenticationMethods) > 0 ||
		len(o.ClientTypes) > 0 ||
		len(o.SecurityIntegrations) > 0 ||
		o.MfaEnrollment != nil ||
		len(o.MfaAllowedMethods) > 0 ||
		o.MfaEnforceMfaOnExternalAuth != nil ||
		o.PatDefaultExpiryInDays != nil ||
		o.PatMaxExpiryInDays != nil ||
		o.PatNetworkPolicyEvaluation != nil ||
		o.PatRequireRoleRestriction != nil ||
		len(o.WorkloadIdentityAllowedProviders) > 0 ||
		len(o.WorkloadIdentityAllowedAwsAccounts) > 0 ||
		len(o.WorkloadIdentityAllowedAzureIssuers) > 0 ||
		len(o.WorkloadIdentityAllowedOidcIssuers) > 0 ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// AuthenticationPolicyClient provides operations against Snowflake authentication policies.
type AuthenticationPolicyClient struct {
	client SQLExecutor
}

// NewAuthenticationPolicyClient creates a new AuthenticationPolicyClient backed by the given SQLExecutor.
func NewAuthenticationPolicyClient(c SQLExecutor) *AuthenticationPolicyClient {
	return &AuthenticationPolicyClient{client: c}
}

// writeListClauses writes list and scalar clauses to a Builder for CREATE.
func writeListClauses(b *sqlbuilder.Builder, opts *CreateAuthenticationPolicyOptions) {
	b.SetKeywordList("AUTHENTICATION_METHODS", opts.AuthenticationMethods)
	b.SetKeywordList("CLIENT_TYPES", opts.ClientTypes)
	b.SetEscapedList("SECURITY_INTEGRATIONS", opts.SecurityIntegrations)

	b.SetKeyword("MFA_ENROLLMENT", opts.MfaEnrollment)

	// MFA sub-policy fields.
	b.SetKeywordList("MFA_AUTHENTICATION_METHODS", opts.MfaAllowedMethods)

	b.SetKeyword("ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION", opts.MfaEnforceMfaOnExternalAuth)

	// PAT sub-policy fields.
	b.SetInt32("PAT_DEFAULT_EXPIRY_IN_DAYS", opts.PatDefaultExpiryInDays)
	b.SetInt32("PAT_MAX_EXPIRY_IN_DAYS", opts.PatMaxExpiryInDays)
	b.SetKeyword("PAT_NETWORK_POLICY_EVALUATION", opts.PatNetworkPolicyEvaluation)
	b.SetBool("PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS", opts.PatRequireRoleRestriction)

	// Workload identity sub-policy fields.
	b.SetKeywordList("WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", opts.WorkloadIdentityAllowedProviders)
	b.SetEscapedList("WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", opts.WorkloadIdentityAllowedAwsAccounts)
	b.SetEscapedList("WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", opts.WorkloadIdentityAllowedAzureIssuers)
	b.SetEscapedList("WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", opts.WorkloadIdentityAllowedOidcIssuers)

	b.SetString("COMMENT", opts.Comment)
}

// buildCreateAuthenticationPolicySQL builds the CREATE AUTHENTICATION POLICY SQL statement.
func buildCreateAuthenticationPolicySQL(opts CreateAuthenticationPolicyOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "AUTHENTICATION POLICY", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

	writeListClauses(&b, &opts)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an authentication policy in Snowflake.
func (c *AuthenticationPolicyClient) Create(ctx context.Context, opts CreateAuthenticationPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create authentication policy options: %w", err))
	}

	sql, err := buildCreateAuthenticationPolicySQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create authentication policy SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating authentication policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAuthenticationPolicyStatements builds ALTER AUTHENTICATION POLICY statements.
func buildAlterAuthenticationPolicyStatements(opts AlterAuthenticationPolicyOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var sc sqlbuilder.SetClauses

	sc.KeywordList("AUTHENTICATION_METHODS", opts.AuthenticationMethods)
	sc.KeywordList("CLIENT_TYPES", opts.ClientTypes)
	sc.EscapedList("SECURITY_INTEGRATIONS", opts.SecurityIntegrations)

	sc.Keyword("MFA_ENROLLMENT", opts.MfaEnrollment)
	sc.KeywordList("MFA_AUTHENTICATION_METHODS", opts.MfaAllowedMethods)
	sc.Keyword("ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION", opts.MfaEnforceMfaOnExternalAuth)

	sc.Int32("PAT_DEFAULT_EXPIRY_IN_DAYS", opts.PatDefaultExpiryInDays)
	sc.Int32("PAT_MAX_EXPIRY_IN_DAYS", opts.PatMaxExpiryInDays)
	sc.Keyword("PAT_NETWORK_POLICY_EVALUATION", opts.PatNetworkPolicyEvaluation)
	sc.Bool("PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS", opts.PatRequireRoleRestriction)

	sc.KeywordList("WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", opts.WorkloadIdentityAllowedProviders)
	sc.EscapedList("WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", opts.WorkloadIdentityAllowedAwsAccounts)
	sc.EscapedList("WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", opts.WorkloadIdentityAllowedAzureIssuers)
	sc.EscapedList("WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", opts.WorkloadIdentityAllowedOidcIssuers)

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("AUTHENTICATION POLICY", fqn, &sc, opts.UnsetFields)
}

// Alter alters an authentication policy in Snowflake.
func (c *AuthenticationPolicyClient) Alter(ctx context.Context, opts AlterAuthenticationPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter authentication policy options: %w", err))
	}

	stmts, err := buildAlterAuthenticationPolicyStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter authentication policy statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering authentication policy %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an authentication policy from Snowflake.
func (c *AuthenticationPolicyClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("authentication policy name is required"))
	}

	stmt := sqlbuilder.DropIfExists("AUTHENTICATION POLICY", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping authentication policy %s: %w", name, err)
	}

	return nil
}

// buildShowAuthenticationPolicyByIDSQL builds a SHOW AUTHENTICATION POLICIES LIKE ... IN SCHEMA SQL statement.
func buildShowAuthenticationPolicyByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("AUTHENTICATION POLICIES", name.Name(), scope)
}

// ShowByID queries SHOW AUTHENTICATION POLICIES for a specific policy.
func (c *AuthenticationPolicyClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.AuthenticationPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("authentication policy name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowAuthenticationPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing authentication policy %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanAuthenticationPolicyShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE AUTHENTICATION POLICY and returns key-value pairs of properties.
func (c *AuthenticationPolicyClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("authentication policy name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE AUTHENTICATION POLICY %s", name.FullyQualifiedName())

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing authentication policy %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into an AuthenticationPolicyObservation.
func (c *AuthenticationPolicyClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*AuthenticationPolicyObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &AuthenticationPolicyObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := c.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &AuthenticationPolicyObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &AuthenticationPolicyObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanAuthenticationPolicyShowOutput scans SHOW AUTHENTICATION POLICIES results for a matching row.
func scanAuthenticationPolicyShowOutput(rows *sql.Rows, name string) (*v1alpha1.AuthenticationPolicyShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.AuthenticationPolicyShowOutput, error) {
		return &v1alpha1.AuthenticationPolicyShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Comment:      m["comment"],
		}, nil
	})
}
