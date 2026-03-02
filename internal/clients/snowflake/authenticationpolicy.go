package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// AuthenticationPolicyObservation holds the result of observing a Snowflake authentication policy.
type AuthenticationPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW AUTHENTICATION POLICIES row.
	ShowOutput *AuthenticationPolicyShowOutput

	// DescribeOutput contains the DESCRIBE AUTHENTICATION POLICY output (key-value pairs).
	DescribeOutput map[string]string
}

// AuthenticationPolicyShowOutput contains the fields from SHOW AUTHENTICATION POLICIES.
type AuthenticationPolicyShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
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

// buildKeywordListClause formats a keyword list like AUTHENTICATION_METHODS = (PASSWORD, SAML).
func buildKeywordListClause(key string, vals []string) string {
	return fmt.Sprintf("%s = (%s)", key, strings.Join(vals, ", "))
}

// buildQuotedListClause formats a quoted list like SECURITY_INTEGRATIONS = ('INT1', 'INT2').
func buildQuotedListClause(key string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
	}

	return fmt.Sprintf("%s = (%s)", key, strings.Join(quoted, ", "))
}

// writeListClauses writes list and scalar clauses to a Builder for CREATE.
func writeListClauses(b *sqlbuilder.Builder, opts *CreateAuthenticationPolicyOptions) {
	if len(opts.AuthenticationMethods) > 0 {
		b.WriteString(" ")
		b.WriteString(buildKeywordListClause("AUTHENTICATION_METHODS", opts.AuthenticationMethods))
	}

	if len(opts.ClientTypes) > 0 {
		b.WriteString(" ")
		b.WriteString(buildKeywordListClause("CLIENT_TYPES", opts.ClientTypes))
	}

	if len(opts.SecurityIntegrations) > 0 {
		b.WriteString(" ")
		b.WriteString(buildQuotedListClause("SECURITY_INTEGRATIONS", opts.SecurityIntegrations))
	}

	b.SetKeyword("MFA_ENROLLMENT", opts.MfaEnrollment)

	// MFA sub-policy fields.
	if len(opts.MfaAllowedMethods) > 0 {
		b.WriteString(" ")
		b.WriteString(buildKeywordListClause("MFA_AUTHENTICATION_METHODS", opts.MfaAllowedMethods))
	}

	b.SetKeyword("ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION", opts.MfaEnforceMfaOnExternalAuth)

	// PAT sub-policy fields.
	b.SetInt32("PAT_DEFAULT_EXPIRY_IN_DAYS", opts.PatDefaultExpiryInDays)
	b.SetInt32("PAT_MAX_EXPIRY_IN_DAYS", opts.PatMaxExpiryInDays)
	b.SetKeyword("PAT_NETWORK_POLICY_EVALUATION", opts.PatNetworkPolicyEvaluation)
	b.SetBool("PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS", opts.PatRequireRoleRestriction)

	// Workload identity sub-policy fields.
	if len(opts.WorkloadIdentityAllowedProviders) > 0 {
		b.WriteString(" ")
		b.WriteString(buildKeywordListClause("WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", opts.WorkloadIdentityAllowedProviders))
	}

	if len(opts.WorkloadIdentityAllowedAwsAccounts) > 0 {
		b.WriteString(" ")
		b.WriteString(buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", opts.WorkloadIdentityAllowedAwsAccounts))
	}

	if len(opts.WorkloadIdentityAllowedAzureIssuers) > 0 {
		b.WriteString(" ")
		b.WriteString(buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", opts.WorkloadIdentityAllowedAzureIssuers))
	}

	if len(opts.WorkloadIdentityAllowedOidcIssuers) > 0 {
		b.WriteString(" ")
		b.WriteString(buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", opts.WorkloadIdentityAllowedOidcIssuers))
	}

	b.SetString("COMMENT", opts.Comment)
}

// buildCreateAuthenticationPolicySQL builds the CREATE AUTHENTICATION POLICY SQL statement.
func buildCreateAuthenticationPolicySQL(opts CreateAuthenticationPolicyOptions) string {
	var b sqlbuilder.Builder

	if opts.UseCreateOrAlter {
		b.WriteString("CREATE OR ALTER AUTHENTICATION POLICY ")
	} else {
		b.WriteString("CREATE AUTHENTICATION POLICY IF NOT EXISTS ")
	}

	b.WriteString(opts.Name.FullyQualifiedName())

	writeListClauses(&b, &opts)

	return b.String()
}

// Create creates an authentication policy in Snowflake.
func (c *AuthenticationPolicyClient) Create(ctx context.Context, opts CreateAuthenticationPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create authentication policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildCreateAuthenticationPolicySQL(opts)); err != nil {
		return fmt.Errorf("creating authentication policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterAuthenticationPolicyStatements builds ALTER AUTHENTICATION POLICY statements.
func buildAlterAuthenticationPolicyStatements(opts AlterAuthenticationPolicyOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var statements []string

	// Build SET clauses.
	var setClauses []string

	if len(opts.AuthenticationMethods) > 0 {
		setClauses = append(setClauses, buildKeywordListClause("AUTHENTICATION_METHODS", opts.AuthenticationMethods))
	}

	if len(opts.ClientTypes) > 0 {
		setClauses = append(setClauses, buildKeywordListClause("CLIENT_TYPES", opts.ClientTypes))
	}

	if len(opts.SecurityIntegrations) > 0 {
		setClauses = append(setClauses, buildQuotedListClause("SECURITY_INTEGRATIONS", opts.SecurityIntegrations))
	}

	if opts.MfaEnrollment != nil {
		setClauses = append(setClauses, fmt.Sprintf("MFA_ENROLLMENT = %s", *opts.MfaEnrollment))
	}

	if len(opts.MfaAllowedMethods) > 0 {
		setClauses = append(setClauses, buildKeywordListClause("MFA_AUTHENTICATION_METHODS", opts.MfaAllowedMethods))
	}

	if opts.MfaEnforceMfaOnExternalAuth != nil {
		setClauses = append(setClauses, fmt.Sprintf("ENFORCE_MFA_ON_EXTERNAL_AUTHENTICATION = %s", *opts.MfaEnforceMfaOnExternalAuth))
	}

	if opts.PatDefaultExpiryInDays != nil {
		setClauses = append(setClauses, fmt.Sprintf("PAT_DEFAULT_EXPIRY_IN_DAYS = %d", *opts.PatDefaultExpiryInDays))
	}

	if opts.PatMaxExpiryInDays != nil {
		setClauses = append(setClauses, fmt.Sprintf("PAT_MAX_EXPIRY_IN_DAYS = %d", *opts.PatMaxExpiryInDays))
	}

	if opts.PatNetworkPolicyEvaluation != nil {
		setClauses = append(setClauses, fmt.Sprintf("PAT_NETWORK_POLICY_EVALUATION = %s", *opts.PatNetworkPolicyEvaluation))
	}

	if opts.PatRequireRoleRestriction != nil {
		val := "FALSE"
		if *opts.PatRequireRoleRestriction {
			val = "TRUE"
		}

		setClauses = append(setClauses, fmt.Sprintf("PAT_REQUIRE_ROLE_RESTRICTION_FOR_SERVICE_USERS = %s", val))
	}

	if len(opts.WorkloadIdentityAllowedProviders) > 0 {
		setClauses = append(setClauses, buildKeywordListClause("WORKLOAD_IDENTITY_ALLOWED_PROVIDERS", opts.WorkloadIdentityAllowedProviders))
	}

	if len(opts.WorkloadIdentityAllowedAwsAccounts) > 0 {
		setClauses = append(setClauses, buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_AWS_ACCOUNTS", opts.WorkloadIdentityAllowedAwsAccounts))
	}

	if len(opts.WorkloadIdentityAllowedAzureIssuers) > 0 {
		setClauses = append(setClauses, buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_AZURE_ISSUERS", opts.WorkloadIdentityAllowedAzureIssuers))
	}

	if len(opts.WorkloadIdentityAllowedOidcIssuers) > 0 {
		setClauses = append(setClauses, buildQuotedListClause("WORKLOAD_IDENTITY_ALLOWED_OIDC_ISSUERS", opts.WorkloadIdentityAllowedOidcIssuers))
	}

	if opts.Comment != nil {
		setClauses = append(setClauses, fmt.Sprintf("COMMENT = '%s'", sqlbuilder.EscapeString(*opts.Comment)))
	}

	if len(setClauses) > 0 {
		statements = append(statements, fmt.Sprintf("ALTER AUTHENTICATION POLICY %s SET %s", fqn, strings.Join(setClauses, " ")))
	}

	// Build UNSET statement.
	if len(opts.UnsetFields) > 0 {
		statements = append(statements, fmt.Sprintf("ALTER AUTHENTICATION POLICY %s UNSET %s", fqn, strings.Join(opts.UnsetFields, ", ")))
	}

	return statements, nil
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
func (c *AuthenticationPolicyClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*AuthenticationPolicyShowOutput, error) {
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
func scanAuthenticationPolicyShowOutput(rows *sql.Rows, name string) (*AuthenticationPolicyShowOutput, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		if !strings.EqualFold(colMap["name"], name) {
			continue
		}

		return &AuthenticationPolicyShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["database_name"],
			SchemaName:   colMap["schema_name"],
			Owner:        colMap["owner"],
			Comment:      colMap["comment"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
