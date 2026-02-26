package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// StreamSourceType mirrors the API-level stream source type enum.
type StreamSourceType string

// StreamSourceType constants define the supported stream source types.
const (
	StreamSourceTable         StreamSourceType = "TABLE"
	StreamSourceView          StreamSourceType = "VIEW"
	StreamSourceExternalTable StreamSourceType = "EXTERNAL_TABLE"
	StreamSourceStage         StreamSourceType = "STAGE"
	StreamSourceDynamicTable  StreamSourceType = "DYNAMIC_TABLE"
)

// StreamObservation holds the result of observing a Snowflake stream.
type StreamObservation struct {
	// Exists indicates whether the stream was found.
	Exists bool

	// ShowOutput contains the SHOW STREAMS row.
	ShowOutput *StreamShowOutput
}

// StreamShowOutput contains the fields from SHOW STREAMS.
type StreamShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
	TableName    string // fully qualified source object name
	SourceType   string // TABLE, VIEW, STAGE, etc.
	Mode         string // DEFAULT, APPEND_ONLY, INSERT_ONLY
	Stale        bool
	StaleAfter   string
}

// CreateStreamOptions holds the parameters for creating a stream.
type CreateStreamOptions struct {
	Name            SchemaObjectIdentifier
	SourceType      StreamSourceType
	SourceName      string // fully qualified source name
	AppendOnly      *bool
	InsertOnly      *bool
	ShowInitialRows *bool
	Comment         *string
}

// Validate checks the CreateStreamOptions for validity.
func (o *CreateStreamOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("stream name is required"))
	}

	if o.SourceName == "" {
		errs = append(errs, fmt.Errorf("stream source name is required"))
	}

	return errors.Join(errs...)
}

// AlterStreamOptions holds the parameters for altering a stream.
type AlterStreamOptions struct {
	Name SchemaObjectIdentifier

	// Comment is the new comment to set.
	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterStreamOptions for validity.
func (o *AlterStreamOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("stream name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterStreamOptions) HasChanges() bool {
	return o.Comment != nil || len(o.UnsetFields) > 0
}

// StreamClient provides operations against Snowflake streams.
type StreamClient struct {
	client SQLExecutor
}

// NewStreamClient creates a new StreamClient backed by the given SQLExecutor.
func NewStreamClient(c SQLExecutor) *StreamClient {
	return &StreamClient{client: c}
}

// sourceTypeKeyword converts a StreamSourceType to the ON <type> clause keyword.
func sourceTypeKeyword(st StreamSourceType) string {
	switch st {
	case StreamSourceExternalTable:
		return "EXTERNAL TABLE"
	case StreamSourceDynamicTable:
		return "DYNAMIC TABLE"
	default:
		return string(st)
	}
}

// buildCreateStreamSQL builds the CREATE STREAM SQL statement.
func buildCreateStreamSQL(opts CreateStreamOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("CREATE STREAM IF NOT EXISTS ")
	b.WriteString(opts.Name.FullyQualifiedName())

	b.WriteString(" ON ")
	b.WriteString(sourceTypeKeyword(opts.SourceType))
	b.WriteString(" ")
	b.WriteString(opts.SourceName)

	if opts.AppendOnly != nil && *opts.AppendOnly {
		b.WriteString(" APPEND_ONLY = TRUE")
	}

	if opts.InsertOnly != nil && *opts.InsertOnly {
		b.WriteString(" INSERT_ONLY = TRUE")
	}

	if opts.ShowInitialRows != nil && *opts.ShowInitialRows {
		b.WriteString(" SHOW_INITIAL_ROWS = TRUE")
	}

	// COMMENT must appear after ON <source> and mode options per Snowflake syntax.
	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a stream in Snowflake.
func (s *StreamClient) Create(ctx context.Context, opts CreateStreamOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create stream options: %w", err))
	}

	if _, err := s.client.Exec(ctx, buildCreateStreamSQL(opts)); err != nil {
		return fmt.Errorf("creating stream %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterStreamStatements builds the ALTER STREAM SQL statements.
func buildAlterStreamStatements(opts AlterStreamOptions) ([]string, error) {
	fqn := opts.Name.FullyQualifiedName()

	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("STREAM", fqn, &sc, opts.UnsetFields)
}

// Alter alters a stream in Snowflake. Only comment can be altered.
func (s *StreamClient) Alter(ctx context.Context, opts AlterStreamOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter stream options: %w", err))
	}

	stmts, err := buildAlterStreamStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter stream statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := s.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering stream %s: %w", opts.Name, err)
		}
	}

	return nil
}

// buildDropStreamSQL builds the DROP STREAM SQL statement.
func buildDropStreamSQL(name SchemaObjectIdentifier) string {
	return sqlbuilder.DropIfExists("STREAM", name.FullyQualifiedName())
}

// Drop drops a stream from Snowflake.
func (s *StreamClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("stream name is required"))
	}

	if _, err := s.client.Exec(ctx, buildDropStreamSQL(name)); err != nil {
		return fmt.Errorf("dropping stream %s: %w", name, err)
	}

	return nil
}

// buildShowStreamByIDSQL builds a SHOW STREAMS LIKE SQL scoped to a schema.
func buildShowStreamByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	return sqlbuilder.ShowLikeIn("STREAMS", name.Name(), scope)
}

// ShowByID queries SHOW STREAMS for a specific stream within a schema.
func (s *StreamClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*StreamShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("stream name is required"))
	}

	rows, err := s.client.Query(ctx, buildShowStreamByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing stream %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanStreamShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a StreamObservation.
func (s *StreamClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*StreamObservation, error) {
	show, err := s.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &StreamObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &StreamObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanStreamShowOutput scans SHOW STREAMS results for a matching row.
func scanStreamShowOutput(rows *sql.Rows, name string) (*StreamShowOutput, error) {
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

		return &StreamShowOutput{
			CreatedOn:    colMap["created_on"],
			Name:         colMap["name"],
			DatabaseName: colMap["database_name"],
			SchemaName:   colMap["schema_name"],
			Owner:        colMap["owner"],
			Comment:      colMap["comment"],
			TableName:    colMap["table_name"],
			SourceType:   colMap["source_type"],
			Mode:         colMap["mode"],
			Stale:        strings.EqualFold(colMap["stale"], "true"),
			StaleAfter:   colMap["stale_after"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
