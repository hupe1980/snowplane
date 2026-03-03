package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// SequenceObservation holds the result of observing a Snowflake sequence.
type SequenceObservation struct {
	// Exists indicates whether the sequence was found.
	Exists bool

	// ShowOutput contains the SHOW SEQUENCES row.
	ShowOutput *SequenceShowOutput
}

// SequenceShowOutput contains the fields from SHOW SEQUENCES.
type SequenceShowOutput struct {
	CreatedOn    string
	Name         string
	DatabaseName string
	SchemaName   string
	Owner        string
	Comment      string
	NextValue    string
	Interval     string
	Ordering     string
}

// CreateSequenceOptions holds the parameters for creating a sequence.
type CreateSequenceOptions struct {
	Name SchemaObjectIdentifier

	// UseCreateOrAlter emits CREATE OR ALTER SEQUENCE instead of
	// CREATE SEQUENCE IF NOT EXISTS.
	UseCreateOrAlter bool

	// Start is the initial value. Immutable after creation.
	// +optional
	Start *int64

	// Increment is the step interval.
	// +optional
	Increment *int64

	// Ordering is ORDER or NOORDER.
	// +optional
	Ordering *string

	// Comment is the sequence comment.
	// +optional
	Comment *string
}

// Validate checks the CreateSequenceOptions for validity.
func (o *CreateSequenceOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("sequence name is required"))
	}

	return errors.Join(errs...)
}

// AlterSequenceOptions holds the parameters for altering a sequence.
type AlterSequenceOptions struct {
	Name SchemaObjectIdentifier

	// Increment is the step interval.
	// +optional
	Increment *int64

	// Ordering is ORDER or NOORDER.
	// +optional
	Ordering *string

	// Comment is the sequence comment.
	// +optional
	Comment *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterSequenceOptions for validity.
func (o *AlterSequenceOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("sequence name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterSequenceOptions) HasChanges() bool {
	return o.Increment != nil ||
		o.Ordering != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// SequenceClient provides operations against Snowflake sequences.
type SequenceClient struct {
	client SQLExecutor
}

// NewSequenceClient creates a new SequenceClient.
func NewSequenceClient(c SQLExecutor) *SequenceClient {
	return &SequenceClient{client: c}
}

// buildCreateSequenceSQL builds the CREATE SEQUENCE SQL statement.
func buildCreateSequenceSQL(opts CreateSequenceOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "SEQUENCE", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)

	if opts.Start != nil {
		fmt.Fprintf(&b.Builder, " START = %d", *opts.Start)
	}

	if opts.Increment != nil {
		fmt.Fprintf(&b.Builder, " INCREMENT = %d", *opts.Increment)
	}

	if opts.Ordering != nil {
		b.WriteString(" " + *opts.Ordering)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a sequence in Snowflake.
func (sc *SequenceClient) Create(ctx context.Context, opts CreateSequenceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create sequence options: %w", err))
	}

	sql, err := buildCreateSequenceSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create sequence SQL: %w", err))
	}

	if _, err := sc.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating sequence %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterSequenceStatements builds the ALTER SEQUENCE SQL statements.
func buildAlterSequenceStatements(opts AlterSequenceOptions) ([]string, error) {
	var sc sqlbuilder.SetClauses
	fqn := opts.Name.FullyQualifiedName()

	if opts.Increment != nil {
		sc.UnsafeRaw(fmt.Sprintf("INCREMENT BY = %d", *opts.Increment))
	}

	if opts.Ordering != nil {
		// ORDER/NOORDER are keywords, not SET parameters.
		sc.UnsafeRaw(*opts.Ordering)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("SEQUENCE", fqn, &sc, opts.UnsetFields)
}

// Alter alters a sequence in Snowflake.
func (sc *SequenceClient) Alter(ctx context.Context, opts AlterSequenceOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter sequence options: %w", err))
	}

	stmts, err := buildAlterSequenceStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter sequence statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := sc.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering sequence %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a sequence from Snowflake.
func (sc *SequenceClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("sequence name is required"))
	}

	stmt := sqlbuilder.DropIfExists("SEQUENCE", name.FullyQualifiedName())

	if _, err := sc.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping sequence %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW SEQUENCES for a specific sequence.
func (sc *SequenceClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*SequenceShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("sequence name is required"))
	}

	scope := fmt.Sprintf("SCHEMA %s", sqlbuilder.QuoteIdentifier(name.DatabaseName())+"."+sqlbuilder.QuoteIdentifier(name.SchemaName()))
	stmt := sqlbuilder.ShowLikeIn("SEQUENCES", name.Name(), scope)

	rows, err := sc.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("showing sequence %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanSequenceShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a SequenceObservation.
func (sc *SequenceClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*SequenceObservation, error) {
	show, err := sc.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &SequenceObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &SequenceObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanSequenceShowOutput scans SHOW SEQUENCES results for a matching row.
func scanSequenceShowOutput(rows *sql.Rows, name string) (*SequenceShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*SequenceShowOutput, error) {
		return &SequenceShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Comment:      m["comment"],
			NextValue:    m["next_value"],
			Interval:     m["interval"],
			Ordering:     m["ordered"],
		}, nil
	})
}
