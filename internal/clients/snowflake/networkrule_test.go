package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildValueListClause(t *testing.T) {
	t.Parallel()

	t.Run("SingleValue", func(t *testing.T) {
		t.Parallel()
		got := buildValueListClause([]string{"1.2.3.4"})
		assert.Equal(t, "VALUE_LIST = ('1.2.3.4')", got)
	})

	t.Run("MultipleValues", func(t *testing.T) {
		t.Parallel()
		got := buildValueListClause([]string{"1.2.3.4", "5.6.7.8"})
		assert.Equal(t, "VALUE_LIST = ('1.2.3.4', '5.6.7.8')", got)
	})
}

func TestBuildCreateNetworkRuleSQL(t *testing.T) {
	t.Parallel()

	t.Run("IPV4Basic", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MY_RULE"),
			Type:      "IPV4",
			Mode:      "INGRESS",
			ValueList: []string{"1.2.3.4", "10.0.0.0/24"},
		}
		got := buildCreateNetworkRuleSQL(opts)
		assert.Contains(t, got, `CREATE NETWORK RULE IF NOT EXISTS "DB"."SCH"."MY_RULE"`)
		assert.Contains(t, got, "TYPE = IPV4")
		assert.Contains(t, got, "MODE = INGRESS")
		assert.Contains(t, got, "VALUE_LIST = ('1.2.3.4', '10.0.0.0/24')")
	})

	t.Run("HostPortEgress", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "EGRESS_RULE"),
			Type:      "HOST_PORT",
			Mode:      "EGRESS",
			ValueList: []string{"example.com:443", "api.example.com:8080"},
		}
		got := buildCreateNetworkRuleSQL(opts)
		assert.Contains(t, got, "TYPE = HOST_PORT")
		assert.Contains(t, got, "MODE = EGRESS")
		assert.Contains(t, got, "example.com:443")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		comment := "allow office IPs"
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "RULE"),
			Type:      "IPV4",
			Mode:      "INGRESS",
			ValueList: []string{"10.0.0.1"},
			Comment:   &comment,
		}
		got := buildCreateNetworkRuleSQL(opts)
		assert.Contains(t, got, "COMMENT = 'allow office IPs'")
	})

	t.Run("AWSVPCEIDInternalStage", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "VPCE_RULE"),
			Type:      "AWSVPCEID",
			Mode:      "INTERNAL_STAGE",
			ValueList: []string{"vpce-01234567890abcdef"},
		}
		got := buildCreateNetworkRuleSQL(opts)
		assert.Contains(t, got, "TYPE = AWSVPCEID")
		assert.Contains(t, got, "MODE = INTERNAL_STAGE")
		assert.Contains(t, got, "vpce-01234567890abcdef")
	})
}

func TestBuildAlterNetworkRuleStatements(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "R"),
		}
		stmts, err := buildAlterNetworkRuleStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetValueList", func(t *testing.T) {
		t.Parallel()
		vals := []string{"10.0.0.1", "10.0.0.2"}
		opts := AlterNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "R"),
			ValueList: &vals,
		}
		stmts, err := buildAlterNetworkRuleStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "ALTER NETWORK RULE")
		assert.Contains(t, stmts[0], "VALUE_LIST = ('10.0.0.1', '10.0.0.2')")
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()
		comment := "updated"
		opts := AlterNetworkRuleOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "R"),
			Comment: &comment,
		}
		stmts, err := buildAlterNetworkRuleStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'updated'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "R"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterNetworkRuleStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("ValueListAndComment", func(t *testing.T) {
		t.Parallel()
		vals := []string{"10.0.0.1"}
		comment := "c"
		opts := AlterNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "R"),
			ValueList: &vals,
			Comment:   &comment,
		}
		stmts, err := buildAlterNetworkRuleStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		// First statement: VALUE_LIST SET
		assert.Contains(t, stmts[0], "VALUE_LIST")
		// Second statement: COMMENT SET
		assert.Contains(t, stmts[1], "COMMENT = 'c'")
	})
}

func TestBuildShowNetworkRuleByIDSQL(t *testing.T) {
	t.Parallel()
	got := buildShowNetworkRuleByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_RULE"))
	assert.Contains(t, got, "SHOW NETWORK RULES LIKE")
	assert.Contains(t, got, "MY\\_RULE")
	assert.Contains(t, got, `IN SCHEMA "DB"."SCH"`)
}

func TestCreateNetworkRuleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "R"),
			Type:      "IPV4",
			Mode:      "INGRESS",
			ValueList: []string{"1.2.3.4"},
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Type:      "IPV4",
			Mode:      "INGRESS",
			ValueList: []string{"1.2.3.4"},
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "R"),
			Mode:      "INGRESS",
			ValueList: []string{"1.2.3.4"},
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingMode", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "R"),
			Type:      "IPV4",
			ValueList: []string{"1.2.3.4"},
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("EmptyValueList", func(t *testing.T) {
		t.Parallel()
		opts := CreateNetworkRuleOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "R"),
			Type: "IPV4",
			Mode: "INGRESS",
		}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterNetworkRuleOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "R")}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{}
		assert.Error(t, opts.Validate())
	})
}

func TestAlterNetworkRuleOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "R")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithValueList", func(t *testing.T) {
		t.Parallel()
		vals := []string{"10.0.0.1"}
		opts := AlterNetworkRuleOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "R"), ValueList: &vals}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()
		c := "x"
		opts := AlterNetworkRuleOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "R"), Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnset", func(t *testing.T) {
		t.Parallel()
		opts := AlterNetworkRuleOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "R"), UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}
