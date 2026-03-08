package snowflake

import (
	"context"
	"database/sql"
	"fmt"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ImageRepositoryObservation holds the result of observing a Snowflake image repository.
type ImageRepositoryObservation struct {
	// Exists indicates whether the image repository was found.
	Exists bool

	// ShowOutput contains the SHOW IMAGE REPOSITORIES row.
	ShowOutput *v1alpha1.ImageRepositoryShowOutput
}

// CreateImageRepositoryOptions holds the parameters for creating an image repository.
type CreateImageRepositoryOptions struct {
	Name SchemaObjectIdentifier
}

// Validate checks the CreateImageRepositoryOptions for validity.
func (o *CreateImageRepositoryOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("image repository name is required")
	}

	return nil
}

// ImageRepositoryClient provides operations against Snowflake image repositories.
type ImageRepositoryClient struct {
	client SQLExecutor
}

// NewImageRepositoryClient creates a new ImageRepositoryClient backed by the given SQLExecutor.
func NewImageRepositoryClient(c SQLExecutor) *ImageRepositoryClient {
	return &ImageRepositoryClient{client: c}
}

// buildShowImageRepositoryByIDSQL builds a SHOW IMAGE REPOSITORIES LIKE query scoped to a schema.
func buildShowImageRepositoryByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("IMAGE REPOSITORIES", name.Name(), scope)
}

// ShowByID queries SHOW IMAGE REPOSITORIES for a specific repository within a schema.
func (c *ImageRepositoryClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.ImageRepositoryShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("image repository name is required"))
	}

	rows, err := c.client.Query(ctx, buildShowImageRepositoryByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing image repository %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanImageRepositoryShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an ImageRepositoryObservation.
func (c *ImageRepositoryClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ImageRepositoryObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) || IsObjectNotExistOrNotAuthorized(err) {
			return &ImageRepositoryObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ImageRepositoryObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanImageRepositoryShowOutput scans SHOW IMAGE REPOSITORIES results for a matching row.
func scanImageRepositoryShowOutput(rows *sql.Rows, name string) (*v1alpha1.ImageRepositoryShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ImageRepositoryShowOutput, error) {
		return &v1alpha1.ImageRepositoryShowOutput{
			CreatedOn:     m["created_on"],
			Name:          m["name"],
			DatabaseName:  m["database_name"],
			SchemaName:    m["schema_name"],
			RepositoryURL: m["repository_url"],
			Owner:         m["owner"],
		}, nil
	})
}

// Create creates an image repository in Snowflake.
func (c *ImageRepositoryClient) Create(ctx context.Context, opts CreateImageRepositoryOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create image repository options: %w", err))
	}

	q := fmt.Sprintf("CREATE IMAGE REPOSITORY %s", opts.Name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, q); err != nil {
		return fmt.Errorf("creating image repository %s: %w", opts.Name, err)
	}

	return nil
}

// Drop removes an image repository from Snowflake.
func (c *ImageRepositoryClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("image repository name is required"))
	}

	stmt := sqlbuilder.DropIfExists("IMAGE REPOSITORY", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping image repository %s: %w", name, err)
	}

	return nil
}
