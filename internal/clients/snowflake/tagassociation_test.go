package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildSetTagSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicSet", func(t *testing.T) {
		t.Parallel()
		opts := SetTagOptions{
			TagName:    `"DB"."SCH"."COST_CENTER"`,
			TagValue:   "finance",
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."MY_TABLE"`,
		}
		got := buildSetTagSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."SCH"."MY_TABLE" SET TAG "DB"."SCH"."COST_CENTER" = 'finance'`, got)
	})

	t.Run("ValueWithQuotes", func(t *testing.T) {
		t.Parallel()
		opts := SetTagOptions{
			TagName:    `"DB"."SCH"."T"`,
			TagValue:   "it's a test",
			ObjectType: "WAREHOUSE",
			ObjectName: `"MY_WH"`,
		}
		got := buildSetTagSQL(opts)
		assert.Equal(t, `ALTER WAREHOUSE "MY_WH" SET TAG "DB"."SCH"."T" = 'it''s a test'`, got)
	})

	t.Run("DatabaseObject", func(t *testing.T) {
		t.Parallel()
		opts := SetTagOptions{
			TagName:    `"DB"."SCH"."ENV"`,
			TagValue:   "production",
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"`,
		}
		got := buildSetTagSQL(opts)
		assert.Equal(t, `ALTER DATABASE "MY_DB" SET TAG "DB"."SCH"."ENV" = 'production'`, got)
	})
}

func TestBuildUnsetTagSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicUnset", func(t *testing.T) {
		t.Parallel()
		opts := UnsetTagOptions{
			TagName:    `"DB"."SCH"."COST_CENTER"`,
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."MY_TABLE"`,
		}
		got := buildUnsetTagSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."SCH"."MY_TABLE" UNSET TAG "DB"."SCH"."COST_CENTER"`, got)
	})
}

func TestBuildGetTagSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicGet", func(t *testing.T) {
		t.Parallel()
		id := TagAssociationIdentifier{
			TagName:    `"DB"."SCH"."COST_CENTER"`,
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."MY_TABLE"`,
		}
		got := buildGetTagSQL(id)
		assert.Equal(t, `SELECT SYSTEM$GET_TAG('"DB"."SCH"."COST_CENTER"', '"DB"."SCH"."MY_TABLE"', 'TABLE')`, got)
	})

	t.Run("ViewMapsToTableDomain", func(t *testing.T) {
		t.Parallel()
		id := TagAssociationIdentifier{
			TagName:    `"DB"."SCH"."T"`,
			ObjectType: "VIEW",
			ObjectName: `"DB"."SCH"."MY_VIEW"`,
		}
		got := buildGetTagSQL(id)
		// Views use TABLE domain in SYSTEM$GET_TAG.
		assert.Contains(t, got, "'TABLE'")
	})
}

func TestObjectDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		objectType string
		expected   string
	}{
		{"TABLE", "TABLE"},
		{"VIEW", "TABLE"},
		{"WAREHOUSE", "WAREHOUSE"},
		{"DATABASE", "DATABASE"},
		{"SCHEMA", "SCHEMA"},
		{"ROLE", "ROLE"},
		{"USER", "USER"},
		{"DATABASE ROLE", "DATABASE ROLE"},
	}

	for _, tc := range tests {
		t.Run(tc.objectType, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, objectDomain(tc.objectType))
		})
	}
}

// --------------------------------------------------------------------------
// Identifier tests
// --------------------------------------------------------------------------

func TestTagAssociationIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	id := TagAssociationIdentifier{
		TagName:    `"DB"."SCH"."COST_CENTER"`,
		ObjectType: "TABLE",
		ObjectName: `"DB"."SCH"."MY_TABLE"`,
	}
	assert.Equal(t, `TAG "DB"."SCH"."COST_CENTER" ON TABLE "DB"."SCH"."MY_TABLE"`, id.FullyQualifiedName())
	assert.Equal(t, id.FullyQualifiedName(), id.String())
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestSetTagOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := &SetTagOptions{
			TagName:    `"DB"."SCH"."T"`,
			TagValue:   "v",
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."TBL"`,
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingTagName", func(t *testing.T) {
		t.Parallel()
		opts := &SetTagOptions{TagValue: "v", ObjectType: "TABLE", ObjectName: "TBL"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingObjectType", func(t *testing.T) {
		t.Parallel()
		opts := &SetTagOptions{TagName: "T", TagValue: "v", ObjectName: "TBL"}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingObjectName", func(t *testing.T) {
		t.Parallel()
		opts := &SetTagOptions{TagName: "T", TagValue: "v", ObjectType: "TABLE"}
		require.Error(t, opts.Validate())
	})
}

func TestUnsetTagOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetTagOptions{
			TagName:    `"DB"."SCH"."T"`,
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."TBL"`,
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingTagName", func(t *testing.T) {
		t.Parallel()
		opts := &UnsetTagOptions{ObjectType: "TABLE", ObjectName: "TBL"}
		require.Error(t, opts.Validate())
	})
}
