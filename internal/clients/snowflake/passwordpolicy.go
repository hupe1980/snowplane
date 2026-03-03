package snowflake

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// PasswordPolicyObservation holds the result of observing a Snowflake password policy.
type PasswordPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW PASSWORD POLICIES row.
	ShowOutput *PasswordPolicyShowOutput

	// DescribeOutput contains the DESCRIBE PASSWORD POLICY output (key-value pairs).
	DescribeOutput map[string]string
}

// PasswordPolicyShowOutput contains the fields from SHOW PASSWORD POLICIES.
type PasswordPolicyShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
}

// CreatePasswordPolicyOptions holds the parameters for creating a password policy.
type CreatePasswordPolicyOptions struct {
	Name SchemaObjectIdentifier

	// UseCreateOrAlter emits CREATE OR ALTER PASSWORD POLICY instead of
	// CREATE PASSWORD POLICY IF NOT EXISTS.
	UseCreateOrAlter bool

	PasswordMinLength       *int32
	PasswordMaxLength       *int32
	PasswordMinUpperCase    *int32
	PasswordMinLowerCase    *int32
	PasswordMinNumeric      *int32
	PasswordMinSpecial      *int32
	PasswordMinAgeDays      *int32
	PasswordMaxAgeDays      *int32
	PasswordMaxRetries      *int32
	PasswordLockoutTimeMins *int32
	PasswordHistory         *int32
	Comment                 *string
}

// Validate checks the CreatePasswordPolicyOptions for validity.
func (o *CreatePasswordPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("password policy name is required")
	}

	return nil
}

// AlterPasswordPolicyOptions holds the parameters for altering a password policy.
type AlterPasswordPolicyOptions struct {
	Name                    SchemaObjectIdentifier
	PasswordMinLength       *int32
	PasswordMaxLength       *int32
	PasswordMinUpperCase    *int32
	PasswordMinLowerCase    *int32
	PasswordMinNumeric      *int32
	PasswordMinSpecial      *int32
	PasswordMinAgeDays      *int32
	PasswordMaxAgeDays      *int32
	PasswordMaxRetries      *int32
	PasswordLockoutTimeMins *int32
	PasswordHistory         *int32
	Comment                 *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterPasswordPolicyOptions for validity.
func (o *AlterPasswordPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("password policy name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterPasswordPolicyOptions) HasChanges() bool {
	return o.PasswordMinLength != nil ||
		o.PasswordMaxLength != nil ||
		o.PasswordMinUpperCase != nil ||
		o.PasswordMinLowerCase != nil ||
		o.PasswordMinNumeric != nil ||
		o.PasswordMinSpecial != nil ||
		o.PasswordMinAgeDays != nil ||
		o.PasswordMaxAgeDays != nil ||
		o.PasswordMaxRetries != nil ||
		o.PasswordLockoutTimeMins != nil ||
		o.PasswordHistory != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// PasswordPolicyClient provides operations against Snowflake password policies.
type PasswordPolicyClient struct {
	client SQLExecutor
}

// NewPasswordPolicyClient creates a new PasswordPolicyClient backed by the given SQLExecutor.
func NewPasswordPolicyClient(c SQLExecutor) *PasswordPolicyClient {
	return &PasswordPolicyClient{client: c}
}

// buildCreatePasswordPolicySQL builds the CREATE PASSWORD POLICY SQL statement.
func buildCreatePasswordPolicySQL(opts CreatePasswordPolicyOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "PASSWORD POLICY", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

	b.SetInt32("PASSWORD_MIN_LENGTH", opts.PasswordMinLength)
	b.SetInt32("PASSWORD_MAX_LENGTH", opts.PasswordMaxLength)
	b.SetInt32("PASSWORD_MIN_UPPER_CASE_CHARS", opts.PasswordMinUpperCase)
	b.SetInt32("PASSWORD_MIN_LOWER_CASE_CHARS", opts.PasswordMinLowerCase)
	b.SetInt32("PASSWORD_MIN_NUMERIC_CHARS", opts.PasswordMinNumeric)
	b.SetInt32("PASSWORD_MIN_SPECIAL_CHARS", opts.PasswordMinSpecial)
	b.SetInt32("PASSWORD_MIN_AGE_DAYS", opts.PasswordMinAgeDays)
	b.SetInt32("PASSWORD_MAX_AGE_DAYS", opts.PasswordMaxAgeDays)
	b.SetInt32("PASSWORD_MAX_RETRIES", opts.PasswordMaxRetries)
	b.SetInt32("PASSWORD_LOCKOUT_TIME_MINS", opts.PasswordLockoutTimeMins)
	b.SetInt32("PASSWORD_HISTORY", opts.PasswordHistory)
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a password policy in Snowflake.
func (pp *PasswordPolicyClient) Create(ctx context.Context, opts CreatePasswordPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create password policy options: %w", err))
	}

	sql, err := buildCreatePasswordPolicySQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create password policy SQL: %w", err))
	}

	if _, err := pp.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating password policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterPasswordPolicyStatements builds ALTER PASSWORD POLICY statements.
func buildAlterPasswordPolicyStatements(opts AlterPasswordPolicyOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := opts.Name.FullyQualifiedName()

	sc.Int32("PASSWORD_MIN_LENGTH", opts.PasswordMinLength)
	sc.Int32("PASSWORD_MAX_LENGTH", opts.PasswordMaxLength)
	sc.Int32("PASSWORD_MIN_UPPER_CASE_CHARS", opts.PasswordMinUpperCase)
	sc.Int32("PASSWORD_MIN_LOWER_CASE_CHARS", opts.PasswordMinLowerCase)
	sc.Int32("PASSWORD_MIN_NUMERIC_CHARS", opts.PasswordMinNumeric)
	sc.Int32("PASSWORD_MIN_SPECIAL_CHARS", opts.PasswordMinSpecial)
	sc.Int32("PASSWORD_MIN_AGE_DAYS", opts.PasswordMinAgeDays)
	sc.Int32("PASSWORD_MAX_AGE_DAYS", opts.PasswordMaxAgeDays)
	sc.Int32("PASSWORD_MAX_RETRIES", opts.PasswordMaxRetries)
	sc.Int32("PASSWORD_LOCKOUT_TIME_MINS", opts.PasswordLockoutTimeMins)
	sc.Int32("PASSWORD_HISTORY", opts.PasswordHistory)
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("PASSWORD POLICY", fqn, &sc, opts.UnsetFields)
}

// Alter alters a password policy in Snowflake.
func (pp *PasswordPolicyClient) Alter(ctx context.Context, opts AlterPasswordPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter password policy options: %w", err))
	}

	stmts, err := buildAlterPasswordPolicyStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter password policy statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := pp.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering password policy %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a password policy from Snowflake.
func (pp *PasswordPolicyClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("password policy name is required"))
	}

	stmt := sqlbuilder.DropIfExists("PASSWORD POLICY", name.FullyQualifiedName())

	if _, err := pp.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping password policy %s: %w", name, err)
	}

	return nil
}

// buildShowPasswordPolicyByIDSQL builds a SHOW PASSWORD POLICIES LIKE ... IN SCHEMA SQL statement.
func buildShowPasswordPolicyByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("PASSWORD POLICIES", name.Name(), scope)
}

// ShowByID queries SHOW PASSWORD POLICIES for a specific policy.
func (pp *PasswordPolicyClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*PasswordPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("password policy name is required"))
	}

	rows, err := pp.client.Query(ctx, buildShowPasswordPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing password policy %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanPasswordPolicyShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE PASSWORD POLICY and returns key-value pairs of properties.
func (pp *PasswordPolicyClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("password policy name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE PASSWORD POLICY %s", name.FullyQualifiedName())

	rows, err := pp.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing password policy %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a PasswordPolicyObservation.
func (pp *PasswordPolicyClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*PasswordPolicyObservation, error) {
	show, err := pp.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &PasswordPolicyObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := pp.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &PasswordPolicyObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &PasswordPolicyObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanPasswordPolicyShowOutput scans SHOW PASSWORD POLICIES results for a matching row.
func scanPasswordPolicyShowOutput(rows *sql.Rows, name string) (*PasswordPolicyShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*PasswordPolicyShowOutput, error) {
		return &PasswordPolicyShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Comment:      m["comment"],
		}, nil
	})
}
