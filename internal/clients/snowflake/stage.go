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

// StageObservation holds the result of observing a Snowflake stage.
type StageObservation struct {
	// Exists indicates whether the stage was found.
	Exists bool

	// ShowOutput contains the SHOW STAGES row.
	ShowOutput *v1alpha1.StageShowOutput
}

// CreateStageOptions holds the parameters for creating a stage.
type CreateStageOptions struct {
	Name               SchemaObjectIdentifier
	URL                *string // External stages only
	StorageIntegration *string // External stages only
	Encryption         *StageEncryptionOptions
	Directory          *StageDirectoryCreateOptions
	FileFormat         *string
	Comment            *string
}

// StageEncryptionOptions specifies encryption for a stage.
type StageEncryptionOptions struct {
	Type string // SNOWFLAKE_FULL, SNOWFLAKE_SSE, AWS_SSE_S3, etc.
}

// StageDirectoryCreateOptions configures directory table settings for creation.
type StageDirectoryCreateOptions struct {
	Enable                  bool
	AutoRefresh             *bool
	RefreshOnCreate         *bool
	NotificationIntegration *string
}

// Validate checks the CreateStageOptions for validity.
func (o *CreateStageOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("stage name is required"))
	}

	if o.Encryption != nil {
		if err := sqlbuilder.ValidateEncryptionType(o.Encryption.Type); err != nil {
			errs = append(errs, fmt.Errorf("encryption: %w", err))
		}
	}

	if o.FileFormat != nil {
		if err := sqlbuilder.ValidateFileFormat(*o.FileFormat); err != nil {
			errs = append(errs, fmt.Errorf("file format: %w", err))
		}
	}

	return errors.Join(errs...)
}

// AlterStageOptions holds the parameters for altering a stage.
type AlterStageOptions struct {
	Name               SchemaObjectIdentifier
	URL                *string // External only
	StorageIntegration *string // External only
	FileFormat         *string
	Comment            *string
	Directory          *StageDirectoryCreateOptions

	// UnsetFields lists Snowflake parameter names to revert to their
	// server-side defaults (e.g. when a user removes a field from the spec).
	UnsetFields []string
}

// Validate checks the AlterStageOptions for validity.
func (o *AlterStageOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("stage name is required"))
	}

	if o.FileFormat != nil {
		if err := sqlbuilder.ValidateFileFormat(*o.FileFormat); err != nil {
			errs = append(errs, fmt.Errorf("file format: %w", err))
		}
	}

	return errors.Join(errs...)
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStageOptions) HasChanges() bool {
	return o.URL != nil ||
		o.StorageIntegration != nil ||
		o.FileFormat != nil ||
		o.Comment != nil ||
		o.Directory != nil ||
		len(o.UnsetFields) > 0
}

// StageClient provides operations against Snowflake stages.
type StageClient struct {
	client SQLExecutor
}

// NewStageClient creates a new StageClient backed by the given SQLExecutor.
func NewStageClient(c SQLExecutor) *StageClient {
	return &StageClient{client: c}
}

// buildCreateStageSQL builds the CREATE STAGE SQL statement.
func buildCreateStageSQL(opts CreateStageOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "STAGE", opts.Name.FullyQualifiedName(), false, false)

	if opts.URL != nil {
		fmt.Fprintf(&b.Builder, " URL = '%s'", sqlbuilder.EscapeString(*opts.URL))
	}

	if opts.StorageIntegration != nil {
		fmt.Fprintf(&b.Builder, " STORAGE_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.StorageIntegration))
	}

	if opts.Encryption != nil {
		// Encryption.Type is validated in Validate() — use EscapeString as defense-in-depth.
		fmt.Fprintf(&b.Builder, " ENCRYPTION = (TYPE = '%s')", sqlbuilder.EscapeString(opts.Encryption.Type))
	}

	if opts.Directory != nil {
		dir := "FALSE"
		if opts.Directory.Enable {
			dir = "TRUE"
		}

		dirClause := fmt.Sprintf(" DIRECTORY = (ENABLE = %s", dir)
		if opts.Directory.AutoRefresh != nil {
			dirClause += fmt.Sprintf(" AUTO_REFRESH = %s", sqlbuilder.BoolToSQL(*opts.Directory.AutoRefresh))
		}

		if opts.Directory.RefreshOnCreate != nil {
			dirClause += fmt.Sprintf(" REFRESH_ON_CREATE = %s", sqlbuilder.BoolToSQL(*opts.Directory.RefreshOnCreate))
		}

		if opts.Directory.NotificationIntegration != nil {
			dirClause += fmt.Sprintf(" NOTIFICATION_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.Directory.NotificationIntegration))
		}

		dirClause += ")"
		b.WriteString(dirClause)
	}

	if opts.FileFormat != nil {
		fmt.Fprintf(&b.Builder, " FILE_FORMAT = (%s)", *opts.FileFormat)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a stage in Snowflake.
func (s *StageClient) Create(ctx context.Context, opts CreateStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create stage options: %w", err))
	}

	sql, err := buildCreateStageSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create stage SQL: %w", err))
	}

	if _, err := s.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating stage %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStageStatements builds the ALTER STAGE SQL statements.
func buildAlterStageStatements(opts AlterStageOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName()

	// Handle directory separately — mirror the CREATE logic for all sub-options.
	if opts.Directory != nil {
		dir := "FALSE"
		if opts.Directory.Enable {
			dir = "TRUE"
		}

		dirClause := fmt.Sprintf("ALTER STAGE %s SET DIRECTORY = (ENABLE = %s", fqn, dir)

		if opts.Directory.AutoRefresh != nil {
			dirClause += fmt.Sprintf(" AUTO_REFRESH = %s", sqlbuilder.BoolToSQL(*opts.Directory.AutoRefresh))
		}

		if opts.Directory.NotificationIntegration != nil {
			dirClause += fmt.Sprintf(" NOTIFICATION_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.Directory.NotificationIntegration))
		}

		dirClause += ")"
		statements = append(statements, dirClause)
	}

	// Build SET clause.
	var sc sqlbuilder.SetClauses

	if opts.URL != nil {
		sc.UnsafeRaw(fmt.Sprintf("URL = '%s'", sqlbuilder.EscapeString(*opts.URL)))
	}

	if opts.StorageIntegration != nil {
		sc.UnsafeRaw(fmt.Sprintf("STORAGE_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.StorageIntegration)))
	}

	if opts.FileFormat != nil {
		sc.UnsafeRaw(fmt.Sprintf("FILE_FORMAT = (%s)", *opts.FileFormat))
	}

	sc.String("COMMENT", opts.Comment)

	if sc.HasClauses() || len(opts.UnsetFields) > 0 {
		stmts, err := sqlbuilder.BuildAlterStatements("STAGE", fqn, &sc, opts.UnsetFields)
		if err != nil {
			return nil, fmt.Errorf("building SET/UNSET clauses: %w", err)
		}

		statements = append(statements, stmts...)
	}

	return statements, nil
}

// Alter alters a stage in Snowflake.
func (s *StageClient) Alter(ctx context.Context, opts AlterStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter stage options: %w", err))
	}

	stmts, err := buildAlterStageStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter stage statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := s.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering stage %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropStageSQL builds the DROP STAGE SQL statement.
func buildDropStageSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("STAGE", name.FullyQualifiedName())
}

// Drop drops a stage from Snowflake.
func (s *StageClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("stage name is required"))
	}

	if _, err := s.client.Exec(ctx, buildDropStageSQL(name)); err != nil {
		return fmt.Errorf("dropping stage %s: %w", name, err)
	}

	return nil
}

// buildShowStageByIDSQL builds a SHOW STAGES LIKE SQL statement scoped to a schema.
func buildShowStageByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("STAGES", name.Name(), scope)
}

// ShowByID queries SHOW STAGES for a specific stage name within a schema.
func (s *StageClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.StageShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("stage name is required"))
	}

	rows, err := s.client.Query(ctx, buildShowStageByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing stage %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStageShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a StageObservation.
func (s *StageClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*StageObservation, error) {
	show, err := s.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StageObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &StageObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanStageShowOutput scans SHOW STAGES results for a matching row.
func scanStageShowOutput(rows *sql.Rows, name string) (*v1alpha1.StageShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.StageShowOutput, error) {
		return &v1alpha1.StageShowOutput{
			CreatedOn:          m["created_on"],
			Name:               m["name"],
			DatabaseName:       m["database_name"],
			SchemaName:         m["schema_name"],
			URL:                m["url"],
			Owner:              m["owner"],
			Comment:            m["comment"],
			Type:               m["type"],
			StorageIntegration: m["storage_integration"],
			DirectoryEnabled:   strings.EqualFold(m["directory_enabled"], "Y"),
		}, nil
	})
}
