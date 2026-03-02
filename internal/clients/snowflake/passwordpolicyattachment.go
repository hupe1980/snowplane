package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PasswordPolicyAttachmentObservation holds the result of observing a password policy attachment.
type PasswordPolicyAttachmentObservation struct {
	// Exists indicates whether a password policy is currently attached to the target.
	Exists bool

	// PolicyName is the fully qualified name of the currently attached policy,
	// as returned by POLICY_REFERENCES or SHOW PARAMETERS.
	PolicyName string
}

// PasswordPolicyAttachmentIdentifier uniquely identifies a password policy attachment.
type PasswordPolicyAttachmentIdentifier struct {
	// PolicyName is the fully qualified password policy name.
	PolicyName string

	// TargetType is TargetTypeAccount or "USER".
	TargetType string

	// TargetName is the user name (empty for ACCOUNT).
	TargetName string
}

// FullyQualifiedName returns a human-readable representation.
func (id PasswordPolicyAttachmentIdentifier) FullyQualifiedName() string {
	if id.TargetType == TargetTypeAccount {
		return fmt.Sprintf("PASSWORD_POLICY %s ON ACCOUNT", id.PolicyName)
	}

	return fmt.Sprintf("PASSWORD_POLICY %s ON USER %s", id.PolicyName, id.TargetName)
}

// String returns the fully qualified name.
func (id PasswordPolicyAttachmentIdentifier) String() string { return id.FullyQualifiedName() }

// SetPasswordPolicyOptions holds the parameters for attaching a password policy.
type SetPasswordPolicyOptions struct {
	PolicyName string // fully qualified policy name
	TargetType string // TargetTypeAccount or "USER"
	TargetName string // user name (for USER target)
}

// Validate checks the SetPasswordPolicyOptions.
func (o *SetPasswordPolicyOptions) Validate() error {
	if o.PolicyName == "" {
		return fmt.Errorf("policy name is required")
	}

	if o.TargetType == "" {
		return fmt.Errorf("target type is required")
	}

	if o.TargetType == "USER" && o.TargetName == "" {
		return fmt.Errorf("target name is required for USER target type")
	}

	return nil
}

// UnsetPasswordPolicyOptions holds the parameters for detaching a password policy.
type UnsetPasswordPolicyOptions struct {
	TargetType string
	TargetName string
}

// Validate checks the UnsetPasswordPolicyOptions.
func (o *UnsetPasswordPolicyOptions) Validate() error {
	if o.TargetType == "" {
		return fmt.Errorf("target type is required")
	}

	if o.TargetType == "USER" && o.TargetName == "" {
		return fmt.Errorf("target name is required for USER target type")
	}

	return nil
}

// PasswordPolicyAttachmentClient provides operations against Snowflake password policy attachments.
type PasswordPolicyAttachmentClient struct {
	client SQLExecutor
}

// NewPasswordPolicyAttachmentClient creates a new PasswordPolicyAttachmentClient.
func NewPasswordPolicyAttachmentClient(c SQLExecutor) *PasswordPolicyAttachmentClient {
	return &PasswordPolicyAttachmentClient{client: c}
}

// buildSetPasswordPolicySQL builds the ALTER ... SET PASSWORD POLICY SQL statement.
func buildSetPasswordPolicySQL(opts SetPasswordPolicyOptions) string {
	if opts.TargetType == TargetTypeAccount {
		return fmt.Sprintf("ALTER ACCOUNT SET PASSWORD POLICY %s", opts.PolicyName)
	}

	return fmt.Sprintf("ALTER USER %s SET PASSWORD POLICY %s", opts.TargetName, opts.PolicyName)
}

// buildUnsetPasswordPolicySQL builds the ALTER ... UNSET PASSWORD POLICY SQL statement.
func buildUnsetPasswordPolicySQL(opts UnsetPasswordPolicyOptions) string {
	if opts.TargetType == TargetTypeAccount {
		return "ALTER ACCOUNT UNSET PASSWORD POLICY"
	}

	return fmt.Sprintf("ALTER USER %s UNSET PASSWORD POLICY", opts.TargetName)
}

// buildPolicyReferencesSQL builds the SELECT from POLICY_REFERENCES table function.
func buildPolicyReferencesSQL(entityName, entityDomain string) string {
	return fmt.Sprintf(
		"SELECT POLICY_DB, POLICY_SCHEMA, POLICY_NAME, POLICY_KIND FROM TABLE(SNOWFLAKE.INFORMATION_SCHEMA.POLICY_REFERENCES(REF_ENTITY_NAME => '%s', REF_ENTITY_DOMAIN => '%s'))",
		strings.ReplaceAll(entityName, "'", "''"),
		entityDomain,
	)
}

// Observe queries Snowflake to check if a password policy is attached.
// For USER targets, uses POLICY_REFERENCES table function.
// For ACCOUNT targets, uses POLICY_REFERENCES with domain ACCOUNT.
func (c *PasswordPolicyAttachmentClient) Observe(ctx context.Context, id PasswordPolicyAttachmentIdentifier) (*PasswordPolicyAttachmentObservation, error) {
	entityName := id.TargetName
	entityDomain := "USER"

	if id.TargetType == TargetTypeAccount {
		entityName = ""
		entityDomain = "ACCOUNT"
	}

	rows, err := c.client.Query(ctx, buildPolicyReferencesSQL(entityName, entityDomain))
	if err != nil {
		if IsObjectNotExistOrNotAuthorized(err) || IsSQLCompilationError(err) {
			return &PasswordPolicyAttachmentObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("observing password policy attachment %s: %w", id, err)
	}
	defer closeRows(rows)

	return scanPasswordPolicyReference(rows, id.PolicyName)
}

// scanPasswordPolicyReference parses POLICY_REFERENCES results for PASSWORD_POLICY.
func scanPasswordPolicyReference(rows *sql.Rows, expectedPolicy string) (*PasswordPolicyAttachmentObservation, error) {
	for rows.Next() {
		var policyDB, policySchema, policyName, policyKind sql.NullString

		if err := rows.Scan(&policyDB, &policySchema, &policyName, &policyKind); err != nil {
			return nil, fmt.Errorf("scanning policy reference row: %w", err)
		}

		if !policyKind.Valid || !strings.EqualFold(policyKind.String, "PASSWORD_POLICY") {
			continue
		}

		if !policyName.Valid {
			continue
		}

		// Build the fully qualified name from the result.
		fqn := fmt.Sprintf(`"%s"."%s"."%s"`, policyDB.String, policySchema.String, policyName.String)

		if strings.EqualFold(fqn, expectedPolicy) {
			return &PasswordPolicyAttachmentObservation{
				Exists:     true,
				PolicyName: fqn,
			}, nil
		}

		// A different password policy is attached.
		return &PasswordPolicyAttachmentObservation{Exists: false, PolicyName: fqn}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating policy reference rows: %w", err)
	}

	return &PasswordPolicyAttachmentObservation{Exists: false}, nil
}

// IsSQLCompilationError checks if the error is a SQL compilation error.
func IsSQLCompilationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQL compilation error")
}

// SetPasswordPolicy attaches a password policy to the target.
func (c *PasswordPolicyAttachmentClient) SetPasswordPolicy(ctx context.Context, opts SetPasswordPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid set password policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildSetPasswordPolicySQL(opts)); err != nil {
		return fmt.Errorf("setting password policy %s on %s %s: %w", opts.PolicyName, opts.TargetType, opts.TargetName, err)
	}

	return nil
}

// UnsetPasswordPolicy detaches a password policy from the target.
func (c *PasswordPolicyAttachmentClient) UnsetPasswordPolicy(ctx context.Context, opts UnsetPasswordPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid unset password policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildUnsetPasswordPolicySQL(opts)); err != nil {
		return fmt.Errorf("unsetting password policy from %s %s: %w", opts.TargetType, opts.TargetName, err)
	}

	return nil
}
