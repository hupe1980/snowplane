package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// NetworkPolicyObservation holds the result of observing a Snowflake network policy.
type NetworkPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW NETWORK POLICIES row.
	ShowOutput *NetworkPolicyShowOutput
}

// NetworkPolicyShowOutput contains the fields from SHOW NETWORK POLICIES.
type NetworkPolicyShowOutput struct {
	CreatedOn              string
	Name                   string
	Comment                string
	EntriesInAllowedIPList string
	EntriesInBlockedIPList string
}

// CreateNetworkPolicyOptions holds the parameters for creating a network policy.
type CreateNetworkPolicyOptions struct {
	Name                   AccountObjectIdentifier
	AllowedIPList          []string
	BlockedIPList          []string
	AllowedNetworkRuleList []string
	BlockedNetworkRuleList []string
	Comment                *string
}

// Validate checks the CreateNetworkPolicyOptions for validity.
func (o *CreateNetworkPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("network policy name is required")
	}

	return nil
}

// AlterNetworkPolicyOptions holds the parameters for altering a network policy.
type AlterNetworkPolicyOptions struct {
	Name                   AccountObjectIdentifier
	AllowedIPList          *[]string
	BlockedIPList          *[]string
	AllowedNetworkRuleList *[]string
	BlockedNetworkRuleList *[]string
	Comment                *string

	// UnsetFields lists Snowflake parameter names to UNSET.
	UnsetFields []string
}

// Validate checks the AlterNetworkPolicyOptions for validity.
func (o *AlterNetworkPolicyOptions) Validate() error {
	if !ValidObjectIdentifier(o.Name) {
		return fmt.Errorf("network policy name is required")
	}

	return nil
}

// HasChanges reports whether any fields are set for alteration.
func (o *AlterNetworkPolicyOptions) HasChanges() bool {
	return o.AllowedIPList != nil ||
		o.BlockedIPList != nil ||
		o.AllowedNetworkRuleList != nil ||
		o.BlockedNetworkRuleList != nil ||
		o.Comment != nil ||
		len(o.UnsetFields) > 0
}

// NetworkPolicyClient provides operations against Snowflake network policies.
type NetworkPolicyClient struct {
	client SQLExecutor
}

// NewNetworkPolicyClient creates a new NetworkPolicyClient backed by the given SQLExecutor.
func NewNetworkPolicyClient(c SQLExecutor) *NetworkPolicyClient {
	return &NetworkPolicyClient{client: c}
}

// buildIPListClause builds an IP list clause like ALLOWED_IP_LIST = ('1.2.3.4', '5.6.7.8').
func buildIPListClause(keyword string, ips []string) string {
	quoted := make([]string, len(ips))
	for i, ip := range ips {
		quoted[i] = fmt.Sprintf("'%s'", ip)
	}

	return fmt.Sprintf("%s = (%s)", keyword, strings.Join(quoted, ", "))
}

// buildNetworkRuleListClause builds a network rule list clause.
func buildNetworkRuleListClause(keyword string, rules []string) string {
	quoted := make([]string, len(rules))
	for i, r := range rules {
		quoted[i] = fmt.Sprintf("'%s'", r)
	}

	return fmt.Sprintf("%s = (%s)", keyword, strings.Join(quoted, ", "))
}

// buildCreateNetworkPolicySQL builds the CREATE NETWORK POLICY SQL statement.
func buildCreateNetworkPolicySQL(opts CreateNetworkPolicyOptions) string {
	var b sqlbuilder.Builder
	b.WriteString("CREATE NETWORK POLICY IF NOT EXISTS ")
	b.WriteString(sqlbuilder.QuoteIdentifier(opts.Name.Name()))

	if len(opts.AllowedNetworkRuleList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildNetworkRuleListClause("ALLOWED_NETWORK_RULE_LIST", opts.AllowedNetworkRuleList))
	}

	if len(opts.BlockedNetworkRuleList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildNetworkRuleListClause("BLOCKED_NETWORK_RULE_LIST", opts.BlockedNetworkRuleList))
	}

	if len(opts.AllowedIPList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildIPListClause("ALLOWED_IP_LIST", opts.AllowedIPList))
	}

	if len(opts.BlockedIPList) > 0 {
		b.WriteString(" ")
		b.WriteString(buildIPListClause("BLOCKED_IP_LIST", opts.BlockedIPList))
	}

	b.SetString("COMMENT", opts.Comment)

	return b.String()
}

// Create creates a network policy in Snowflake.
func (np *NetworkPolicyClient) Create(ctx context.Context, opts CreateNetworkPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create network policy options: %w", err))
	}

	if _, err := np.client.Exec(ctx, buildCreateNetworkPolicySQL(opts)); err != nil {
		return fmt.Errorf("creating network policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterNetworkPolicyStatements builds the ALTER NETWORK POLICY SQL statements.
func buildAlterNetworkPolicyStatements(opts AlterNetworkPolicyOptions) ([]string, error) {
	var statements []string
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	if opts.AllowedIPList != nil {
		statements = append(statements, fmt.Sprintf("ALTER NETWORK POLICY %s SET %s", fqn, buildIPListClause("ALLOWED_IP_LIST", *opts.AllowedIPList)))
	}

	if opts.BlockedIPList != nil {
		statements = append(statements, fmt.Sprintf("ALTER NETWORK POLICY %s SET %s", fqn, buildIPListClause("BLOCKED_IP_LIST", *opts.BlockedIPList)))
	}

	if opts.AllowedNetworkRuleList != nil {
		statements = append(statements, fmt.Sprintf("ALTER NETWORK POLICY %s SET %s", fqn, buildNetworkRuleListClause("ALLOWED_NETWORK_RULE_LIST", *opts.AllowedNetworkRuleList)))
	}

	if opts.BlockedNetworkRuleList != nil {
		statements = append(statements, fmt.Sprintf("ALTER NETWORK POLICY %s SET %s", fqn, buildNetworkRuleListClause("BLOCKED_NETWORK_RULE_LIST", *opts.BlockedNetworkRuleList)))
	}

	// Handle comment and other SET/UNSET fields.
	var sc sqlbuilder.SetClauses
	sc.String("COMMENT", opts.Comment)

	alterStmts, err := sqlbuilder.BuildAlterStatements("NETWORK POLICY", fqn, &sc, opts.UnsetFields)
	if err != nil {
		return nil, err
	}

	statements = append(statements, alterStmts...)

	return statements, nil
}

// Alter alters a network policy in Snowflake.
func (np *NetworkPolicyClient) Alter(ctx context.Context, opts AlterNetworkPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid alter network policy options: %w", err))
	}

	stmts, err := buildAlterNetworkPolicyStatements(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building alter network policy statements: %w", err))
	}

	for _, stmt := range stmts {
		if _, err := np.client.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("altering network policy %s: %w", opts.Name, err)
		}
	}

	return nil
}

// Drop drops a network policy from Snowflake.
func (np *NetworkPolicyClient) Drop(ctx context.Context, name AccountObjectIdentifier) error {
	if !ValidObjectIdentifier(name) {
		return NewTerminalError(fmt.Errorf("network policy name is required"))
	}

	stmt := sqlbuilder.DropIfExists("NETWORK POLICY", sqlbuilder.QuoteIdentifier(name.Name()))

	if _, err := np.client.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("dropping network policy %s: %w", name, err)
	}

	return nil
}

// buildShowNetworkPolicyByIDSQL builds a SHOW NETWORK POLICIES LIKE SQL statement.
func buildShowNetworkPolicyByIDSQL(name AccountObjectIdentifier) string {
	return sqlbuilder.ShowLike("NETWORK POLICIES", name.Name())
}

// ShowByID queries SHOW NETWORK POLICIES for a specific policy.
func (np *NetworkPolicyClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*NetworkPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("network policy name is required"))
	}

	rows, err := np.client.Query(ctx, buildShowNetworkPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing network policy %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()

	return scanNetworkPolicyShowOutput(rows, name.Name())
}

// Observe combines ShowByID into a NetworkPolicyObservation.
func (np *NetworkPolicyClient) Observe(ctx context.Context, name AccountObjectIdentifier) (*NetworkPolicyObservation, error) {
	show, err := np.ShowByID(ctx, name)
	if err != nil {
		if IsObjectNotFound(err) {
			return &NetworkPolicyObservation{Exists: false}, nil
		}

		return nil, err
	}

	return &NetworkPolicyObservation{
		Exists:     true,
		ShowOutput: show,
	}, nil
}

// scanNetworkPolicyShowOutput scans SHOW NETWORK POLICIES results for a matching row.
func scanNetworkPolicyShowOutput(rows *sql.Rows, name string) (*NetworkPolicyShowOutput, error) {
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

		return &NetworkPolicyShowOutput{
			CreatedOn:              colMap["created_on"],
			Name:                   colMap["name"],
			Comment:                colMap["comment"],
			EntriesInAllowedIPList: colMap["entries_in_allowed_ip_list"],
			EntriesInBlockedIPList: colMap["entries_in_blocked_ip_list"],
		}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}
