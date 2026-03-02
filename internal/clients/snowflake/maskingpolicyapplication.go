package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// MaskingPolicyApplicationObservation holds the result of observing a masking policy application.
type MaskingPolicyApplicationObservation struct {
	// Exists indicates whether a masking policy is currently applied to the column.
	Exists bool

	// PolicyName is the fully qualified name of the currently applied policy.
	PolicyName string
}

// MaskingPolicyApplicationIdentifier uniquely identifies a masking policy application.
type MaskingPolicyApplicationIdentifier struct {
	// PolicyName is the fully qualified masking policy name.
	PolicyName string

	// TableName is the fully qualified table name.
	TableName string

	// ColumnName is the column name.
	ColumnName string
}

// FullyQualifiedName returns a human-readable representation.
func (id MaskingPolicyApplicationIdentifier) FullyQualifiedName() string {
	return fmt.Sprintf("MASKING_POLICY %s ON %s.%s", id.PolicyName, id.TableName, id.ColumnName)
}

// String returns the fully qualified name.
func (id MaskingPolicyApplicationIdentifier) String() string { return id.FullyQualifiedName() }

// SetMaskingPolicyOptions holds the parameters for applying a masking policy.
type SetMaskingPolicyOptions struct {
	PolicyName   string   // fully qualified masking policy name
	TableName    string   // fully qualified table name
	ColumnName   string   // column name
	UsingColumns []string // optional conditional masking columns
}

// Validate checks the SetMaskingPolicyOptions.
func (o *SetMaskingPolicyOptions) Validate() error {
	if o.PolicyName == "" {
		return fmt.Errorf("policy name is required")
	}

	if o.TableName == "" {
		return fmt.Errorf("table name is required")
	}

	if o.ColumnName == "" {
		return fmt.Errorf("column name is required")
	}

	return nil
}

// UnsetMaskingPolicyOptions holds the parameters for removing a masking policy.
type UnsetMaskingPolicyOptions struct {
	TableName  string
	ColumnName string
}

// Validate checks the UnsetMaskingPolicyOptions.
func (o *UnsetMaskingPolicyOptions) Validate() error {
	if o.TableName == "" {
		return fmt.Errorf("table name is required")
	}

	if o.ColumnName == "" {
		return fmt.Errorf("column name is required")
	}

	return nil
}

// MaskingPolicyApplicationClient provides operations against Snowflake masking policy applications.
type MaskingPolicyApplicationClient struct {
	client SQLExecutor
}

// NewMaskingPolicyApplicationClient creates a new MaskingPolicyApplicationClient.
func NewMaskingPolicyApplicationClient(c SQLExecutor) *MaskingPolicyApplicationClient {
	return &MaskingPolicyApplicationClient{client: c}
}

// buildSetMaskingPolicySQL builds the ALTER TABLE ... ALTER COLUMN ... SET MASKING POLICY SQL.
func buildSetMaskingPolicySQL(opts SetMaskingPolicyOptions) string {
	query := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET MASKING POLICY %s",
		opts.TableName,
		opts.ColumnName,
		opts.PolicyName,
	)

	if len(opts.UsingColumns) > 0 {
		query += fmt.Sprintf(" USING (%s)", strings.Join(opts.UsingColumns, ", "))
	}

	return query
}

// buildUnsetMaskingPolicySQL builds the ALTER TABLE ... ALTER COLUMN ... UNSET MASKING POLICY SQL.
func buildUnsetMaskingPolicySQL(opts UnsetMaskingPolicyOptions) string {
	return fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s UNSET MASKING POLICY",
		opts.TableName,
		opts.ColumnName,
	)
}

// buildMaskingPolicyReferencesSQL builds the SELECT from POLICY_REFERENCES table function.
func buildMaskingPolicyReferencesSQL(tableName string) string {
	return fmt.Sprintf(
		"SELECT POLICY_DB, POLICY_SCHEMA, POLICY_NAME, POLICY_KIND, REF_COLUMN_NAME FROM TABLE(SNOWFLAKE.INFORMATION_SCHEMA.POLICY_REFERENCES(REF_ENTITY_NAME => '%s', REF_ENTITY_DOMAIN => 'TABLE'))",
		strings.ReplaceAll(tableName, "'", "''"),
	)
}

// Observe queries Snowflake to check if a masking policy is applied to the column.
func (c *MaskingPolicyApplicationClient) Observe(ctx context.Context, id MaskingPolicyApplicationIdentifier) (*MaskingPolicyApplicationObservation, error) {
	rows, err := c.client.Query(ctx, buildMaskingPolicyReferencesSQL(id.TableName))
	if err != nil {
		if IsObjectNotExistOrNotAuthorized(err) || IsSQLCompilationError(err) {
			return &MaskingPolicyApplicationObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("observing masking policy application %s: %w", id, err)
	}
	defer closeRows(rows)

	return scanMaskingPolicyReference(rows, id.PolicyName, id.ColumnName)
}

// scanMaskingPolicyReference parses POLICY_REFERENCES results for MASKING_POLICY on a specific column.
func scanMaskingPolicyReference(rows *sql.Rows, expectedPolicy, expectedColumn string) (*MaskingPolicyApplicationObservation, error) {
	for rows.Next() {
		var policyDB, policySchema, policyName, policyKind, refColumnName sql.NullString

		if err := rows.Scan(&policyDB, &policySchema, &policyName, &policyKind, &refColumnName); err != nil {
			return nil, fmt.Errorf("scanning policy reference row: %w", err)
		}

		if !policyKind.Valid || !strings.EqualFold(policyKind.String, "MASKING_POLICY") {
			continue
		}

		// Match on the specific column.
		if !refColumnName.Valid || !strings.EqualFold(refColumnName.String, expectedColumn) {
			continue
		}

		if !policyName.Valid {
			continue
		}

		fqn := fmt.Sprintf(`"%s"."%s"."%s"`, policyDB.String, policySchema.String, policyName.String)

		if strings.EqualFold(fqn, expectedPolicy) {
			return &MaskingPolicyApplicationObservation{
				Exists:     true,
				PolicyName: fqn,
			}, nil
		}

		// A different masking policy is applied to this column.
		return &MaskingPolicyApplicationObservation{Exists: false, PolicyName: fqn}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating policy reference rows: %w", err)
	}

	return &MaskingPolicyApplicationObservation{Exists: false}, nil
}

// SetMaskingPolicy applies a masking policy to a table column.
func (c *MaskingPolicyApplicationClient) SetMaskingPolicy(ctx context.Context, opts SetMaskingPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid set masking policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildSetMaskingPolicySQL(opts)); err != nil {
		return fmt.Errorf("setting masking policy %s on %s.%s: %w", opts.PolicyName, opts.TableName, opts.ColumnName, err)
	}

	return nil
}

// UnsetMaskingPolicy removes a masking policy from a table column.
func (c *MaskingPolicyApplicationClient) UnsetMaskingPolicy(ctx context.Context, opts UnsetMaskingPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid unset masking policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildUnsetMaskingPolicySQL(opts)); err != nil {
		return fmt.Errorf("unsetting masking policy from %s.%s: %w", opts.TableName, opts.ColumnName, err)
	}

	return nil
}
