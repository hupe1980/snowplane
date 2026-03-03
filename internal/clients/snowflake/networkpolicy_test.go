package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildEscapedListClause_IP(t *testing.T) {
	t.Parallel()

	t.Run("SingleIP", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("ALLOWED_IP_LIST", []string{"1.2.3.4"})
		assert.Equal(t, "ALLOWED_IP_LIST = ('1.2.3.4')", got)
	})

	t.Run("MultipleIPs", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("BLOCKED_IP_LIST", []string{"1.1.1.1", "2.2.2.2"})
		assert.Equal(t, "BLOCKED_IP_LIST = ('1.1.1.1', '2.2.2.2')", got)
	})

	t.Run("InjectionAttemptEscaped", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("ALLOWED_IP_LIST", []string{"1.2.3.4' OR '1'='1"})
		assert.Equal(t, "ALLOWED_IP_LIST = ('1.2.3.4'' OR ''1''=''1')", got)
	})
}

func TestBuildEscapedListClause_NetworkRule(t *testing.T) {
	t.Parallel()

	t.Run("ValidRules", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("ALLOWED_NETWORK_RULE_LIST", []string{"rule1", "rule2"})
		assert.Equal(t, "ALLOWED_NETWORK_RULE_LIST = ('rule1', 'rule2')", got)
	})

	t.Run("InjectionAttemptEscaped", func(t *testing.T) {
		t.Parallel()
		got := sqlbuilder.BuildEscapedListClause("ALLOWED_NETWORK_RULE_LIST", []string{"rule'; DROP TABLE x;--"})
		assert.Equal(t, "ALLOWED_NETWORK_RULE_LIST = ('rule''; DROP TABLE x;--')", got)
	})
}

func TestBuildCreateNetworkPolicySQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicPolicy", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{
			Name: NewAccountObjectIdentifier("MY_POLICY"),
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE NETWORK POLICY IF NOT EXISTS "MY_POLICY"`, got)
	})

	t.Run("WithAllowedIPs", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("POL"),
			AllowedIPList: []string{"10.0.0.0/8"},
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ALLOWED_IP_LIST = ('10.0.0.0/8')")
	})

	t.Run("WithBlockedIPs", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("POL"),
			BlockedIPList: []string{"192.168.1.1", "192.168.1.2"},
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "BLOCKED_IP_LIST = ('192.168.1.1', '192.168.1.2')")
	})

	t.Run("WithNetworkRules", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{
			Name:                   NewAccountObjectIdentifier("POL"),
			AllowedNetworkRuleList: []string{"rule_a"},
			BlockedNetworkRuleList: []string{"rule_b"},
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "ALLOWED_NETWORK_RULE_LIST = ('rule_a')")
		assert.Contains(t, got, "BLOCKED_NETWORK_RULE_LIST = ('rule_b')")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "security policy"
		opts := CreateNetworkPolicyOptions{
			Name:    NewAccountObjectIdentifier("POL"),
			Comment: &comment,
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, "COMMENT = 'security policy'")
	})

	t.Run("AllOptions", func(t *testing.T) {
		t.Parallel()
		comment := "full"
		opts := CreateNetworkPolicyOptions{
			Name:                   NewAccountObjectIdentifier("FULL_POL"),
			AllowedIPList:          []string{"10.0.0.0/8"},
			BlockedIPList:          []string{"10.0.0.1"},
			AllowedNetworkRuleList: []string{"r1"},
			BlockedNetworkRuleList: []string{"r2"},
			Comment:                &comment,
		}
		got, err := buildCreateNetworkPolicySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, got, `"FULL_POL"`)
		assert.Contains(t, got, "ALLOWED_IP_LIST")
		assert.Contains(t, got, "BLOCKED_IP_LIST")
		assert.Contains(t, got, "ALLOWED_NETWORK_RULE_LIST")
		assert.Contains(t, got, "BLOCKED_NETWORK_RULE_LIST")
		assert.Contains(t, got, "COMMENT")
	})
}

func TestBuildAlterNetworkPolicyStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{
			Name: NewAccountObjectIdentifier("POL"),
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetAllowedIPList", func(t *testing.T) {
		t.Parallel()
		ips := []string{"1.2.3.4"}
		opts := AlterNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("POL"),
			AllowedIPList: &ips,
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET ALLOWED_IP_LIST = ('1.2.3.4')")
	})

	t.Run("SetBlockedIPList", func(t *testing.T) {
		t.Parallel()
		ips := []string{"5.6.7.8"}
		opts := AlterNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("POL"),
			BlockedIPList: &ips,
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET BLOCKED_IP_LIST = ('5.6.7.8')")
	})

	t.Run("SetNetworkRules", func(t *testing.T) {
		t.Parallel()
		allowed := []string{"r1"}
		blocked := []string{"r2"}
		opts := AlterNetworkPolicyOptions{
			Name:                   NewAccountObjectIdentifier("POL"),
			AllowedNetworkRuleList: &allowed,
			BlockedNetworkRuleList: &blocked,
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALLOWED_NETWORK_RULE_LIST = ('r1')")
		assert.Contains(t, stmts[0], "BLOCKED_NETWORK_RULE_LIST = ('r2')")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "new comment"
		opts := AlterNetworkPolicyOptions{
			Name:    NewAccountObjectIdentifier("POL"),
			Comment: &comment,
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'new comment'")
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{
			Name:        NewAccountObjectIdentifier("POL"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterNetworkPolicyStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})
}

func TestBuildShowNetworkPolicyByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowNetworkPolicyByIDSQL(NewAccountObjectIdentifier("MY_POLICY"))
	assert.Contains(t, got, "SHOW NETWORK POLICIES LIKE")
	assert.Contains(t, got, "MY\\_POLICY")
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateNetworkPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{Name: NewAccountObjectIdentifier("P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterNetworkPolicyOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{Name: NewAccountObjectIdentifier("P")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterNetworkPolicyOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{Name: NewAccountObjectIdentifier("P")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("AllowedIPList", func(t *testing.T) {
		t.Parallel()
		ips := []string{"1.2.3.4"}
		opts := AlterNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("P"),
			AllowedIPList: &ips,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("BlockedIPList", func(t *testing.T) {
		t.Parallel()
		ips := []string{"1.2.3.4"}
		opts := AlterNetworkPolicyOptions{
			Name:          NewAccountObjectIdentifier("P"),
			BlockedIPList: &ips,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("AllowedNetworkRuleList", func(t *testing.T) {
		t.Parallel()
		rules := []string{"r1"}
		opts := AlterNetworkPolicyOptions{
			Name:                   NewAccountObjectIdentifier("P"),
			AllowedNetworkRuleList: &rules,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("CommentSet", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterNetworkPolicyOptions{
			Name:    NewAccountObjectIdentifier("P"),
			Comment: &c,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("UnsetFields", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkPolicyOptions{
			Name:        NewAccountObjectIdentifier("P"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}
