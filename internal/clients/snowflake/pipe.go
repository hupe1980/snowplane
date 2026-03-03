package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// PipeObservation holds the result of observing a Snowflake pipe.
type PipeObservation struct {
	// Exists indicates whether the pipe was found.
	Exists bool

	// ShowOutput contains the SHOW PIPES row.
	ShowOutput *PipeShowOutput
}

// PipeShowOutput contains the fields from SHOW PIPES.
type PipeShowOutput struct {
	CreatedOn           string
	Name                string
	DatabaseName        string
	SchemaName          string
	Owner               string
	Comment             string
	Definition          string
	NotificationChannel string
	Integration         string
	ErrorIntegration    string
	AwsSnsTopic         string
}

// CreatePipeOptions holds the parameters for creating a pipe.
type CreatePipeOptions struct {
	Name             SchemaObjectIdentifier
	CopyStatement    string
	AutoIngest       *bool
	Integration      *string
	AwsSnsTopic      *string
	ErrorIntegration *string
	Comment          *string
}

// Validate checks the CreatePipeOptions for validity.
func (o *CreatePipeOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("pipe name is required"))
	}

	if o.CopyStatement == "" {
		errs = append(errs, fmt.Errorf("copy statement is required"))
	}

	if o.AutoIngest != nil && *o.AutoIngest && o.Integration == nil && o.AwsSnsTopic == nil {
		errs = append(errs, fmt.Errorf("integration or awsSnsTopic is required when autoIngest is true"))
	}

	return errors.Join(errs...)
}

// AlterPipeOptions holds the parameters for altering a pipe.
type AlterPipeOptions struct {
	Name SchemaObjectIdentifier

	// Comment is the new comment to set.
	Comment *string

	// ErrorIntegration is the new error integration to set.
	ErrorIntegration *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterPipeOptions for validity.
func (o *AlterPipeOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("pipe name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterPipeOptions) HasChanges() bool {
	return o.Comment != nil || o.ErrorIntegration != nil || len(o.UnsetFields) > 0
}

// PipeClient provides operations against Snowflake pipes.
type PipeClient struct {
	client SQLExecutor
}

// NewPipeClient creates a new PipeClient backed by the given SQLExecutor.
func NewPipeClient(c SQLExecutor) *PipeClient {
	return &PipeClient{client: c}
}

// buildCreatePipeSQL builds the CREATE PIPE SQL statement.
func buildCreatePipeSQL(opts CreatePipeOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "PIPE", opts.Name.FullyQualifiedName(), false, false)

	if opts.AutoIngest != nil && *opts.AutoIngest {
		b.WriteString(" AUTO_INGEST = TRUE")
	}

	b.SetString("INTEGRATION", opts.Integration)

	if opts.AwsSnsTopic != nil {
		fmt.Fprintf(&b.Builder, " AWS_SNS_TOPIC = '%s'", sqlbuilder.EscapeString(*opts.AwsSnsTopic))
	}

	b.SetString("ERROR_INTEGRATION", opts.ErrorIntegration)
	b.SetString("COMMENT", opts.Comment)

	b.WriteString(" AS ")
	b.WriteString(opts.CopyStatement)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a pipe in Snowflake.
func (p *PipeClient) Create(ctx context.Context, opts CreatePipeOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create pipe options: %w", err))
	}

	sql, err := buildCreatePipeSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create pipe SQL: %w", err))
	}

	if _, err := p.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating pipe %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterPipeStatements builds the ALTER PIPE SQL statements.
func buildAlterPipeStatements(opts AlterPipeOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)
	sc.String("ERROR_INTEGRATION", opts.ErrorIntegration)

	return sqlbuilder.BuildAlterStatements("PIPE", fqn, &sc, opts.UnsetFields)
}

// Alter alters a pipe in Snowflake.
func (p *PipeClient) Alter(ctx context.Context, opts AlterPipeOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter pipe options: %w", err))
	}

	stmts, err := buildAlterPipeStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter pipe statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := p.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering pipe %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropPipeSQL builds the DROP PIPE SQL statement.
func buildDropPipeSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("PIPE", name.FullyQualifiedName())
}

// Drop drops a pipe from Snowflake.
func (p *PipeClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("pipe name is required"))
	}

	if _, err := p.client.Exec(ctx, buildDropPipeSQL(name)); err != nil {
		return fmt.Errorf("dropping pipe %s: %w", name, err)
	}

	return nil
}

// buildShowPipeByIDSQL builds a SHOW PIPES LIKE SQL scoped to a schema.
func buildShowPipeByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("PIPES", name.Name(), scope)
}

// ShowByID queries SHOW PIPES for a specific pipe within a schema.
func (p *PipeClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*PipeShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("pipe name is required"))
	}

	rows, err := p.client.Query(ctx, buildShowPipeByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing pipe %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanPipeShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a PipeObservation.
func (p *PipeClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*PipeObservation, error) {
	show, err := p.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &PipeObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &PipeObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanPipeShowOutput scans SHOW PIPES results for a matching row.
func scanPipeShowOutput(rows *sql.Rows, name string) (*PipeShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*PipeShowOutput, error) {
		return &PipeShowOutput{
			CreatedOn:           m["created_on"],
			Name:                m["name"],
			DatabaseName:        m["database_name"],
			SchemaName:          m["schema_name"],
			Owner:               m["owner"],
			Comment:             m["comment"],
			Definition:          m["definition"],
			NotificationChannel: m["notification_channel"],
			Integration:         m["integration"],
			ErrorIntegration:    m["error_integration"],
			AwsSnsTopic:         m["aws_sns_topic"],
		}, nil
	})
}
