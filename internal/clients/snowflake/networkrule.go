package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// NetworkRuleObservation holds the result of observing a Snowflake network rule.
type NetworkRuleObservation struct {
	// Exists indicates whether the rule was found.
	Exists bool

	// ShowOutput contains the SHOW NETWORK RULES row.
	ShowOutput *v1alpha1.NetworkRuleShowOutput

	// DescribeOutput contains the DESCRIBE NETWORK RULE output (key-value pairs).
	DescribeOutput map[string]string
}

// CreateNetworkRuleOptions holds the parameters for creating a network rule.
type CreateNetworkRuleOptions struct {
	Name      SchemaObjectIdentifier
	Type      string // IPV4, AWSVPCEID, AZURELINKID, GCPPSCID, HOST_PORT, PRIVATE_HOST_PORT
	Mode      string // INGRESS, INTERNAL_STAGE, EGRESS
	ValueList []string
	Comment   *string

	// UseCreateOrAlter emits CREATE OR ALTER NETWORK RULE instead of
	// CREATE NETWORK RULE IF NOT EXISTS.
	UseCreateOrAlter bool
}

// Validate checks the CreateNetworkRuleOptions for validity.
func (o *CreateNetworkRuleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("network rule name is required")
	}

	if o.Type == "" {
		return fmt.Errorf("network rule type is required")
	}

	if o.Mode == "" {
		return fmt.Errorf("network rule mode is required")
	}

	if len(o.ValueList) == 0 {
		return fmt.Errorf("network rule value list is required")
	}

	return nil
}

// AlterNetworkRuleOptions holds the parameters for altering a network rule.
type AlterNetworkRuleOptions struct {
	Name      SchemaObjectIdentifier
	ValueList *[]string
	Comment   *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterNetworkRuleOptions for validity.
func (o *AlterNetworkRuleOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("network rule name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterNetworkRuleOptions) HasChanges() bool {
	return o.ValueList != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// NetworkRuleClient provides operations against Snowflake network rules.
type NetworkRuleClient struct {
	client SQLExecutor
}

// NewNetworkRuleClient creates a new NetworkRuleClient backed by the given SQLExecutor.
func NewNetworkRuleClient(c SQLExecutor) *NetworkRuleClient {
	return &NetworkRuleClient{client: c}
}

// buildValueListClause builds a VALUE_LIST clause like VALUE_LIST = ('1.2.3.4', '5.6.7.8').
func buildValueListClause(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("'%s'", sqlbuilder.EscapeString(v))
	}

	return fmt.Sprintf("VALUE_LIST = (%s)", strings.Join(quoted, ", "))
}

// buildCreateNetworkRuleSQL builds the CREATE NETWORK RULE SQL statement.
func buildCreateNetworkRuleSQL(opts CreateNetworkRuleOptions) (string, error) {
	var b sqlbuilder.Builder

	sqlbuilder.BuildCreatePreamble(&b, "NETWORK RULE", opts.Name.FullyQualifiedName(), opts.UseCreateOrAlter, false)
	fmt.Fprintf(&b.Builder, " TYPE = %s", opts.Type)
	fmt.Fprintf(&b.Builder, " MODE = %s", opts.Mode)

	if len(opts.ValueList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildValueListClause(opts.ValueList))
	}

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a network rule in Snowflake.
func (nr *NetworkRuleClient) Create(ctx context.Context, opts CreateNetworkRuleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create network rule options: %w", err))
	}

	sql, err := buildCreateNetworkRuleSQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create network rule SQL: %w", err))
	}

	if _, err := nr.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating network rule %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterNetworkRuleStatements builds ALTER NETWORK RULE SQL statements.
func buildAlterNetworkRuleStatements(opts AlterNetworkRuleOptions) ([]string, error) {
	var statements []string
	fqn := opts.Name.FullyQualifiedName()

	// VALUE_LIST is a separate ALTER SET clause.
	if opts.ValueList != nil {
		statements = append(statements, fmt.Sprintf("ALTER NETWORK RULE %s SET %s", fqn, buildValueListClause(*opts.ValueList)))
	}

	// Comment and other SET/UNSET fields — only build ALTER when there are changes.
	if opts.Comment != nil || len(opts.UnsetFields) > 0 {
		var sc sqlbuilder.SetClauses
		sc.String("COMMENT", opts.Comment)

		alterStmts, err := sqlbuilder.BuildAlterStatements("NETWORK RULE", fqn, &sc, opts.UnsetFields)
		if err != nil {
			return nil, err
		}

		statements = append(statements, alterStmts...)
	}

	return statements, nil
}

// Alter alters a network rule in Snowflake.
func (nr *NetworkRuleClient) Alter(ctx context.Context, opts AlterNetworkRuleOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter network rule options: %w", err))
	}

	stmts, err := buildAlterNetworkRuleStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter network rule statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := nr.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering network rule %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a network rule from Snowflake.
func (nr *NetworkRuleClient) Drop(ctx context.Context, name SchemaObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("network rule name is required"))
	}

	stmt := sqlbuilder.DropIfExists("NETWORK RULE", name.FullyQualifiedName())

	if _, err := nr.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping network rule %s: %w", name, err)
	}

	return nil
}

// buildShowNetworkRuleByIDSQL builds a SHOW NETWORK RULES LIKE ... IN SCHEMA SQL statement.
func buildShowNetworkRuleByIDSQL(name SchemaObjectIdentifier) string {
	scope := fmt.Sprintf("SCHEMA %s.%s",
		sqlbuilder.QuoteIdentifier(name.DatabaseName()),
		sqlbuilder.QuoteIdentifier(name.SchemaName()),
	)

	return sqlbuilder.ShowLikeIn("NETWORK RULES", name.Name(), scope)
}

// ShowByID queries SHOW NETWORK RULES for a specific rule.
func (nr *NetworkRuleClient) ShowByID(ctx context.Context, name SchemaObjectIdentifier) (*v1alpha1.NetworkRuleShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("network rule name is required"))
	}

	rows, err := nr.client.Query(ctx, buildShowNetworkRuleByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing network rule %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanNetworkRuleShowOutput(rows, name.Name())
}

// Describe runs DESCRIBE NETWORK RULE and returns key-value pairs.
func (nr *NetworkRuleClient) Describe(ctx context.Context, name SchemaObjectIdentifier) (map[string]string, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("network rule name is required"))
	}

	stmt := fmt.Sprintf("DESCRIBE NETWORK RULE %s", name.FullyQualifiedName())

	rows, err := nr.client.Query(ctx, stmt)
	if err != nil {
		return nil, fmt.Errorf("describing network rule %s: %w", name, err)
	}
	defer closeRows(rows)

	return scanDescribeKeyValue(rows)
}

// Observe combines ShowByID and Describe into a NetworkRuleObservation.
func (nr *NetworkRuleClient) Observe(ctx context.Context, name SchemaObjectIdentifier) (*NetworkRuleObservation, error) {
	show, err := nr.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &NetworkRuleObservation{Exists: false}, nil
		}

		return nil, err
	}

	desc, err := nr.Describe(ctx, name)
	if err != nil {
		// If DESCRIBE fails but SHOW succeeded, return partial info.
		return &NetworkRuleObservation{
			Exists:     true,
			ShowOutput: show,
		}, nil
	}

	return &NetworkRuleObservation{
		Exists:         true,
		ShowOutput:     show,
		DescribeOutput: desc,
	}, nil
}

// scanNetworkRuleShowOutput scans SHOW NETWORK RULES results for a matching row.
func scanNetworkRuleShowOutput(rows *sql.Rows, name string) (*v1alpha1.NetworkRuleShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.NetworkRuleShowOutput, error) {
		return &v1alpha1.NetworkRuleShowOutput{
			CreatedOn:    m["created_on"],
			Name:         m["name"],
			DatabaseName: m["database_name"],
			SchemaName:   m["schema_name"],
			Owner:        m["owner"],
			Type:         m["type"],
			Mode:         m["mode"],
			Comment:      m["comment"],
		}, nil
	})
}
