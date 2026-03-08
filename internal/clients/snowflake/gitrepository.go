package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// GitRepositoryObservation holds the result of observing a Snowflake Git Repository.
type GitRepositoryObservation struct {
	// Exists indicates whether the repository was found.
	Exists bool

	// ShowOutput contains the SHOW GIT REPOSITORIES row.
	ShowOutput *v1alpha1.GitRepositoryShowOutput
}

// CreateGitRepositoryOptions holds the parameters for creating a Git Repository.
type CreateGitRepositoryOptions struct {
	Name           SchemaObjectIdentifier
	Origin         string
	APIIntegration string
	GitCredentials *string
	Comment        *string
}

// Validate checks the CreateGitRepositoryOptions for validity.
func (o *CreateGitRepositoryOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("git repository name is required"))
	}

	if o.Origin == "" {
		errs = append(errs, fmt.Errorf("origin URL is required"))
	}

	if o.APIIntegration == "" {
		errs = append(errs, fmt.Errorf("API integration is required"))
	}

	return errors.Join(errs...)
}

// AlterGitRepositoryOptions holds the parameters for altering a Git Repository.
type AlterGitRepositoryOptions struct {
	Name SchemaObjectIdentifier

	// APIIntegration sets the new API integration.
	APIIntegration *string

	// GitCredentials sets the new git credentials secret.
	GitCredentials *string

	// Comment sets a new comment.
	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string

	// Fetch triggers a FETCH of the repository from the remote.
	Fetch bool
}

// Validate checks the AlterGitRepositoryOptions for validity.
func (o *AlterGitRepositoryOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("git repository name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterGitRepositoryOptions) HasChanges() bool {
	return o.APIIntegration != nil || o.GitCredentials != nil || o.Comment != nil ||
		len(o.UnsetFields) > 0 || o.Fetch
}

// buildCreateGitRepositorySQL builds the CREATE GIT REPOSITORY SQL.
func buildCreateGitRepositorySQL(opts CreateGitRepositoryOptions) (string, error) {
	var b sqlbuilder.Builder

	b.WriteString("CREATE GIT REPOSITORY ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.WriteString(" ORIGIN = '")
	b.WriteString(sqlbuilder.EscapeString(opts.Origin))
	b.WriteString("'")

	b.WriteString(" API_INTEGRATION = ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.APIIntegration))

	if opts.GitCredentials != nil {
		b.WriteString(" GIT_CREDENTIALS = ")
		b.WriteString(sqlbuilder.QuoteIdentifierParts(*opts.GitCredentials))
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// buildAlterGitRepositoryStatements builds the ALTER GIT REPOSITORY SQL statements.
func buildAlterGitRepositoryStatements(opts AlterGitRepositoryOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var stmts []string

	// SET operations — each uses a separate ALTER statement.
	if opts.APIIntegration != nil {
		stmt := fmt.Sprintf("ALTER GIT REPOSITORY %s SET API_INTEGRATION = %s",
			fqn, sqlbuilder.QuoteIdentifier(*opts.APIIntegration))
		stmts = append(stmts, stmt)
	}

	if opts.GitCredentials != nil {
		stmt := fmt.Sprintf("ALTER GIT REPOSITORY %s SET GIT_CREDENTIALS = %s",
			fqn, sqlbuilder.QuoteIdentifierParts(*opts.GitCredentials))
		stmts = append(stmts, stmt)
	}

	if opts.Comment != nil {
		stmt := fmt.Sprintf("ALTER GIT REPOSITORY %s SET COMMENT = '%s'",
			fqn, sqlbuilder.EscapeString(*opts.Comment))
		stmts = append(stmts, stmt)
	}

	// UNSET fields.
	for _, field := range opts.UnsetFields {
		stmt := fmt.Sprintf("ALTER GIT REPOSITORY %s UNSET %s", fqn, field)
		stmts = append(stmts, stmt)
	}

	// FETCH refreshes content from remote.
	if opts.Fetch {
		stmt := fmt.Sprintf("ALTER GIT REPOSITORY %s FETCH", fqn)
		stmts = append(stmts, stmt)
	}

	return stmts, nil
}

// GitRepositoryClient provides operations against Snowflake Git Repositories.
type GitRepositoryClient struct {
	client SQLExecutor
}

// NewGitRepositoryClient creates a new GitRepositoryClient.
func NewGitRepositoryClient(c SQLExecutor) *GitRepositoryClient {
	return &GitRepositoryClient{client: c}
}

// Create creates a Git Repository in Snowflake.
func (c *GitRepositoryClient) Create(ctx context.Context, opts CreateGitRepositoryOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create git repository options: %w", err))
	}

	sql, err := buildCreateGitRepositorySQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create git repository SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating git repository %s: %w", opts.Name, err)
	}

	return nil
}

// Alter alters a Git Repository in Snowflake.
func (c *GitRepositoryClient) Alter(ctx context.Context, opts AlterGitRepositoryOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter git repository options: %w", err))
	}

	stmts, err := buildAlterGitRepositoryStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter git repository statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering git repository %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a Git Repository from Snowflake.
func (c *GitRepositoryClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("git repository name is required"))
	}

	stmt := sqlbuilder.DropIfExists("GIT REPOSITORY", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping git repository %s: %w", name, err)
	}

	return nil
}

// buildShowGitRepositoryByIDSQL builds a SHOW GIT REPOSITORIES LIKE SQL.
func buildShowGitRepositoryByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("GIT REPOSITORIES", name.Name(), scope)
}

// ShowByID queries SHOW GIT REPOSITORIES for a specific repository within a schema.
func (c *GitRepositoryClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.GitRepositoryShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("git repository name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowGitRepositoryByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing git repository %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanGitRepositoryShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a GitRepositoryObservation.
func (c *GitRepositoryClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*GitRepositoryObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &GitRepositoryObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &GitRepositoryObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanGitRepositoryShowOutput scans SHOW GIT REPOSITORIES results for a matching row.
func scanGitRepositoryShowOutput(rows *sql.Rows, name string) (*v1alpha1.GitRepositoryShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.GitRepositoryShowOutput, error) {
		return &v1alpha1.GitRepositoryShowOutput{
			CreatedOn:      m["created_on"],
			Name:           m["name"],
			DatabaseName:   m["database_name"],
			SchemaName:     m["schema_name"],
			Origin:         m["origin"],
			APIIntegration: m["api_integration"],
			GitCredentials: m["git_credentials"],
			Owner:          m["owner"],
			OwnerRoleType:  m["owner_role_type"],
			Comment:        m["comment"],
		}, nil
	})
}
