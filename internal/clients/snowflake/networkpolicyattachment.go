package snowflake

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	// TargetTypeAccount is the target type for account-level policy attachments.
	TargetTypeAccount = "ACCOUNT"
)

// NetworkPolicyAttachmentObservation holds the result of observing a network policy attachment.
type NetworkPolicyAttachmentObservation struct {
	// Exists indicates whether a network policy is currently attached to the target.
	Exists bool

	// PolicyName is the currently attached policy name, as returned by SHOW PARAMETERS.
	PolicyName string
}

// NetworkPolicyAttachmentIdentifier uniquely identifies a network policy attachment.
type NetworkPolicyAttachmentIdentifier struct {
	// PolicyName is the network policy name (simple unquoted name).
	PolicyName string

	// TargetType is TargetTypeAccount or "USER".
	TargetType string

	// TargetName is the user name (empty for ACCOUNT).
	TargetName string
}

// FullyQualifiedName returns a human-readable representation.
func (id NetworkPolicyAttachmentIdentifier) FullyQualifiedName() string {
	if id.TargetType == TargetTypeAccount {
		return fmt.Sprintf("NETWORK_POLICY %s ON ACCOUNT", id.PolicyName)
	}

	return fmt.Sprintf("NETWORK_POLICY %s ON USER %s", id.PolicyName, id.TargetName)
}

// String returns the fully qualified name.
func (id NetworkPolicyAttachmentIdentifier) String() string { return id.FullyQualifiedName() }

// SetNetworkPolicyOptions holds the parameters for attaching a network policy.
type SetNetworkPolicyOptions struct {
	PolicyName string // network policy name
	TargetType string // TargetTypeAccount or "USER"
	TargetName string // user name (for USER target)
}

// Validate checks the SetNetworkPolicyOptions.
func (o *SetNetworkPolicyOptions) Validate() error {
	if o.PolicyName == "" {
		return fmt.Errorf("policy name is required")
	}

	if o.TargetType == "" {
		return fmt.Errorf("target type is required")
	}

	if o.TargetType == "USER" && o.TargetName == "" {
		return fmt.Errorf("target name is required for USER target type")
	}

	return nil
}

// UnsetNetworkPolicyOptions holds the parameters for detaching a network policy.
type UnsetNetworkPolicyOptions struct {
	TargetType string
	TargetName string
}

// Validate checks the UnsetNetworkPolicyOptions.
func (o *UnsetNetworkPolicyOptions) Validate() error {
	if o.TargetType == "" {
		return fmt.Errorf("target type is required")
	}

	if o.TargetType == "USER" && o.TargetName == "" {
		return fmt.Errorf("target name is required for USER target type")
	}

	return nil
}

// NetworkPolicyAttachmentClient provides operations against Snowflake network policy attachments.
type NetworkPolicyAttachmentClient struct {
	client SQLExecutor
}

// NewNetworkPolicyAttachmentClient creates a new NetworkPolicyAttachmentClient.
func NewNetworkPolicyAttachmentClient(c SQLExecutor) *NetworkPolicyAttachmentClient {
	return &NetworkPolicyAttachmentClient{client: c}
}

// buildSetNetworkPolicySQL builds the ALTER ... SET NETWORK_POLICY SQL statement.
func buildSetNetworkPolicySQL(opts SetNetworkPolicyOptions) string {
	if opts.TargetType == TargetTypeAccount {
		return fmt.Sprintf("ALTER ACCOUNT SET NETWORK_POLICY = %s", opts.PolicyName)
	}

	return fmt.Sprintf("ALTER USER %s SET NETWORK_POLICY = %s", opts.TargetName, opts.PolicyName)
}

// buildUnsetNetworkPolicySQL builds the ALTER ... UNSET NETWORK_POLICY SQL statement.
func buildUnsetNetworkPolicySQL(opts UnsetNetworkPolicyOptions) string {
	if opts.TargetType == TargetTypeAccount {
		return "ALTER ACCOUNT UNSET NETWORK_POLICY"
	}

	return fmt.Sprintf("ALTER USER %s UNSET NETWORK_POLICY", opts.TargetName)
}

// buildShowNetworkPolicyParameterSQL builds the SHOW PARAMETERS query.
func buildShowNetworkPolicyParameterSQL(id NetworkPolicyAttachmentIdentifier) string {
	if id.TargetType == TargetTypeAccount {
		return "SHOW PARAMETERS LIKE 'NETWORK_POLICY' IN ACCOUNT"
	}

	return fmt.Sprintf("SHOW PARAMETERS LIKE 'NETWORK_POLICY' FOR USER %s", id.TargetName)
}

// Observe queries Snowflake to check if a network policy is attached, and its current value.
func (c *NetworkPolicyAttachmentClient) Observe(ctx context.Context, id NetworkPolicyAttachmentIdentifier) (*NetworkPolicyAttachmentObservation, error) {
	rows, err := c.client.Query(ctx, buildShowNetworkPolicyParameterSQL(id))
	if err != nil {
		if IsObjectNotExistOrNotAuthorized(err) {
			return &NetworkPolicyAttachmentObservation{Exists: false}, nil
		}

		return nil, fmt.Errorf("observing network policy attachment %s: %w", id, err)
	}
	defer closeRows(rows)

	return scanNetworkPolicyParameter(rows, id.PolicyName)
}

// scanNetworkPolicyParameter parses SHOW PARAMETERS results for the NETWORK_POLICY key.
func scanNetworkPolicyParameter(rows *sql.Rows, expectedPolicy string) (*NetworkPolicyAttachmentObservation, error) {
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
			return nil, fmt.Errorf("scanning parameter row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		key := strings.ToUpper(colMap["key"])
		if key != "NETWORK_POLICY" {
			continue
		}

		val := colMap["value"]

		// A non-empty value means a policy is set.
		if val == "" {
			return &NetworkPolicyAttachmentObservation{Exists: false}, nil
		}

		// The policy exists and matches the expected one.
		if strings.EqualFold(val, expectedPolicy) {
			return &NetworkPolicyAttachmentObservation{
				Exists:     true,
				PolicyName: val,
			}, nil
		}

		// A different policy is attached — our specific attachment does not exist.
		return &NetworkPolicyAttachmentObservation{Exists: false, PolicyName: val}, nil
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parameter rows: %w", err)
	}

	return &NetworkPolicyAttachmentObservation{Exists: false}, nil
}

// SetNetworkPolicy attaches a network policy to the target.
func (c *NetworkPolicyAttachmentClient) SetNetworkPolicy(ctx context.Context, opts SetNetworkPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid set network policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildSetNetworkPolicySQL(opts)); err != nil {
		return fmt.Errorf("setting network policy %s on %s %s: %w", opts.PolicyName, opts.TargetType, opts.TargetName, err)
	}

	return nil
}

// UnsetNetworkPolicy detaches a network policy from the target.
func (c *NetworkPolicyAttachmentClient) UnsetNetworkPolicy(ctx context.Context, opts UnsetNetworkPolicyOptions) error {
	if err := opts.Validate(); err != nil {
		return NewTerminalError(fmt.Errorf("invalid unset network policy options: %w", err))
	}

	if _, err := c.client.Exec(ctx, buildUnsetNetworkPolicySQL(opts)); err != nil {
		return fmt.Errorf("unsetting network policy from %s %s: %w", opts.TargetType, opts.TargetName, err)
	}

	return nil
}
