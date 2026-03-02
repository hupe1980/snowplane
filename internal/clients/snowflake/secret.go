package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SecretType identifies the Snowflake secret type in SQL.
type SecretType string

// Secret types for Snowflake secrets.
const (
	SecretTypeOAuth2        SecretType = "OAUTH2"
	SecretTypePassword      SecretType = "PASSWORD"
	SecretTypeGenericString SecretType = "GENERIC_STRING"
)

// SecretObservation holds the result of observing a Snowflake secret.
type SecretObservation struct {
	// Exists indicates whether the secret was found.
	Exists bool

	// ShowOutput contains the SHOW SECRETS row.
	ShowOutput *SecretShowOutput

	// DescribeOutput contains the DESCRIBE SECRET output (key-value pairs).
	DescribeOutput map[string]string
}

// SecretShowOutput contains the fields from SHOW SECRETS.
type SecretShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
	SecretType   string
	OAuthScopes  string
}

// CreateSecretOptions holds the parameters for creating a secret.
type CreateSecretOptions struct {
	Name       SchemaObjectIdentifier
	SecretType SecretType

	// OAuth fields (client credentials + authorization code grant).
	APIAuthentication           string
	OAuthScopes                 []string
	OAuthRefreshToken           string
	OAuthRefreshTokenExpiryTime string

	// Basic authentication fields.
	Username string
	Password string //nolint:gosec // G117: not a credential leak

	// Generic string field.
	SecretString string

	Comment *string
}

// Validate checks the CreateSecretOptions for validity.
func (o *CreateSecretOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("secret name is required")
	}

	if o.SecretType == "" {
		return fmt.Errorf("secret type is required")
	}

	switch o.SecretType {
	case SecretTypeOAuth2:
		if o.APIAuthentication == "" {
			return fmt.Errorf("api_authentication is required for OAuth2 secrets")
		}
	case SecretTypePassword:
		if o.Username == "" {
			return fmt.Errorf("username is required for password secrets")
		}

		if o.Password == "" {
			return fmt.Errorf("password is required for password secrets")
		}
	case SecretTypeGenericString:
		if o.SecretString == "" {
			return fmt.Errorf("secret_string is required for generic string secrets")
		}
	}

	return nil
}

// AlterSecretOptions holds the parameters for altering a secret.
type AlterSecretOptions struct {
	Name       SchemaObjectIdentifier
	SecretType SecretType

	// OAuth CC: OAuthScopes
	OAuthScopes *[]string

	// OAuth ACG: OAuthRefreshToken, OAuthRefreshTokenExpiryTime
	OAuthRefreshToken           *string
	OAuthRefreshTokenExpiryTime *string

	// Basic auth: Username, Password
	Username *string
	Password *string //nolint:gosec // G117: not a credential leak

	// Generic string: SecretString
	SecretString *string

	Comment *string

	UnsetFields []string
}

// Validate checks the AlterSecretOptions for validity.
func (o *AlterSecretOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("secret name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterSecretOptions) HasChanges() bool {
	return o.OAuthScopes != nil ||
		o.OAuthRefreshToken != nil ||
		o.OAuthRefreshTokenExpiryTime != nil ||
		o.Username != nil ||
		o.Password != nil ||
		o.SecretString != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// SecretClient provides operations against Snowflake secrets.
type SecretClient struct {
	client SQLExecutor
}

// NewSecretClient creates a new SecretClient backed by the given SQLExecutor.
func NewSecretClient(c SQLExecutor) *SecretClient {
	return &SecretClient{client: c}
}

// buildCreateSecretSQL builds the CREATE SECRET SQL statement.
func buildCreateSecretSQL(opts CreateSecretOptions) string {
	var b sqlbuilder.Builder

	b.WriteString("CREATE SECRET IF NOT EXISTS ")

	b.WriteString(opts.Name.FullyQualifiedName())

	switch opts.SecretType {
	case SecretTypeOAuth2:
		b.WriteString(" TYPE = OAUTH2")

		if opts.APIAuthentication != "" {
			_, _ = fmt.Fprintf(&b, " API_AUTHENTICATION = %s", sqlbuilder.QuoteIdentifier(opts.APIAuthentication))
		}

		if len(opts.OAuthScopes) > 0 {
			b.WriteString(" OAUTH_SCOPES = (")
			quoted := make([]string, len(opts.OAuthScopes))
			for i, s := range opts.OAuthScopes {
				quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(s))
			}
			b.WriteString(strings.Join(quoted, ", "))
			b.WriteString(")")
		}

		if opts.OAuthRefreshToken != "" {
			b.SetString("OAUTH_REFRESH_TOKEN", &opts.OAuthRefreshToken)
		}

		if opts.OAuthRefreshTokenExpiryTime != "" {
			b.SetString("OAUTH_REFRESH_TOKEN_EXPIRY_TIME", &opts.OAuthRefreshTokenExpiryTime)
		}

	case SecretTypePassword:
		b.WriteString(" TYPE = PASSWORD")

		if opts.Username != "" {
			b.SetString("USERNAME", &opts.Username)
		}

		if opts.Password != "" {
			b.SetString("PASSWORD", &opts.Password)
		}

	case SecretTypeGenericString:
		b.WriteString(" TYPE = GENERIC_STRING")

		if opts.SecretString != "" {
			b.SetString("SECRET_STRING", &opts.SecretString)
		}
	}

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a secret in Snowflake.
func (sc *SecretClient) Create(ctx context.Context, opts CreateSecretOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create secret options: %w", err))
	}

	if _, err := sc.client.Exec(ctx, buildCreateSecretSQL(opts)); err != nil {
		return fmt.Errorf("creating secret %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSecretStatements builds the ALTER SECRET SQL statements.
func buildAlterSecretStatements(opts AlterSecretOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// Type-specific SET clauses.
	switch opts.SecretType {
	case SecretTypeOAuth2:
		if opts.OAuthScopes != nil {
			quoted := make([]string, len(*opts.OAuthScopes))
			for i, s := range *opts.OAuthScopes {
				quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(s))
			}

			statements = append(statements, fmt.Sprintf("ALTER SECRET %s SET OAUTH_SCOPES = (%s)", fqn, strings.Join(quoted, ", ")))
		}

		if opts.OAuthRefreshToken != nil {
			var sc sqlbuilder.SetClauses
			sc.String("OAUTH_REFRESH_TOKEN", opts.OAuthRefreshToken)

			stmts, err := sqlbuilder.BuildAlterStatements("SECRET", fqn, &sc, nil)
			if err != nil {
				return nil, err
			}

			statements = append(statements, stmts...)
		}

		if opts.OAuthRefreshTokenExpiryTime != nil {
			var sc sqlbuilder.SetClauses
			sc.String("OAUTH_REFRESH_TOKEN_EXPIRY_TIME", opts.OAuthRefreshTokenExpiryTime)

			stmts, err := sqlbuilder.BuildAlterStatements("SECRET", fqn, &sc, nil)
			if err != nil {
				return nil, err
			}

			statements = append(statements, stmts...)
		}

	case SecretTypePassword:
		var sc sqlbuilder.SetClauses
		sc.String("USERNAME", opts.Username)
		sc.String("PASSWORD", opts.Password)

		stmts, err := sqlbuilder.BuildAlterStatements("SECRET", fqn, &sc, nil)
		if err != nil {
			return nil, err
		}

		statements = append(statements, stmts...)

	case SecretTypeGenericString:
		var sc sqlbuilder.SetClauses
		sc.String("SECRET_STRING", opts.SecretString)

		stmts, err := sqlbuilder.BuildAlterStatements("SECRET", fqn, &sc, nil)
		if err != nil {
			return nil, err
		}

		statements = append(statements, stmts...)
	}

	// Comment uses SET/UNSET via BuildAlterStatements.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("SECRET", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a secret in Snowflake.
func (sc *SecretClient) Alter(ctx context.Context, opts AlterSecretOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter secret options: %w", err))
	}

	stmts, err := buildAlterSecretStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter secret statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := sc.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering secret %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a secret from Snowflake.
func (sc *SecretClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("secret name is required"))
	}

	stmt := sqlbuilder.DropIfExists("SECRET", name.FullyQualifiedName())

	if _, err := sc.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping secret %s: %w", name, err)
	}

	return nil
}

// buildShowSecretByIDSQL builds a SHOW SECRETS LIKE ... IN SCHEMA SQL statement.
func buildShowSecretByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("SECRETS", name.Name(), scope)
}

// ShowByID queries SHOW SECRETS for a specific secret.
func (sc *SecretClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*SecretShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("secret name is required"))
	}

	rows, err := sc.client.Query(ctx, buildShowSecretByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing secret %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSecretShowOutput(rows, name.Name())
}

// Describe returns the DESCRIBE SECRET output for a specific secret.
func (sc *SecretClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("secret name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE SECRET %s", name.FullyQualifiedName())

	rows, err := sc.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing secret %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a SecretObservation.
func (sc *SecretClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*SecretObservation, error) {
	show, err := sc.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SecretObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := sc.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &SecretObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &SecretObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanSecretShowOutput scans SHOW SECRETS results for a matching row.
func scanSecretShowOutput(rows *sql.Rows, name string) (*SecretShowOutput, error) {
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

		return &SecretShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["database_name"],
			SchemaName:   colMap["schema_name"],
			Owner:        colMap["owner"],
			Comment:      colMap["comment"],
			SecretType:   colMap["secret_type"],
			OAuthScopes:  colMap["oauth_scopes"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
