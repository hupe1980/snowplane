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

// ExternalStageObservation holds the result of observing a Snowflake external stage.
type ExternalStageObservation struct {
	// Exists indicates whether the stage was found.
	Exists bool

	// ShowOutput contains the SHOW STAGES row.
	ShowOutput *v1alpha1.ExternalStageShowOutput
}

// CreateExternalStageOptions holds the parameters for creating an external stage.
type CreateExternalStageOptions struct {
	Name               SchemaObjectIdentifier
	URL                string
	StorageIntegration *string
	Encryption         *ExternalStageEncryptionOptions
	Directory          *ExternalStageDirectoryCreateOptions
	FileFormat         *string
	Comment            *string
}

// ExternalStageEncryptionOptions specifies encryption for an external stage.
type ExternalStageEncryptionOptions struct {
	Type string // AWS_CSE, AWS_SSE_S3, AWS_SSE_KMS, GCS_SSE_KMS, AZURE_CSE, NONE
}

// ExternalStageDirectoryCreateOptions configures directory table settings at creation.
type ExternalStageDirectoryCreateOptions struct {
	Enable                  bool
	AutoRefresh             *bool
	RefreshOnCreate         *bool
	NotificationIntegration *string
}

// Validate checks the CreateExternalStageOptions for validity.
func (o *CreateExternalStageOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("stage name is required"))
	}

	if o.URL == "" {
		errs = append(errs, fmt.Errorf("url is required for external stages"))
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

// AlterExternalStageOptions holds the parameters for altering an external stage.
type AlterExternalStageOptions struct {
	Name               SchemaObjectIdentifier
	StorageIntegration *string
	FileFormat         *string
	Comment            *string
	Directory          *ExternalStageDirectoryCreateOptions

	// UnsetFields lists Snowflake parameter names to revert to their
	// server-side defaults.
	UnsetFields []string
}

// Validate checks the AlterExternalStageOptions for validity.
func (o *AlterExternalStageOptions) Validate() error {
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
func (o *AlterExternalStageOptions) HasChanges() bool {
	return o.StorageIntegration != nil ||
		o.FileFormat != nil ||
		o.Comment != nil ||
		o.Directory != nil ||
		len(o.UnsetFields) > 0
}

// ExternalStageClient provides operations against Snowflake external stages.
type ExternalStageClient struct {
	client SQLExecutor
}

// NewExternalStageClient creates a new ExternalStageClient.
func NewExternalStageClient(c SQLExecutor) *ExternalStageClient {
	return &ExternalStageClient{client: c}
}

// buildCreateExternalStageSQL builds the CREATE STAGE SQL statement for an external stage.
func buildCreateExternalStageSQL(opts CreateExternalStageOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "STAGE", opts.Name.FullyQualifiedName(), false, false)

	fmt.Fprintf(&b.Builder, " URL = '%s'", sqlbuilder.EscapeString(opts.URL))

	if opts.StorageIntegration != nil {
		fmt.Fprintf(&b.Builder, " STORAGE_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.StorageIntegration))
	}

	if opts.Encryption != nil {
		fmt.Fprintf(&b.Builder, " ENCRYPTION = (TYPE = '%s')", sqlbuilder.EscapeString(opts.Encryption.Type))
	}

	if opts.Directory != nil {
		dirClause := fmt.Sprintf(" DIRECTORY = (ENABLE = %s", sqlbuilder.BoolToSQL(opts.Directory.Enable))

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

// Create creates an external stage in Snowflake.
func (s *ExternalStageClient) Create(ctx context.Context, opts CreateExternalStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create external stage options: %w", err))
	}

	sql, err := buildCreateExternalStageSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create external stage SQL: %w", err))
	}

	if _, err := s.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating external stage %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterExternalStageStatements builds the ALTER STAGE SQL statements.
func buildAlterExternalStageStatements(opts AlterExternalStageOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName()

	// Handle directory separately.
	if opts.Directory != nil {
		dirClause := fmt.Sprintf("ALTER STAGE %s SET DIRECTORY = (ENABLE = %s", fqn, sqlbuilder.BoolToSQL(opts.Directory.Enable))

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

	if opts.StorageIntegration != nil {
		sc.UnsafeRaw(fmt.Sprintf("STORAGE_INTEGRATION = %s", sqlbuilder.QuoteIdentifier(*opts.StorageIntegration))) //nolint:forbidigo // identifier safely quoted
	}

	if opts.FileFormat != nil {
		sc.UnsafeRaw(fmt.Sprintf("FILE_FORMAT = (%s)", *opts.FileFormat)) //nolint:forbidigo // format validated by CRD spec
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

// Alter alters an external stage in Snowflake.
func (s *ExternalStageClient) Alter(ctx context.Context, opts AlterExternalStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter external stage options: %w", err))
	}

	stmts, err := buildAlterExternalStageStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter external stage statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := s.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering external stage %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an external stage from Snowflake.
func (s *ExternalStageClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("stage name is required"))
	}

	stmt := sqlbuilder.DropIfExists("STAGE", name.FullyQualifiedName())

	if _, err := s.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping external stage %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW STAGES for a specific external stage.
func (s *ExternalStageClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.ExternalStageShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("stage name is required"))
	}

	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))

	rows, err := s.client.Query(ctx, sqlbuilder.ShowLikeIn("STAGES", name.Name(), scope))
	if err != nil {
		return nil, fmt.Errorf("showing external stage %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanExternalStageShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an ExternalStageObservation.
func (s *ExternalStageClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ExternalStageObservation, error) {
	show, err := s.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ExternalStageObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ExternalStageObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanExternalStageShowOutput scans SHOW STAGES results for a matching row.
func scanExternalStageShowOutput(rows *sql.Rows, name string) (*v1alpha1.ExternalStageShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ExternalStageShowOutput, error) {
		return &v1alpha1.ExternalStageShowOutput{
			CreatedOn:          m["created_on"],
			Name:               m["name"],
			DatabaseName:       m["database_name"],
			SchemaName:         m["schema_name"],
			URL:                m["url"],
			Owner:              m["owner"],
			Comment:            m["comment"],
			StorageIntegration: m["storage_integration"],
			DirectoryEnabled:   strings.EqualFold(m["directory_enabled"], "Y"),
		}, nil
	})
}
