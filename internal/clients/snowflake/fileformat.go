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

// FileFormatObservation holds the result of observing a Snowflake file format.
type FileFormatObservation struct {
	// Exists indicates whether the file format was found.
	Exists bool

	// ShowOutput contains the SHOW FILE FORMATS row.
	ShowOutput *v1alpha1.FileFormatShowOutput
}

// CreateFileFormatOptions holds the parameters for creating a file format.
type CreateFileFormatOptions struct {
	Name SchemaObjectIdentifier
	Type string // CSV, JSON, AVRO, ORC, PARQUET, XML

	// UseCreateOrAlter emits CREATE OR ALTER FILE FORMAT instead of
	// CREATE FILE FORMAT IF NOT EXISTS.
	UseCreateOrAlter           bool
	FieldDelimiter             *string
	RecordDelimiter            *string
	SkipHeader                 *int32
	FieldOptionallyEnclosedBy  *string
	Escape                     *string
	EscapeUnenclosedField      *string
	EmptyFieldAsNull           *bool
	NullIf                     []string
	ErrorOnColumnCountMismatch *bool
	SkipBlankLines             *bool
	ParseHeader                *bool
	Encoding                   *string
	Compression                *string
	StripOuterArray            *bool
	StripNullValues            *bool
	EnableOctal                *bool
	AllowDuplicate             *bool
	BinaryAsText               *bool
	UseLogicalType             *bool
	SnappyCompression          *bool
	PreserveSpace              *bool
	StripOuterElement          *bool
	DisableAutoConvert         *bool
	DisableSnowflakeData       *bool
	ReplaceInvalidCharacters   *bool
	SkipByteOrderMark          *bool
	IgnoreUtf8Errors           *bool
	DateFormat                 *string
	TimeFormat                 *string
	TimestampFormat            *string
	BinaryFormat               *string
	TrimSpace                  *bool
	Comment                    *string
}

// Validate checks the CreateFileFormatOptions for validity.
func (o *CreateFileFormatOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("file format name is required"))
	}

	if o.Type == "" {
		errs = append(errs, fmt.Errorf("file format type is required"))
	}

	return errors.Join(errs...)
}

// AlterFileFormatOptions holds the parameters for altering a file format.
type AlterFileFormatOptions struct {
	Name                       SchemaObjectIdentifier
	FieldDelimiter             *string
	RecordDelimiter            *string
	SkipHeader                 *int32
	FieldOptionallyEnclosedBy  *string
	Escape                     *string
	EscapeUnenclosedField      *string
	EmptyFieldAsNull           *bool
	NullIf                     *[]string
	ErrorOnColumnCountMismatch *bool
	SkipBlankLines             *bool
	ParseHeader                *bool
	Encoding                   *string
	Compression                *string
	StripOuterArray            *bool
	StripNullValues            *bool
	EnableOctal                *bool
	AllowDuplicate             *bool
	BinaryAsText               *bool
	UseLogicalType             *bool
	SnappyCompression          *bool
	PreserveSpace              *bool
	StripOuterElement          *bool
	DisableAutoConvert         *bool
	DisableSnowflakeData       *bool
	ReplaceInvalidCharacters   *bool
	SkipByteOrderMark          *bool
	IgnoreUtf8Errors           *bool
	DateFormat                 *string
	TimeFormat                 *string
	TimestampFormat            *string
	BinaryFormat               *string
	TrimSpace                  *bool
	Comment                    *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterFileFormatOptions for validity.
func (o *AlterFileFormatOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("file format name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterFileFormatOptions) HasChanges() bool {
	return o.FieldDelimiter != nil ||
		o.RecordDelimiter != nil ||
		o.SkipHeader != nil ||
		o.FieldOptionallyEnclosedBy != nil ||
		o.Escape != nil ||
		o.EscapeUnenclosedField != nil ||
		o.EmptyFieldAsNull != nil ||
		o.NullIf != nil ||
		o.ErrorOnColumnCountMismatch != nil ||
		o.SkipBlankLines != nil ||
		o.ParseHeader != nil ||
		o.Encoding != nil ||
		o.Compression != nil ||
		o.StripOuterArray != nil ||
		o.StripNullValues != nil ||
		o.EnableOctal != nil ||
		o.AllowDuplicate != nil ||
		o.BinaryAsText != nil ||
		o.UseLogicalType != nil ||
		o.SnappyCompression != nil ||
		o.PreserveSpace != nil ||
		o.StripOuterElement != nil ||
		o.DisableAutoConvert != nil ||
		o.DisableSnowflakeData != nil ||
		o.ReplaceInvalidCharacters != nil ||
		o.SkipByteOrderMark != nil ||
		o.IgnoreUtf8Errors != nil ||
		o.DateFormat != nil ||
		o.TimeFormat != nil ||
		o.TimestampFormat != nil ||
		o.BinaryFormat != nil ||
		o.TrimSpace != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// FileFormatClient provides operations against Snowflake file formats.
type FileFormatClient struct {
	client SQLExecutor
}

// NewFileFormatClient creates a new FileFormatClient.
func NewFileFormatClient(c SQLExecutor) *FileFormatClient {
	return &FileFormatClient{client: c}
}

// buildCreateFileFormatSQL builds the CREATE FILE FORMAT SQL statement.
func buildCreateFileFormatSQL(opts CreateFileFormatOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "FILE FORMAT", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)
	fmt.Fprintf(&b.Builder, " TYPE = '%s'", opts.Type)

	// CSV-specific.
	b.SetString("FIELD_DELIMITER", opts.FieldDelimiter)
	b.SetString("RECORD_DELIMITER", opts.RecordDelimiter)
	b.SetInt32("SKIP_HEADER", opts.SkipHeader)
	b.SetString("FIELD_OPTIONALLY_ENCLOSED_BY", opts.FieldOptionallyEnclosedBy)
	b.SetString("ESCAPE", opts.Escape)
	b.SetString("ESCAPE_UNENCLOSED_FIELD", opts.EscapeUnenclosedField)
	b.SetBool("EMPTY_FIELD_AS_NULL", opts.EmptyFieldAsNull)
	b.SetBool("ERROR_ON_COLUMN_COUNT_MISMATCH", opts.ErrorOnColumnCountMismatch)
	b.SetBool("SKIP_BLANK_LINES", opts.SkipBlankLines)
	b.SetBool("PARSE_HEADER", opts.ParseHeader)
	b.SetString("ENCODING", opts.Encoding)

	// JSON-specific.
	b.SetBool("STRIP_OUTER_ARRAY", opts.StripOuterArray)
	b.SetBool("STRIP_NULL_VALUES", opts.StripNullValues)
	b.SetBool("ENABLE_OCTAL", opts.EnableOctal)
	b.SetBool("ALLOW_DUPLICATE", opts.AllowDuplicate)
	b.SetBool("DISABLE_SNOWFLAKE_DATA", opts.DisableSnowflakeData)

	// Parquet-specific.
	b.SetBool("BINARY_AS_TEXT", opts.BinaryAsText)
	b.SetBool("USE_LOGICAL_TYPE", opts.UseLogicalType)
	b.SetBool("SNAPPY_COMPRESSION", opts.SnappyCompression)

	// XML-specific.
	b.SetBool("PRESERVE_SPACE", opts.PreserveSpace)
	b.SetBool("STRIP_OUTER_ELEMENT", opts.StripOuterElement)
	b.SetBool("DISABLE_AUTO_CONVERT", opts.DisableAutoConvert)

	// Cross-format.
	b.SetBool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	b.SetBool("SKIP_BYTE_ORDER_MARK", opts.SkipByteOrderMark)
	b.SetBool("IGNORE_UTF8_ERRORS", opts.IgnoreUtf8Errors)
	b.SetString("DATE_FORMAT", opts.DateFormat)
	b.SetString("TIME_FORMAT", opts.TimeFormat)
	b.SetString("TIMESTAMP_FORMAT", opts.TimestampFormat)
	b.SetKeyword("BINARY_FORMAT", opts.BinaryFormat)

	// Common.
	b.SetKeyword("COMPRESSION", opts.Compression)
	b.SetBool("TRIM_SPACE", opts.TrimSpace)
	b.SetString("COMMENT", opts.Comment)

	if len(opts.NullIf) > 0 {
		quoted := make([]string, len(opts.NullIf))
		for i, v := range opts.NullIf {
			quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
		}

		fmt.Fprintf(&b.Builder, " NULL_IF = (%s)", strings.Join(quoted, ", "))
	}

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a file format in Snowflake.
func (ff *FileFormatClient) Create(ctx context.Context, opts CreateFileFormatOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create file format options: %w", err))
	}

	sql, err := buildCreateFileFormatSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create file format SQL: %w", err))
	}

	if _, err := ff.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating file format %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterFileFormatStatements builds the ALTER FILE FORMAT SQL statements.
func buildAlterFileFormatStatements(opts AlterFileFormatOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := opts.Name.FullyQualifiedName()

	sc.String("FIELD_DELIMITER", opts.FieldDelimiter)
	sc.String("RECORD_DELIMITER", opts.RecordDelimiter)
	sc.Int32("SKIP_HEADER", opts.SkipHeader)
	sc.String("FIELD_OPTIONALLY_ENCLOSED_BY", opts.FieldOptionallyEnclosedBy)
	sc.String("ESCAPE", opts.Escape)
	sc.String("ESCAPE_UNENCLOSED_FIELD", opts.EscapeUnenclosedField)
	sc.Bool("EMPTY_FIELD_AS_NULL", opts.EmptyFieldAsNull)
	sc.Bool("ERROR_ON_COLUMN_COUNT_MISMATCH", opts.ErrorOnColumnCountMismatch)
	sc.Bool("SKIP_BLANK_LINES", opts.SkipBlankLines)
	sc.Bool("PARSE_HEADER", opts.ParseHeader)
	sc.String("ENCODING", opts.Encoding)
	sc.Bool("STRIP_OUTER_ARRAY", opts.StripOuterArray)
	sc.Bool("STRIP_NULL_VALUES", opts.StripNullValues)
	sc.Bool("ENABLE_OCTAL", opts.EnableOctal)
	sc.Bool("ALLOW_DUPLICATE", opts.AllowDuplicate)
	sc.Bool("DISABLE_SNOWFLAKE_DATA", opts.DisableSnowflakeData)
	sc.Bool("BINARY_AS_TEXT", opts.BinaryAsText)
	sc.Bool("USE_LOGICAL_TYPE", opts.UseLogicalType)
	sc.Bool("SNAPPY_COMPRESSION", opts.SnappyCompression)
	sc.Bool("PRESERVE_SPACE", opts.PreserveSpace)
	sc.Bool("STRIP_OUTER_ELEMENT", opts.StripOuterElement)
	sc.Bool("DISABLE_AUTO_CONVERT", opts.DisableAutoConvert)
	sc.Bool("REPLACE_INVALID_CHARACTERS", opts.ReplaceInvalidCharacters)
	sc.Bool("SKIP_BYTE_ORDER_MARK", opts.SkipByteOrderMark)
	sc.Bool("IGNORE_UTF8_ERRORS", opts.IgnoreUtf8Errors)
	sc.String("DATE_FORMAT", opts.DateFormat)
	sc.String("TIME_FORMAT", opts.TimeFormat)
	sc.String("TIMESTAMP_FORMAT", opts.TimestampFormat)
	sc.Keyword("BINARY_FORMAT", opts.BinaryFormat)
	sc.Keyword("COMPRESSION", opts.Compression)
	sc.Bool("TRIM_SPACE", opts.TrimSpace)
	sc.String("COMMENT", opts.Comment)

	if opts.NullIf != nil {
		quoted := make([]string, len(*opts.NullIf))
		for i, v := range *opts.NullIf {
			quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
		}

		sc.UnsafeRaw(fmt.Sprintf("NULL_IF = (%s)", strings.Join(quoted, ", "))) //nolint:forbidigo // elements individually escaped via EscapeString
	}

	return sqlbuilder.BuildAlterStatements("FILE FORMAT", fqn, &sc, opts.UnsetFields)
}

// Alter alters a file format in Snowflake.
func (ff *FileFormatClient) Alter(ctx context.Context, opts AlterFileFormatOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter file format options: %w", err))
	}

	stmts, err := buildAlterFileFormatStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter file format statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := ff.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering file format %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a file format from Snowflake.
func (ff *FileFormatClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("file format name is required"))
	}

	stmt := sqlbuilder.DropIfExists("FILE FORMAT", name.FullyQualifiedName())

	if _, err := ff.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping file format %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW FILE FORMATS for a specific file format.
func (ff *FileFormatClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.FileFormatShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("file format name is required"))
	}

	scope := fmt.Sprintf("SCHEMA %s", sqlbuilder.QuoteIdentifier(name.DatabaseName())+"."+sqlbuilder.QuoteIdentifier(name.SchemaName()))
	stmt := sqlbuilder.ShowLikeIn("FILE FORMATS", name.Name(), scope)

	rows, err := ff.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing file format %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanFileFormatShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a FileFormatObservation.
func (ff *FileFormatClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*FileFormatObservation, error) {
	show, err := ff.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &FileFormatObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &FileFormatObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanFileFormatShowOutput scans SHOW FILE FORMATS results for a matching row.
func scanFileFormatShowOutput(rows *sql.Rows, name string) (*v1alpha1.FileFormatShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.FileFormatShowOutput, error) {
		return &v1alpha1.FileFormatShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Comment:      m["comment"],
			Type:         m["type"],
		}, nil
	})
}
