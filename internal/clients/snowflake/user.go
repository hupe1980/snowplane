package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// UserObservation holds the result of observing a Snowflake user.
type UserObservation struct {
	// Exists indicates whether the user was found.
	Exists bool

	// ShowOutput contains the SHOW USERS row.
	ShowOutput *UserShowOutput

	// DescribeOutput contains the DESCRIBE USER output.
	DescribeOutput *UserDescribeOutput
}

// UserShowOutput contains the fields from SHOW USERS.
type UserShowOutput struct {
	CreatedOn             string
	Name                  string
	LoginName             string
	DisplayName           string
	Email                 string
	FirstName             string
	LastName              string
	MiddleName            string
	Comment               string
	DefaultRole           string
	DefaultSecondaryRoles string
	DefaultWarehouse      string
	DefaultNamespace      string
	Owner                 string
	Disabled              bool
	MustChangePassword    bool
	HasRSAPublicKey       bool
	Type                  string
	DaysToExpiry          string
	MinsToUnlock          string
	MinsToBypassMFA       string
	DisableMFA            bool
}

// UserDescribeOutput holds additional fields from DESCRIBE USER.
type UserDescribeOutput struct {
	RSAPublicKeyFP  string
	RSAPublicKey2FP string
	NetworkPolicy   string
}

// CreateUserOptions holds the parameters for creating a user.
type CreateUserOptions struct {
	Name AccountObjectIdentifier

	// UseCreateOrAlter emits CREATE OR ALTER USER instead of
	// CREATE USER IF NOT EXISTS.
	UseCreateOrAlter bool

	LoginName             *string
	DisplayName           *string
	Email                 *string
	FirstName             *string
	LastName              *string
	MiddleName            *string
	Comment               *string
	Password              *string //nolint:gosec // G117: user creation requires password field
	RSAPublicKey          *string
	RSAPublicKey2         *string
	Type                  *string
	DefaultRole           *string
	DefaultSecondaryRoles *string
	DefaultWarehouse      *string
	DefaultNamespace      *string
	MustChangePassword    *bool
	Disabled              *bool
	DaysToExpiry          *int32
	MinsToUnlock          *int32
	MinsToBypassMFA       *int32
	NetworkPolicy         *string
	DisableMFA            *bool
}

// Validate checks the CreateUserOptions for validity.
func (o *CreateUserOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("user name is required")
	}

	if o.Type != nil {
		if err := validateUserType(*o.Type); err != nil {
			return err
		}
	}

	return nil
}

// AlterUserOptions holds the parameters for altering a user.
type AlterUserOptions struct {
	Name                  AccountObjectIdentifier
	LoginName             *string
	DisplayName           *string
	Email                 *string
	FirstName             *string
	LastName              *string
	MiddleName            *string
	Comment               *string
	Password              *string //nolint:gosec // G117: user alteration requires password field
	RSAPublicKey          *string
	RSAPublicKey2         *string
	DefaultRole           *string
	DefaultSecondaryRoles *string
	DefaultWarehouse      *string
	DefaultNamespace      *string
	MustChangePassword    *bool
	Disabled              *bool
	DaysToExpiry          *int32
	MinsToUnlock          *int32
	MinsToBypassMFA       *int32
	NetworkPolicy         *string
	DisableMFA            *bool

	// UnsetFields lists parameter names to revert via ALTER USER ... UNSET.
	UnsetFields []string
}

// Validate checks the AlterUserOptions for validity.
func (o *AlterUserOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("user name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterUserOptions) HasChanges() bool {
	return o.LoginName != nil ||
		o.DisplayName != nil ||
		o.Email != nil ||
		o.FirstName != nil ||
		o.LastName != nil ||
		o.MiddleName != nil ||
		o.Comment != nil ||
		o.Password != nil ||
		o.RSAPublicKey != nil ||
		o.RSAPublicKey2 != nil ||
		o.DefaultRole != nil ||
		o.DefaultSecondaryRoles != nil ||
		o.DefaultWarehouse != nil ||
		o.DefaultNamespace != nil ||
		o.MustChangePassword != nil ||
		o.Disabled != nil ||
		o.DaysToExpiry != nil ||
		o.MinsToUnlock != nil ||
		o.MinsToBypassMFA != nil ||
		o.NetworkPolicy != nil ||
		o.DisableMFA != nil ||
		len(o.UnsetFields) > 0
}

// validateUserType checks that the user type is a valid Snowflake user type.
func validateUserType(t string) error {
	switch t {
	case "PERSON", "SERVICE", "LEGACY_SERVICE":
		return nil
	default:
		return fmt.Errorf("user type must be PERSON, SERVICE, or LEGACY_SERVICE — got %q", t)
	}
}

// UserClient provides operations against Snowflake users.
type UserClient struct {
	client SQLExecutor
}

// NewUserClient creates a new UserClient backed by the given SQLExecutor.
func NewUserClient(c SQLExecutor) *UserClient {
	return &UserClient{client: c}
}

// buildCreateUserSQL builds the CREATE USER SQL statement.
func buildCreateUserSQL(opts CreateUserOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "USER", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

	b.SetString("PASSWORD", opts.Password)
	b.SetString("LOGIN_NAME", opts.LoginName)
	b.SetString("DISPLAY_NAME", opts.DisplayName)
	b.SetString("EMAIL", opts.Email)
	b.SetString("FIRST_NAME", opts.FirstName)
	b.SetString("LAST_NAME", opts.LastName)
	b.SetString("MIDDLE_NAME", opts.MiddleName)
	b.SetString("COMMENT", opts.Comment)
	b.SetString("RSA_PUBLIC_KEY", opts.RSAPublicKey)
	b.SetString("RSA_PUBLIC_KEY_2", opts.RSAPublicKey2)
	b.SetKeyword("TYPE", opts.Type)
	b.SetString("DEFAULT_ROLE", opts.DefaultRole)

	// DEFAULT_SECONDARY_ROLES = ('ALL') is special syntax — not a simple quoted string.
	if opts.DefaultSecondaryRoles != nil {
		if strings.EqualFold(*opts.DefaultSecondaryRoles, "ALL") {
			b.WriteString(" DEFAULT_SECONDARY_ROLES = ('ALL')")
		} else {
			fmt.Fprintf(&b.Builder, " DEFAULT_SECONDARY_ROLES = ('%s')", sqlbuilder.EscapeString(*opts.DefaultSecondaryRoles))
		}
	}

	b.SetString("DEFAULT_WAREHOUSE", opts.DefaultWarehouse)
	b.SetString("DEFAULT_NAMESPACE", opts.DefaultNamespace)
	b.SetBool("MUST_CHANGE_PASSWORD", opts.MustChangePassword)
	b.SetBool("DISABLED", opts.Disabled)
	b.SetInt32("DAYS_TO_EXPIRY", opts.DaysToExpiry)
	b.SetInt32("MINS_TO_UNLOCK", opts.MinsToUnlock)
	b.SetInt32("MINS_TO_BYPASS_MFA", opts.MinsToBypassMFA)
	b.SetString("NETWORK_POLICY", opts.NetworkPolicy)
	b.SetBool("DISABLE_MFA", opts.DisableMFA)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a user in Snowflake.
func (u *UserClient) Create(ctx context.Context, opts CreateUserOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create user options: %w", err))
	}

	sql, err := buildCreateUserSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create user SQL: %w", err))
	}

	if _, err := u.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating user %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterUserStatements builds the ALTER USER SQL statement(s).
func buildAlterUserStatements(opts AlterUserOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses

	sc.String("PASSWORD", opts.Password)
	sc.String("LOGIN_NAME", opts.LoginName)
	sc.String("DISPLAY_NAME", opts.DisplayName)
	sc.String("EMAIL", opts.Email)
	sc.String("FIRST_NAME", opts.FirstName)
	sc.String("LAST_NAME", opts.LastName)
	sc.String("MIDDLE_NAME", opts.MiddleName)
	sc.String("COMMENT", opts.Comment)
	sc.String("RSA_PUBLIC_KEY", opts.RSAPublicKey)
	sc.String("RSA_PUBLIC_KEY_2", opts.RSAPublicKey2)
	sc.String("DEFAULT_ROLE", opts.DefaultRole)

	// DEFAULT_SECONDARY_ROLES = ('ALL') is special syntax — not a simple quoted string.
	if opts.DefaultSecondaryRoles != nil {
		if strings.EqualFold(*opts.DefaultSecondaryRoles, "ALL") {
			sc.UnsafeRaw("DEFAULT_SECONDARY_ROLES = ('ALL')")
		} else {
			sc.UnsafeRaw(fmt.Sprintf("DEFAULT_SECONDARY_ROLES = ('%s')", sqlbuilder.EscapeString(*opts.DefaultSecondaryRoles)))
		}
	}

	sc.String("DEFAULT_WAREHOUSE", opts.DefaultWarehouse)
	sc.String("DEFAULT_NAMESPACE", opts.DefaultNamespace)
	sc.Bool("MUST_CHANGE_PASSWORD", opts.MustChangePassword)
	sc.Bool("DISABLED", opts.Disabled)
	sc.Int32("DAYS_TO_EXPIRY", opts.DaysToExpiry)
	sc.Int32("MINS_TO_UNLOCK", opts.MinsToUnlock)
	sc.Int32("MINS_TO_BYPASS_MFA", opts.MinsToBypassMFA)
	sc.String("NETWORK_POLICY", opts.NetworkPolicy)
	sc.Bool("DISABLE_MFA", opts.DisableMFA)

	return sqlbuilder.BuildAlterStatements("USER", opts.Name.FullyQualifiedName(), &sc, opts.UnsetFields)
}

// Alter alters a user in Snowflake.
func (u *UserClient) Alter(ctx context.Context, opts AlterUserOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter user options: %w", err))
	}

	stmts, err := buildAlterUserStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter user statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := u.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering user %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropUserSQL builds the DROP USER SQL statement.
func buildDropUserSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.DropIfExists("USER", name.FullyQualifiedName())
}

// Drop drops a user from Snowflake.
func (u *UserClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("user name is required"))
	}

	if _, err := u.client.Exec(ctx, buildDropUserSQL(name)); err != nil {
		return fmt.Errorf("dropping user %s: %w", name, err)
	}

	return nil
}

// buildShowUserByIDSQL builds the SHOW USERS LIKE SQL statement.
func buildShowUserByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("USERS", name.Name())
}

// ShowByID queries SHOW USERS for a specific user name.
func (u *UserClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*UserShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("user name is required"))
	}

	rows, err := u.client.Query(ctx, buildShowUserByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing user %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanUserShowOutput(rows, name.Name())
}

// buildDescribeUserSQL builds the DESCRIBE USER SQL statement.
func buildDescribeUserSQL(name AccountObjectIdentifier) string {
	return fmt.Sprintf("DESCRIBE USER %s", name.FullyQualifiedName())
}

// Describe queries DESCRIBE USER for additional user properties.
func (u *UserClient) Describe(ctx context.Context, name AccountObjectIdentifier) (*UserDescribeOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("user name is required"))
	}

	rows, err := u.client.Query(ctx, buildDescribeUserSQL(name))
	if err != nil {
		return nil, fmt.Errorf("describing user %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanUserDescribeOutput(rows)
}

// Observe combines ShowByID and Describe into a UserObservation.
func (u *UserClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*UserObservation, error) {
	show, err := u.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &UserObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := u.Describe(ctx, name)
	if err != nil {
		return nil, err
	}

	return &UserObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanUserShowOutput scans SHOW USERS results for a matching row.
func scanUserShowOutput(rows *sql.Rows, name string) (*UserShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*UserShowOutput, error) {
		return &UserShowOutput{
			CreatedOn:             m["created_on"],
			Name:                  m["name"],
			LoginName:             m["login_name"],
			DisplayName:           m["display_name"],
			Email:                 m["email"],
			FirstName:             m["first_name"],
			LastName:              m["last_name"],
			MiddleName:            m["middle_name"],
			Comment:               m["comment"],
			DefaultRole:           m["default_role"],
			DefaultSecondaryRoles: m["default_secondary_roles"],
			DefaultWarehouse:      m["default_warehouse"],
			DefaultNamespace:      m["default_namespace"],
			Owner:                 m["owner"],
			Disabled:              strings.EqualFold(m["disabled"], "true"),
			MustChangePassword:    strings.EqualFold(m["must_change_password"], "true"),
			HasRSAPublicKey:       strings.EqualFold(m["has_rsa_public_key"], "true"),
			Type:                  m["type"],
			DaysToExpiry:          m["days_to_expiry"],
			MinsToUnlock:          m["mins_to_unlock"],
			MinsToBypassMFA:       m["mins_to_bypass_mfa"],
			DisableMFA:            strings.EqualFold(m["disable_mfa"], "true"),
		}, nil
	})
}

// scanUserDescribeOutput scans DESCRIBE USER results.
// DESCRIBE USER returns a property-value table with columns: property, value, default, description.
func scanUserDescribeOutput(rows *sql.Rows) (*UserDescribeOutput, error) {
	result := &UserDescribeOutput{}

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
			return nil, fmt.Errorf("scanning describe row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		property := strings.ToUpper(colMap["property"])

		switch property {
		case "RSA_PUBLIC_KEY_FP":
			result.RSAPublicKeyFP = colMap["value"]
		case "RSA_PUBLIC_KEY_2_FP":
			result.RSAPublicKey2FP = colMap["value"]
		case "NETWORK_POLICY":
			result.NetworkPolicy = colMap["value"]
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating describe rows: %w", err)
	}

	return result, nil
}
