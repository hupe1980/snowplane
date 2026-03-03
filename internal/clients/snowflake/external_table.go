package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// ExternalTableObservation holds the result of observing a Snowflake external table.
type ExternalTableObservation struct {
	// Exists indicates whether the external table was found.
	Exists bool

	// ShowOutput contains the SHOW EXTERNAL TABLES row.
	ShowOutput *ExternalTableShowOutput
}

// ExternalTableShowOutput contains the fields from SHOW EXTERNAL TABLES.
type ExternalTableShowOutput struct {
	CreatedOn           string
	Name                string
	DatabaseName        string
	SchemaName          string
	Invalid             string
	InvalidReason       string
	Owner               string
	Comment             string
	Stage               string
	Location            string
	FileFormatName      string
	FileFormatType      string
	Cloud               string
	Region              string
	NotificationChannel string
	LastRefreshedOn     string
	TableFormat         string
	LastRefreshDetails  string
	OwnerRoleType       string
}

// ExternalTableColumnOpt represents a column definition for CREATE EXTERNAL TABLE.
type ExternalTableColumnOpt struct {
	Name string
	Type string
	As   string // SQL expression (e.g. "value:col1::varchar")
}

// CreateExternalTableOptions holds the parameters for creating an external table.
type CreateExternalTableOptions struct {
	Name SchemaObjectIdentifier

	// Columns defines the external table column definitions.
	Columns []ExternalTableColumnOpt

	// Location is the external stage and optional path (e.g. "@DB.SCHEMA.STAGE/path/").
	Location string

	// FileFormat is the file format specification (e.g. "TYPE = PARQUET").
	FileFormat string

	// PartitionBy lists partition column names.
	PartitionBy []string

	// PartitionType is "USER_SPECIFIED" for manual partitions.
	PartitionType *string

	// Pattern is a regex pattern for file matching.
	Pattern *string

	// RefreshOnCreate controls metadata refresh on creation.
	RefreshOnCreate *bool

	// AutoRefresh controls automatic metadata refresh.
	AutoRefresh *bool

	// AwsSnsTopic is the SNS topic ARN for S3 auto-refresh.
	AwsSnsTopic *string

	// TableFormat is "DELTA" for Delta Lake tables.
	TableFormat *string

	// Integration is the notification integration name.
	Integration *string

	// Comment is the external table description.
	Comment *string
}

// Validate checks the CreateExternalTableOptions for validity.
func (o *CreateExternalTableOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("external table name is required"))
	}

	if o.Location == "" {
		errs = append(errs, fmt.Errorf("location is required"))
	} else if err := sqlbuilder.ValidateStageLocation(o.Location); err != nil {
		errs = append(errs, err)
	}

	if o.FileFormat == "" {
		errs = append(errs, fmt.Errorf("file format is required"))
	}

	for i, col := range o.Columns {
		if col.Name == "" {
			errs = append(errs, fmt.Errorf("column %d: name is required", i))
		}

		if col.Type == "" {
			errs = append(errs, fmt.Errorf("column %d: type is required", i))
		}

		if col.As == "" {
			errs = append(errs, fmt.Errorf("column %d: as expression is required", i))
		}
	}

	return errors.Join(errs...)
}

// AlterExternalTableOptions holds the parameters for altering an external table.
// ALTER EXTERNAL TABLE only supports SET AUTO_REFRESH.
type AlterExternalTableOptions struct {
	Name SchemaObjectIdentifier

	// AutoRefresh controls automatic metadata refresh.
	AutoRefresh *bool
}

// Validate checks the AlterExternalTableOptions for validity.
func (o *AlterExternalTableOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("external table name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterExternalTableOptions) HasChanges() bool {
	return o.AutoRefresh != nil
}

// ExternalTableClient provides operations against Snowflake external tables.
type ExternalTableClient struct {
	client SQLExecutor
}

// NewExternalTableClient creates a new ExternalTableClient.
func NewExternalTableClient(c SQLExecutor) *ExternalTableClient {
	return &ExternalTableClient{client: c}
}

// buildCreateExternalTableSQL builds the CREATE EXTERNAL TABLE SQL statement.
func buildCreateExternalTableSQL(opts CreateExternalTableOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "EXTERNAL TABLE", opts.Name.FullyQualifiedName(), false, false)

	// Column definitions.
	if len(opts.Columns) > 0 {
		b.WriteString(" (")

		for i, col := range opts.Columns {
			if i > 0 {
				b.WriteString(", ")
			}

			b.WriteString(sqlbuilder.QuoteIdentifier(col.Name))
			b.WriteString(" ")
			b.WriteString(col.Type)
			fmt.Fprintf(&b.Builder, " AS (%s)", col.As)
		}

		b.WriteString(")")
	}

	// Cloud provider params (Integration for GCS/Azure).
	if opts.Integration != nil {
		fmt.Fprintf(&b.Builder, " INTEGRATION = '%s'", sqlbuilder.EscapeString(*opts.Integration))
	}

	// Partition columns.
	if len(opts.PartitionBy) > 0 {
		quoted := make([]string, len(opts.PartitionBy))
		for i, col := range opts.PartitionBy {
			quoted[i] = sqlbuilder.QuoteIdentifier(col)
		}

		fmt.Fprintf(&b.Builder, " PARTITION BY (%s)", strings.Join(quoted, ", "))
	}

	// Location (required).
	fmt.Fprintf(&b.Builder, " LOCATION = %s", opts.Location)

	// Partition type.
	if opts.PartitionType != nil {
		b.SetKeyword("PARTITION_TYPE", opts.PartitionType)
	}

	// Refresh on create.
	if opts.RefreshOnCreate != nil {
		if *opts.RefreshOnCreate {
			b.WriteString(" REFRESH_ON_CREATE = TRUE")
		} else {
			b.WriteString(" REFRESH_ON_CREATE = FALSE")
		}
	}

	// Auto refresh.
	if opts.AutoRefresh != nil {
		if *opts.AutoRefresh {
			b.WriteString(" AUTO_REFRESH = TRUE")
		} else {
			b.WriteString(" AUTO_REFRESH = FALSE")
		}
	}

	// Pattern.
	if opts.Pattern != nil {
		fmt.Fprintf(&b.Builder, " PATTERN = '%s'", sqlbuilder.EscapeString(*opts.Pattern))
	}

	// File format (required).
	fmt.Fprintf(&b.Builder, " FILE_FORMAT = (%s)", opts.FileFormat)

	// AWS SNS Topic.
	if opts.AwsSnsTopic != nil {
		fmt.Fprintf(&b.Builder, " AWS_SNS_TOPIC = '%s'", sqlbuilder.EscapeString(*opts.AwsSnsTopic))
	}

	// Table format (DELTA).
	if opts.TableFormat != nil {
		fmt.Fprintf(&b.Builder, " TABLE_FORMAT = %s", *opts.TableFormat)
	}

	// Comment.
	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an external table in Snowflake.
func (c *ExternalTableClient) Create(ctx context.Context, opts CreateExternalTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create external table options: %w", err))
	}

	stmt, err := buildCreateExternalTableSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create external table SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("creating external table %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterExternalTableSQL builds the ALTER EXTERNAL TABLE SQL statement.
func buildAlterExternalTableSQL(opts AlterExternalTableOptions) string {
	fqn := opts.Name.FullyQualifiedName()

	if opts.AutoRefresh != nil {
		if *opts.AutoRefresh {
			return fmt.Sprintf("ALTER EXTERNAL TABLE IF EXISTS %s SET AUTO_REFRESH = TRUE", fqn)
		}

		return fmt.Sprintf("ALTER EXTERNAL TABLE IF EXISTS %s SET AUTO_REFRESH = FALSE", fqn)
	}

	// No changes — should not reach here, but return empty for safety.
	return ""
}

// Alter alters an external table in Snowflake.
func (c *ExternalTableClient) Alter(ctx context.Context, opts AlterExternalTableOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter external table options: %w", err))
	}

	if !opts.HasChanges() {
		return nil
	}

	stmt := buildAlterExternalTableSQL(opts)
	if stmt == "" {
		return nil
	}

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("altering external table %s: %w", opts.Name, err)
	}

	return nil
}

// Drop drops an external table from Snowflake.
func (c *ExternalTableClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("external table name is required"))
	}

	stmt := sqlbuilder.DropIfExists("EXTERNAL TABLE", name.FullyQualifiedName())

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping external table %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW EXTERNAL TABLES for a specific external table.
func (c *ExternalTableClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*ExternalTableShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("external table name is required"))
	}

	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()))
	stmt := sqlbuilder.ShowLikeIn("EXTERNAL TABLES", name.Name(), scope)

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing external table %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanExternalTableShowOutput(rows, name.Name())
}

// Observe combines ShowByID into an ExternalTableObservation.
func (c *ExternalTableClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*ExternalTableObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ExternalTableObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &ExternalTableObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanExternalTableShowOutput scans SHOW EXTERNAL TABLES results for a matching row.
func scanExternalTableShowOutput(rows *sql.Rows, name string) (*ExternalTableShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*ExternalTableShowOutput, error) {
		return &ExternalTableShowOutput{
			CreatedOn:           m["created_on"],
			Name:                m["name"],
			DatabaseName:        m["database_name"],
			SchemaName:          m["schema_name"],
			Invalid:             m["invalid"],
			InvalidReason:       m["invalid_reason"],
			Owner:               m["owner"],
			Comment:             m["comment"],
			Stage:               m["stage"],
			Location:            m["location"],
			FileFormatName:      m["file_format_name"],
			FileFormatType:      m["file_format_type"],
			Cloud:               m["cloud"],
			Region:              m["region"],
			NotificationChannel: m["notification_channel"],
			LastRefreshedOn:     m["last_refreshed_on"],
			TableFormat:         m["table_format"],
			LastRefreshDetails:  m["last_refresh_details"],
			OwnerRoleType:       m["owner_role_type"],
		}, nil
	})
}
