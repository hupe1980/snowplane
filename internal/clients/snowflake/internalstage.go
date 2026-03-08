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

// InternalStageObservation holds the result of observing a Snowflake internal stage.
type InternalStageObservation struct {
	// Exists indicates whether the stage was found.
	Exists bool

	// ShowOutput contains the SHOW STAGES row.
	ShowOutput *v1alpha1.InternalStageShowOutput
}

// CreateInternalStageOptions holds the parameters for creating an internal stage.
type CreateInternalStageOptions struct {
	Name       SchemaObjectIdentifier
	Encryption *InternalStageEncryptionOptions
	Directory  *InternalStageDirectoryCreateOptions
	FileFormat *string
	Comment    *string
}

// InternalStageEncryptionOptions specifies encryption for an internal stage.
type InternalStageEncryptionOptions struct {
	Type string // SNOWFLAKE_FULL or SNOWFLAKE_SSE
}

// InternalStageDirectoryCreateOptions configures directory table settings at creation.
type InternalStageDirectoryCreateOptions struct {
	Enable          bool
	RefreshOnCreate *bool
}

// Validate checks the CreateInternalStageOptions for validity.
func (o *CreateInternalStageOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("stage name is required"))
	}

	if o.Encryption != nil {
		t := strings.ToUpper(o.Encryption.Type)
		if t != "SNOWFLAKE_FULL" && t != "SNOWFLAKE_SSE" {
			errs = append(errs, fmt.Errorf("encryption type must be SNOWFLAKE_FULL or SNOWFLAKE_SSE for internal stages, got %q", o.Encryption.Type))
		}
	}

	if o.FileFormat != nil {
		if err := sqlbuilder.ValidateFileFormat(*o.FileFormat); err != nil {
			errs = append(errs, fmt.Errorf("file format: %w", err))
		}
	}

	return errors.Join(errs...)
}

// AlterInternalStageOptions holds the parameters for altering an internal stage.
type AlterInternalStageOptions struct {
	Name       SchemaObjectIdentifier
	FileFormat *string
	Comment    *string
	Directory  *InternalStageDirectoryCreateOptions

	// UnsetFields lists Snowflake parameter names to revert to their
	// server-side defaults.
	UnsetFields []string
}

// Validate checks the AlterInternalStageOptions for validity.
func (o *AlterInternalStageOptions) Validate() error {
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
func (o *AlterInternalStageOptions) HasChanges() bool {
	return o.FileFormat != nil ||
		o.Comment != nil ||
		o.Directory != nil ||
		len(o.UnsetFields) > 0
}

// InternalStageClient provides operations against Snowflake internal stages.
type InternalStageClient struct {
	client SQLExecutor
}

// NewInternalStageClient creates a new InternalStageClient.
func NewInternalStageClient(c SQLExecutor) *InternalStageClient {
	return &InternalStageClient{client: c}
}

// buildCreateInternalStageSQL builds the CREATE STAGE SQL statement for an internal stage.
func buildCreateInternalStageSQL(opts CreateInternalStageOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "STAGE", opts.Name.FullyQualifiedName(), false, false)

	if opts.Encryption != nil {
		fmt.Fprintf(&b.Builder, " ENCRYPTION = (TYPE = '%s')", sqlbuilder.EscapeString(opts.Encryption.Type))
	}

	if opts.Directory != nil {
		dirClause := fmt.Sprintf(" DIRECTORY = (ENABLE = %s", sqlbuilder.BoolToSQL(opts.Directory.Enable))

		if opts.Directory.RefreshOnCreate != nil {
			dirClause += fmt.Sprintf(" REFRESH_ON_CREATE = %s", sqlbuilder.BoolToSQL(*opts.Directory.RefreshOnCreate))
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

// Create creates an internal stage in Snowflake.
func (s *InternalStageClient) Create(ctx context.Context, opts CreateInternalStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create internal stage options: %w", err))
	}

	sql, err := buildCreateInternalStageSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create internal stage SQL: %w", err))
	}

	if _, err := s.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating internal stage %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterInternalStageStatements builds the ALTER STAGE SQL statements.
func buildAlterInternalStageStatements(opts AlterInternalStageOptions) ([]string, error) {
	var statements []string

	fqn := opts.Name.FullyQualifiedName()

	// Handle directory separately.
	if opts.Directory != nil {
		dirClause := fmt.Sprintf("ALTER STAGE %s SET DIRECTORY = (ENABLE = %s)", fqn, sqlbuilder.BoolToSQL(opts.Directory.Enable))
		statements = append(statements, dirClause)
	}

	// Build SET clause.
	var sc sqlbuilder.SetClauses

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

// Alter alters an internal stage in Snowflake.
func (s *InternalStageClient) Alter(ctx context.Context, opts AlterInternalStageOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter internal stage options: %w", err))
	}

	stmts, err := buildAlterInternalStageStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter internal stage statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := s.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering internal stage %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an internal stage from Snowflake.
func (s *InternalStageClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("stage name is required"))
	}

	stmt := sqlbuilder.DropIfExists("STAGE", name.FullyQualifiedName())

	if _, err := s.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping internal stage %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW STAGES for a specific internal stage.
func (s *InternalStageClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.InternalStageShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("stage name is required"))
	}

	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))

	rows, err := s.client.Query(ctx, sqlbuilder.ShowLikeIn("STAGES", name.Name(), scope))
	if err != nil {
		return nil, fmt.Errorf("showing internal stage %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanInternalStageShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an InternalStageObservation.
func (s *InternalStageClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*InternalStageObservation, error) {
	show, err := s.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &InternalStageObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &InternalStageObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanInternalStageShowOutput scans SHOW STAGES results for a matching row.
func scanInternalStageShowOutput(rows *sql.Rows, name string) (*v1alpha1.InternalStageShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.InternalStageShowOutput, error) {
		return &v1alpha1.InternalStageShowOutput{
			CreatedOn:        m["created_on"],
			Name:             m["name"],
			DatabaseName:     m["database_name"],
			SchemaName:       m["schema_name"],
			Owner:            m["owner"],
			Comment:          m["comment"],
			DirectoryEnabled: strings.EqualFold(m["directory_enabled"], "Y"),
		}, nil
	})
}
