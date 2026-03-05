package snowflake

import (
	"context"
	"database/sql"
	"fmt"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// NetworkPolicyObservation holds the result of observing a Snowflake network policy.
type NetworkPolicyObservation struct {
	// Exists indicates whether the policy was found.
	Exists bool

	// ShowOutput contains the SHOW NETWORK POLICIES row.
	ShowOutput *v1alpha1.NetworkPolicyShowOutput
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

// buildCreateNetworkPolicySQL builds the CREATE NETWORK POLICY SQL statement.
func buildCreateNetworkPolicySQL(opts CreateNetworkPolicyOptions) (string, error) {
	var b sqlbuilder.Builder
	sqlbuilder.BuildCreatePreamble(&b, "NETWORK POLICY", sqlbuilder.QuoteIdentifier(opts.Name.Name()), false, false)

	b.SetEscapedList("ALLOWED_NETWORK_RULE_LIST", opts.AllowedNetworkRuleList)
	b.SetEscapedList("BLOCKED_NETWORK_RULE_LIST", opts.BlockedNetworkRuleList)
	b.SetEscapedList("ALLOWED_IP_LIST", opts.AllowedIPList)
	b.SetEscapedList("BLOCKED_IP_LIST", opts.BlockedIPList)

	b.SetString("COMMENT", opts.Comment)

	if err := b.Err(); err != nil {
		return "", err
	}

	return b.String(), nil
}

// Create creates a network policy in Snowflake.
func (np *NetworkPolicyClient) Create(ctx context.Context, opts CreateNetworkPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid create network policy options: %w", err))
	}

	sql, err := buildCreateNetworkPolicySQL(opts)
	if err != nil {
		return NewTerminalError(fmt.Errorf("building create network policy SQL: %w", err))
	}

	if _, err := np.client.Exec(ctx, sql); err != nil {
		return fmt.Errorf("creating network policy %s: %w", opts.Name, err)
	}

	return nil
}

// buildAlterNetworkPolicyStatements builds the ALTER NETWORK POLICY SQL statements.
func buildAlterNetworkPolicyStatements(opts AlterNetworkPolicyOptions) ([]string, error) {
	fqn := sqlbuilder.QuoteIdentifier(opts.Name.Name())

	var sc sqlbuilder.SetClauses

	if opts.AllowedIPList != nil {
		sc.EscapedList("ALLOWED_IP_LIST", *opts.AllowedIPList)
	}

	if opts.BlockedIPList != nil {
		sc.EscapedList("BLOCKED_IP_LIST", *opts.BlockedIPList)
	}

	if opts.AllowedNetworkRuleList != nil {
		sc.EscapedList("ALLOWED_NETWORK_RULE_LIST", *opts.AllowedNetworkRuleList)
	}

	if opts.BlockedNetworkRuleList != nil {
		sc.EscapedList("BLOCKED_NETWORK_RULE_LIST", *opts.BlockedNetworkRuleList)
	}

	sc.String("COMMENT", opts.Comment)

	return sqlbuilder.BuildAlterStatements("NETWORK POLICY", fqn, &sc, opts.UnsetFields)
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
func (np *NetworkPolicyClient) ShowByID(ctx context.Context, name AccountObjectIdentifier) (*v1alpha1.NetworkPolicyShowOutput, error) {
	if !ValidObjectIdentifier(name) {
		return nil, NewTerminalError(fmt.Errorf("network policy name is required"))
	}

	rows, err := np.client.Query(ctx, buildShowNetworkPolicyByIDSQL(name))
	if err != nil {
		return nil, fmt.Errorf("showing network policy %s: %w", name, err)
	}
	defer closeRows(rows)

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
func scanNetworkPolicyShowOutput(rows *sql.Rows, name string) (*v1alpha1.NetworkPolicyShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.NetworkPolicyShowOutput, error) {
		return &v1alpha1.NetworkPolicyShowOutput{
			CreatedOn:              m["created_on"],
			Name:                   m["name"],
			Comment:                m["comment"],
			EntriesInAllowedIPList: m["entries_in_allowed_ip_list"],
			EntriesInBlockedIPList: m["entries_in_blocked_ip_list"],
		}, nil
	})
}
