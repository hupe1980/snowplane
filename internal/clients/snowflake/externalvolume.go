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

// ExternalVolumeObservation holds the result of observing an external volume.
type ExternalVolumeObservation struct {
	// Exists indicates whether the external volume was found.
	Exists bool

	// ShowOutput contains the SHOW EXTERNAL VOLUMES row.
	ShowOutput *v1alpha1.ExternalVolumeShowOutput

	// StorageLocationNames contains the names of storage locations from DESCRIBE.
	StorageLocationNames []string
}

// ExternalVolumeStorageLocationOption holds the parameters for a single storage location.
type ExternalVolumeStorageLocationOption struct {
	Name                 string
	StorageProvider      string
	StorageBaseURL       string
	StorageAWSRoleARN    *string
	StorageAWSExternalID *string
	EncryptionType       *string
	EncryptionKMSKeyID   *string
	AzureTenantID        *string
}

// CreateExternalVolumeOptions holds the parameters for creating an external volume.
type CreateExternalVolumeOptions struct {
	Name             AccountObjectIdentifier
	StorageLocations []ExternalVolumeStorageLocationOption
	AllowWrites      *bool
	Comment          *string
}

// Validate checks the options for validity.
func (o *CreateExternalVolumeOptions) Validate() error {
	var errs []error

	if !ValidObjectIdentifier(o.Name) {
		errs = append(errs, fmt.Errorf("external volume name is required"))
	}

	if len(o.StorageLocations) == 0 {
		errs = append(errs, fmt.Errorf("at least one storage location is required"))
	}

	for i, loc := range o.StorageLocations {
		if loc.Name == "" {
			errs = append(errs, fmt.Errorf("storage location %d: name is required", i))
		}

		if loc.StorageBaseURL == "" {
			errs = append(errs, fmt.Errorf("storage location %d: storageBaseURL is required", i))
		}

		if loc.StorageProvider == "" {
			errs = append(errs, fmt.Errorf("storage location %d: storageProvider is required", i))
		}
	}

	return errors.Join(errs...)
}

// AlterExternalVolumeOptions holds the parameters for altering an external volume.
type AlterExternalVolumeOptions struct {
	Name AccountObjectIdentifier

	// SET operations.
	AllowWrites *bool
	Comment     *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string

	// AddLocations contains storage locations to ADD.
	AddLocations []ExternalVolumeStorageLocationOption

	// RemoveLocationNames contains storage location names to REMOVE.
	RemoveLocationNames []string
}

// Validate checks the options for validity.
func (o *AlterExternalVolumeOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("external volume name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterExternalVolumeOptions) HasChanges() bool {
	return o.AllowWrites != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0 ||
		len(o.AddLocations) > 0 ||
		len(o.RemoveLocationNames) > 0
}

// ExternalVolumeClient provides operations against Snowflake external volumes.
type ExternalVolumeClient struct {
	client SQLExecutor
}

// NewExternalVolumeClient creates a new client.
func NewExternalVolumeClient(c SQLExecutor) *ExternalVolumeClient {
	return &ExternalVolumeClient{client: c}
}

// buildStorageLocationSQL builds the SQL fragment for a single storage location.
func buildStorageLocationSQL(loc ExternalVolumeStorageLocationOption) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "(NAME = '%s'", sqlbuilder.EscapeString(loc.Name))
	fmt.Fprintf(&sb, " STORAGE_PROVIDER = '%s'", sqlbuilder.EscapeString(loc.StorageProvider))
	fmt.Fprintf(&sb, " STORAGE_BASE_URL = '%s'", sqlbuilder.EscapeString(loc.StorageBaseURL))

	if loc.StorageAWSRoleARN != nil {
		fmt.Fprintf(&sb, " STORAGE_AWS_ROLE_ARN = '%s'", sqlbuilder.EscapeString(*loc.StorageAWSRoleARN))
	}

	if loc.StorageAWSExternalID != nil {
		fmt.Fprintf(&sb, " STORAGE_AWS_EXTERNAL_ID = '%s'", sqlbuilder.EscapeString(*loc.StorageAWSExternalID))
	}

	if loc.EncryptionType != nil {
		sb.WriteString(" ENCRYPTION = (")
		fmt.Fprintf(&sb, " TYPE = '%s'", sqlbuilder.EscapeString(*loc.EncryptionType))

		if loc.EncryptionKMSKeyID != nil {
			fmt.Fprintf(&sb, " KMS_KEY_ID = '%s'", sqlbuilder.EscapeString(*loc.EncryptionKMSKeyID))
		}

		sb.WriteString(" )")
	}

	if loc.AzureTenantID != nil {
		fmt.Fprintf(&sb, " AZURE_TENANT_ID = '%s'", sqlbuilder.EscapeString(*loc.AzureTenantID))
	}

	sb.WriteString(")")

	return sb.String()
}

// buildCreateExternalVolumeSQL builds the CREATE EXTERNAL VOLUME SQL.
func buildCreateExternalVolumeSQL(opts CreateExternalVolumeOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "EXTERNAL VOLUME", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)

	b.WriteString(" STORAGE_LOCATIONS = (")

	for i, loc := range opts.StorageLocations {
		if i > 0 {
			b.WriteString(", ")
		}

		b.WriteString(buildStorageLocationSQL(loc))
	}

	b.WriteString(")")

	if opts.AllowWrites != nil {
		b.SetBool("ALLOW_WRITES", opts.AllowWrites)
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates an external volume in Snowflake.
func (c *ExternalVolumeClient) Create(ctx context.Context, opts CreateExternalVolumeOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create external volume options: %w", err))
	}

	sql, err := buildCreateExternalVolumeSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create external volume SQL: %w", err))
	}

	if _, err := c.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating external volume %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterExternalVolumeStatements builds ALTER EXTERNAL VOLUME statements.
func buildAlterExternalVolumeStatements(opts AlterExternalVolumeOptions) ([]string, error) {
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var stmts []string

	// REMOVE storage locations first (before ADD to avoid name conflicts).
	for _, locName := range opts.RemoveLocationNames {
		stmts = append(stmts, fmt.Sprintf("ALTER EXTERNAL VOLUME %s REMOVE STORAGE_LOCATION '%s'",
			fqn, sqlbuilder.EscapeString(locName)))
	}

	// ADD new storage locations.
	for _, loc := range opts.AddLocations {
		stmts = append(stmts, fmt.Sprintf("ALTER EXTERNAL VOLUME %s ADD STORAGE_LOCATION = %s",
			fqn, buildStorageLocationSQL(loc)))
	}

	// SET/UNSET operations via standard sqlbuilder.
	var sc sqlbuilder.SetClauses

	if opts.AllowWrites != nil {
		sc.Bool("ALLOW_WRITES", opts.AllowWrites)
	}

	sc.String("COMMENT", opts.Comment)

	setStmts, err := sqlbuilder.BuildAlterStatements("EXTERNAL VOLUME", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	stmts = append(stmts, setStmts...)

	return stmts, nil
}

// Alter alters an external volume in Snowflake.
func (c *ExternalVolumeClient) Alter(ctx context.Context, opts AlterExternalVolumeOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter external volume options: %w", err))
	}

	stmts, err := buildAlterExternalVolumeStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter external volume statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := c.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering external volume %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops an external volume from Snowflake.
func (c *ExternalVolumeClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("external volume name is required"))
	}

	stmt := sqlbuilder.DropIfExists("EXTERNAL VOLUME", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := c.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping external volume %s: %w", name, err)
	}

	return nil
}

// ShowByID queries SHOW EXTERNAL VOLUMES for a specific external volume.
func (c *ExternalVolumeClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.ExternalVolumeShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("external volume name is required"))
	}

	rows, err := c.client.Query(ctx, sqlbuilder.ShowLike("EXTERNAL VOLUMES", name.Name()))
	if err != nil {
		return nil, fmt.Errorf("showing external volume %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanExternalVolumeShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE EXTERNAL VOLUME and returns storage location names.
func (c *ExternalVolumeClient) Describe(ctx context.Context, name AccountObjectIdentifier) ([]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("external volume name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE EXTERNAL VOLUME %s", sqlbuilder.QuoteIdentifier(name.Name()))

	rows, err := c.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing external volume %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanExternalVolumeDescribe(rows)
}

// Observe combines ShowByID and Describe into an ExternalVolumeObservation.
func (c *ExternalVolumeClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*ExternalVolumeObservation, error) {
	show, err := c.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &ExternalVolumeObservation{Exists: false}, nil
		}

		return nil, err
	}

	locNames, err := c.Describe(ctx, name)
	if err != nil {
		// DESCRIBE may fail—still return the show output.
		return &ExternalVolumeObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &ExternalVolumeObservation{
		Exists:               true,
		ShowOutput:           show,
		StorageLocationNames: locNames,
	}, nil
}

// scanExternalVolumeShowOutput scans SHOW EXTERNAL VOLUMES results.
func scanExternalVolumeShowOutput(rows *sql.Rows, name string) (*v1alpha1.ExternalVolumeShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.ExternalVolumeShowOutput, error) {
		return &v1alpha1.ExternalVolumeShowOutput{
			Name:        m["name"],
			AllowWrites: strings.EqualFold(m["allow_writes"], "true"),
			Comment:     m["comment"],
		}, nil
	})
}

// scanExternalVolumeDescribe extracts storage location names from DESCRIBE output.
// DESCRIBE EXTERNAL VOLUME returns rows with parent_property / property / property_value.
// We extract unique storage location names from the "STORAGE_LOCATION_" entries.
func scanExternalVolumeDescribe(rows *sql.Rows) ([]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading describe columns: %w", err)
	}

	// Find relevant column indices.
	parentIdx, propIdx, valIdx := -1, -1, -1

	for i, col := range cols {
		lc := strings.ToLower(col)

		switch lc {
		case "parent_property":
			parentIdx = i
		case "property":
			propIdx = i
		case "property_value":
			valIdx = i
		}
	}

	// Fall back to key/value pattern if property columns not found.
	if propIdx == -1 {
		for i, col := range cols {
			lc := strings.ToLower(col)

			switch lc {
			case "property", "name":
				if propIdx == -1 {
					propIdx = i
				}
			case "property_value", "value":
				if valIdx == -1 {
					valIdx = i
				}
			}
		}
	}

	if propIdx == -1 || valIdx == -1 {
		return nil, fmt.Errorf("cannot determine property columns from: %v", cols)
	}

	// Collect unique storage location names.
	nameSet := make(map[string]struct{})

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning describe row: %w", err)
		}

		// Look for STORAGE_LOCATION_NAME property to extract location names.
		propName := ""
		if values[propIdx].Valid {
			propName = strings.ToUpper(values[propIdx].String)
		}

		propValue := ""
		if values[valIdx].Valid {
			propValue = values[valIdx].String
		}

		parentProp := ""
		if parentIdx >= 0 && values[parentIdx].Valid {
			parentProp = strings.ToUpper(values[parentIdx].String)
		}

		// Storage location names appear as STORAGE_LOCATION_NAME property
		// or as NAME under a STORAGE_LOCATIONS parent property.
		if propName == "STORAGE_LOCATION_NAME" || (parentProp == "STORAGE_LOCATIONS" && propName == "NAME") {
			if propValue != "" {
				nameSet[propValue] = struct{}{}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating describe rows: %w", err)
	}

	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}

	return names, nil
}
