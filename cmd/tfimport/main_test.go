package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "MY_DB", "my-db"},
		{"dots", "my.db.name", "my-db-name"},
		{"consecutive underscores", "a__b", "a-b"},
		{"leading trailing", "_foo_", "foo"},
		{"empty", "", "unnamed"},
		{"long", string(make([]byte, 300)), "unnamed"},
		{"mixed", "My_DB.Schema_123", "my-db-schema-123"},
		{"special chars", "DB@#$%^&*()", "db"},
		{"spaces", "my db name", "my-db-name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := sanitizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertDatabase(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":                        "MY_DB",
		"comment":                     "test db",
		"data_retention_time_in_days": float64(7),
		"is_transient":                "true",
	}

	result := convertDatabase(attrs, "default", "snowflake")

	assert.Contains(t, result, "kind: Database")
	assert.Contains(t, result, "name: my-db")
	assert.Contains(t, result, "name: 'MY_DB'")
	assert.Contains(t, result, "comment: 'test db'")
	assert.Contains(t, result, "dataRetentionTimeInDays: 7")
	assert.Contains(t, result, "transient: true")
	assert.Contains(t, result, "deletionPolicy: Orphan")
	assert.Contains(t, result, "namespace: snowflake")
}

func TestConvertSchema(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":     "MY_SCHEMA",
		"database": "MY_DB",
		"comment":  "test schema",
	}

	result := convertSchema(attrs, "default", "default")

	assert.Contains(t, result, "kind: Schema")
	assert.Contains(t, result, "name: my-db-my-schema")
	assert.Contains(t, result, "name: 'MY_SCHEMA'")
	assert.Contains(t, result, "databaseRef:")
	assert.Contains(t, result, "name: my-db")
	assert.Contains(t, result, "comment: 'test schema'")
}

func TestConvertWarehouse(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":           "MY_WH",
		"warehouse_size": "X-Small",
		"auto_suspend":   float64(300),
		"auto_resume":    true,
		"comment":        "test wh",
	}

	result := convertWarehouse(attrs, "prod", "default")

	assert.Contains(t, result, "kind: Warehouse")
	assert.Contains(t, result, "name: my-wh")
	assert.Contains(t, result, "name: 'MY_WH'")
	assert.Contains(t, result, "warehouseSize: 'X-Small'")
	assert.Contains(t, result, "autoSuspend: 300")
	assert.Contains(t, result, "autoResume: true")
	assert.Contains(t, result, "name: prod")
}

func TestConvertUser(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":         "JDOE",
		"login_name":   "jdoe",
		"email":        "jdoe@example.com",
		"comment":      "test user",
		"disabled":     false,
		"default_role": "PUBLIC",
	}

	result := convertUser(attrs, "default", "default")

	assert.Contains(t, result, "kind: User")
	assert.Contains(t, result, "name: 'JDOE'")
	assert.Contains(t, result, "loginName: 'jdoe'")
	assert.Contains(t, result, "email: 'jdoe@example.com'")
	assert.Contains(t, result, "disabled: false")
	assert.Contains(t, result, "defaultRole: 'PUBLIC'")
	// password and rsa keys should NOT appear
	assert.NotContains(t, result, "password")
	assert.NotContains(t, result, "rsaPublicKey")
}

func TestConvertAccountRole(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":    "MY_ROLE",
		"comment": "test role",
	}

	result := convertAccountRole(attrs, "default", "default")

	assert.Contains(t, result, "kind: AccountRole")
	assert.Contains(t, result, "name: 'MY_ROLE'")
	assert.Contains(t, result, "comment: 'test role'")
}

func TestConvertAccountRole_Legacy(t *testing.T) {
	t.Parallel()

	// Verify the legacy "snowflake_role" type maps to AccountRole converter
	converter, ok := resourceTypeMap["snowflake_role"]
	require.True(t, ok)

	attrs := map[string]any{
		"name": "LEGACY_ROLE",
	}

	result := converter(attrs, "default", "default")
	assert.Contains(t, result, "kind: AccountRole")
}

func TestYamlString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "hello", "'hello'"},
		{"with quote", "it's", "'it''s'"},
		{"empty", "", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, yamlString(tt.input))
		})
	}
}

func TestStringAttr(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"present": "value",
		"empty":   "",
		"nil_val": nil,
		"number":  42.0,
	}

	assert.Equal(t, "value", *stringAttr(attrs, "present"))
	assert.Nil(t, stringAttr(attrs, "empty"))
	assert.Nil(t, stringAttr(attrs, "nil_val"))
	assert.Nil(t, stringAttr(attrs, "missing"))
	assert.Nil(t, stringAttr(attrs, "number"))
}

func TestBoolAttr(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"true_val":  true,
		"false_val": false,
		"nil_val":   nil,
		"string":    "true",
	}

	assert.Equal(t, true, *boolAttr(attrs, "true_val"))
	assert.Equal(t, false, *boolAttr(attrs, "false_val"))
	assert.Nil(t, boolAttr(attrs, "nil_val"))
	assert.Nil(t, boolAttr(attrs, "missing"))
	assert.Nil(t, boolAttr(attrs, "string"))
}

func TestInt32Attr(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"num":     float64(42),
		"nil_val": nil,
		"string":  "42",
	}

	assert.Equal(t, int32(42), *int32Attr(attrs, "num"))
	assert.Nil(t, int32Attr(attrs, "nil_val"))
	assert.Nil(t, int32Attr(attrs, "missing"))
	assert.Nil(t, int32Attr(attrs, "string"))
}

func TestIsTruthy(t *testing.T) {
	t.Parallel()

	assert.True(t, isTruthy(map[string]any{"k": true}, "k"))
	assert.True(t, isTruthy(map[string]any{"k": "true"}, "k"))
	assert.True(t, isTruthy(map[string]any{"k": "TRUE"}, "k"))
	assert.False(t, isTruthy(map[string]any{"k": false}, "k"))
	assert.False(t, isTruthy(map[string]any{"k": "false"}, "k"))
	assert.False(t, isTruthy(map[string]any{"k": nil}, "k"))
	assert.False(t, isTruthy(map[string]any{}, "k"))
}

func TestConvertDatabase_NilName(t *testing.T) {
	t.Parallel()

	result := convertDatabase(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertSchema_NilName(t *testing.T) {
	t.Parallel()

	result := convertSchema(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertWarehouse_NilName(t *testing.T) {
	t.Parallel()

	result := convertWarehouse(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertUser_NilName(t *testing.T) {
	t.Parallel()

	result := convertUser(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertAccountRole_NilName(t *testing.T) {
	t.Parallel()

	result := convertAccountRole(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertUser_WithType(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":      "SVC_USER",
		"user_type": "SERVICE",
	}

	result := convertUser(attrs, "default", "default")
	assert.Contains(t, result, "type: 'SERVICE'")
}

func TestConvertDatabase_TransientBool(t *testing.T) {
	t.Parallel()

	// is_transient as bool (not string) should also work
	attrs := map[string]any{
		"name":         "MY_DB",
		"is_transient": true,
	}

	result := convertDatabase(attrs, "default", "default")
	assert.Contains(t, result, "transient: true")
}

func TestConvertDatabaseRole(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":     "DATA_READER",
		"database": "MY_DB",
		"comment":  "read-only role",
	}

	result := convertDatabaseRole(attrs, "default", "snowflake")
	assert.Contains(t, result, "kind: DatabaseRole")
	assert.Contains(t, result, "name: my-db-data-reader")
	assert.Contains(t, result, "name: 'DATA_READER'")
	assert.Contains(t, result, "databaseRef:")
	assert.Contains(t, result, "name: my-db")
	assert.Contains(t, result, "comment: 'read-only role'")
	assert.Contains(t, result, "deletionPolicy: Orphan")
}

func TestConvertDatabaseRole_NilName(t *testing.T) {
	t.Parallel()

	result := convertDatabaseRole(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertGrantPrivilegesToAccountRole_AccountObject(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"account_role_name": "ANALYST",
		"privileges":        []any{"USAGE"},
		"on_account_object": []any{
			map[string]any{
				"object_type": "DATABASE",
				"object_name": "MY_DB",
			},
		},
	}

	result := convertGrantPrivilegesToAccountRole(attrs, "default", "default")
	assert.Contains(t, result, "kind: GrantPrivilegesToAccountRole")
	assert.Contains(t, result, "privilege: 'USAGE'")
	assert.Contains(t, result, "accountRole: 'ANALYST'")
	assert.Contains(t, result, "accountObject:")
	assert.Contains(t, result, "objectType: 'DATABASE'")
	assert.Contains(t, result, "objectName: 'MY_DB'")
	assert.Contains(t, result, "deletionPolicy: Orphan")
}

func TestConvertGrantPrivilegesToAccountRole_OnAccount(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"account_role_name": "ADMIN",
		"privileges":        []any{"CREATE DATABASE"},
		"on_account":        true,
	}

	result := convertGrantPrivilegesToAccountRole(attrs, "default", "default")
	assert.Contains(t, result, "account: true")
	assert.Contains(t, result, "privilege: 'CREATE DATABASE'")
}

func TestConvertGrantPrivilegesToAccountRole_WithGrantOption(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"account_role_name": "ADMIN",
		"privileges":        []any{"USAGE"},
		"on_account":        true,
		"with_grant_option": true,
	}

	result := convertGrantPrivilegesToAccountRole(attrs, "default", "default")
	assert.Contains(t, result, "withGrantOption: true")
}

func TestConvertGrantPrivilegesToAccountRole_AllPrivileges(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"account_role_name": "ADMIN",
		"all_privileges":    true,
		"on_account":        true,
	}

	result := convertGrantPrivilegesToAccountRole(attrs, "default", "default")
	assert.Contains(t, result, "allPrivileges: true")
	assert.NotContains(t, result, "privilege:")
}

func TestConvertGrantPrivilegesToAccountRole_Empty(t *testing.T) {
	t.Parallel()

	result := convertGrantPrivilegesToAccountRole(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertGrantPrivilegesToDatabaseRole(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"database_role_name": "MY_DB.READER",
		"privileges":         []any{"SELECT"},
		"on_schema_object": []any{
			map[string]any{
				"object_type": "TABLE",
				"object_name": "MY_DB.PUBLIC.ORDERS",
			},
		},
	}

	result := convertGrantPrivilegesToDatabaseRole(attrs, "default", "default")
	assert.Contains(t, result, "kind: GrantPrivilegesToDatabaseRole")
	assert.Contains(t, result, "privilege: 'SELECT'")
	assert.Contains(t, result, "databaseRole: 'MY_DB.READER'")
	assert.Contains(t, result, "schemaObject:")
	assert.Contains(t, result, "objectType: 'TABLE'")
}

func TestConvertGrantPrivilegesToAccountRole_FutureGrant(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"account_role_name": "ANALYST",
		"privileges":        []any{"SELECT"},
		"on_schema_object": []any{
			map[string]any{
				"future": []any{
					map[string]any{
						"object_type_plural": "TABLES",
						"in_database":        "MY_DB",
					},
				},
			},
		},
	}

	result := convertGrantPrivilegesToAccountRole(attrs, "default", "default")
	assert.Contains(t, result, "future:")
	assert.Contains(t, result, "objectTypePlural: 'TABLES'")
	assert.Contains(t, result, "inDatabase: 'MY_DB'")
}

func TestConvertGrantPrivilegesToShare(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"USAGE"},
		"on_database": []any{
			map[string]any{
				"database_name": "SHARED_DB",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "kind: GrantPrivilegesToShare")
	assert.Contains(t, result, "privilege: 'USAGE'")
	assert.Contains(t, result, "on:\n    database: 'SHARED_DB'")
	assert.Contains(t, result, "share: 'MY_SHARE'")
	assert.Contains(t, result, "deletionPolicy: Orphan")
}

func TestConvertGrantPrivilegesToShare_Empty(t *testing.T) {
	t.Parallel()

	result := convertGrantPrivilegesToShare(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertGrantPrivilegesToDatabaseRole_AllPrivileges(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"database_role_name": "MY_DB.ADMIN",
		"all_privileges":     true,
		"on_database":        "MY_DB",
	}

	result := convertGrantPrivilegesToDatabaseRole(attrs, "default", "default")
	assert.Contains(t, result, "allPrivileges: true")
	assert.NotContains(t, result, "privilege:")
	assert.Contains(t, result, "databaseRole: 'MY_DB.ADMIN'")
}

func TestConvertGrantPrivilegesToDatabaseRole_OnDatabase(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"database_role_name": "MY_DB.READER",
		"privileges":         []any{"USAGE"},
		"on_database":        "MY_DB",
	}

	result := convertGrantPrivilegesToDatabaseRole(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    database: 'MY_DB'")
	assert.NotContains(t, result, "accountObject:")
}

func TestConvertGrantPrivilegesToDatabaseRole_OnSchema(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"database_role_name": "MY_DB.READER",
		"privileges":         []any{"USAGE"},
		"on_schema": []any{
			map[string]any{
				"schema_name": "MY_DB.PUBLIC",
			},
		},
	}

	result := convertGrantPrivilegesToDatabaseRole(attrs, "default", "default")
	assert.Contains(t, result, "schema:")
	assert.Contains(t, result, "schemaName: 'MY_DB.PUBLIC'")
}

func TestConvertGrantPrivilegesToDatabaseRole_Empty(t *testing.T) {
	t.Parallel()

	result := convertGrantPrivilegesToDatabaseRole(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertGrantPrivilegesToShare_OnSchema(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"USAGE"},
		"on_schema": []any{
			map[string]any{
				"schema_name": "MY_DB.PUBLIC",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    schema: 'MY_DB.PUBLIC'")
}

func TestConvertGrantPrivilegesToShare_OnTable(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"SELECT"},
		"on_table": []any{
			map[string]any{
				"table_name": "MY_DB.PUBLIC.ORDERS",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    table: 'MY_DB.PUBLIC.ORDERS'")
}

func TestConvertGrantPrivilegesToShare_OnView(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"SELECT"},
		"on_view": []any{
			map[string]any{
				"view_name": "MY_DB.PUBLIC.MY_VIEW",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    view: 'MY_DB.PUBLIC.MY_VIEW'")
}

func TestConvertGrantPrivilegesToShare_OnAllTablesInSchema(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"SELECT"},
		"on_all_tables_in_schema": []any{
			map[string]any{
				"schema_name": "MY_DB.PUBLIC",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    allTablesInSchema: 'MY_DB.PUBLIC'")
}

func TestConvertGrantPrivilegesToShare_OnFunction(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"USAGE"},
		"on_function": []any{
			map[string]any{
				"function_name": "MY_DB.PUBLIC.MY_FUNC",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    functionName: 'MY_DB.PUBLIC.MY_FUNC'")
}

func TestConvertGrantPrivilegesToShare_OnTag(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"share_name": "MY_SHARE",
		"privileges": []any{"READ"},
		"on_tag": []any{
			map[string]any{
				"tag_name": "MY_DB.PUBLIC.MY_TAG",
			},
		},
	}

	result := convertGrantPrivilegesToShare(attrs, "default", "default")
	assert.Contains(t, result, "on:\n    tag: 'MY_DB.PUBLIC.MY_TAG'")
}

func TestConvertTable(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":     "ORDERS",
		"database": "MY_DB",
		"schema":   "PUBLIC",
		"comment":  "order table",
		"column": []any{
			map[string]any{"name": "ID", "type": "NUMBER(38,0)", "nullable": false},
			map[string]any{"name": "NAME", "type": "VARCHAR(100)", "comment": "customer name"},
		},
		"change_tracking": true,
	}

	result := convertTable(attrs, "default", "default")
	assert.Contains(t, result, "kind: Table")
	assert.Contains(t, result, "name: my-db-public-orders")
	assert.Contains(t, result, "name: 'ORDERS'")
	assert.Contains(t, result, "databaseRef:")
	assert.Contains(t, result, "schemaRef:")
	assert.Contains(t, result, "columns:")
	assert.Contains(t, result, "name: 'ID'")
	assert.Contains(t, result, "type: 'NUMBER(38,0)'")
	assert.Contains(t, result, "nullable: false")
	assert.Contains(t, result, "name: 'NAME'")
	assert.Contains(t, result, "comment: 'customer name'")
	assert.Contains(t, result, "changeTracking: true")
}

func TestConvertTable_NilName(t *testing.T) {
	t.Parallel()

	result := convertTable(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertTable_ClusterBy(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":       "EVENTS",
		"database":   "MY_DB",
		"schema":     "PUBLIC",
		"cluster_by": []any{"created_at", "region"},
		"column":     []any{map[string]any{"name": "ID", "type": "NUMBER"}},
	}

	result := convertTable(attrs, "default", "default")
	assert.Contains(t, result, "clusterBy:")
	assert.Contains(t, result, "- 'created_at'")
	assert.Contains(t, result, "- 'region'")
}

func TestConvertView(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":      "ACTIVE_USERS",
		"database":  "MY_DB",
		"schema":    "PUBLIC",
		"statement": "SELECT * FROM users WHERE active = TRUE",
		"is_secure": "true",
		"comment":   "active users view",
	}

	result := convertView(attrs, "default", "default")
	assert.Contains(t, result, "kind: View")
	assert.Contains(t, result, "name: my-db-public-active-users")
	assert.Contains(t, result, "name: 'ACTIVE_USERS'")
	assert.Contains(t, result, "databaseRef:")
	assert.Contains(t, result, "schemaRef:")
	assert.Contains(t, result, "statement: |")
	assert.Contains(t, result, "SELECT * FROM users WHERE active = TRUE")
	assert.Contains(t, result, "secure: true")
	assert.Contains(t, result, "comment: 'active users view'")
}

func TestConvertView_NilName(t *testing.T) {
	t.Parallel()

	result := convertView(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestConvertStage_Internal(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":     "MY_STAGE",
		"database": "MY_DB",
		"schema":   "PUBLIC",
		"comment":  "internal stage",
	}

	result := convertStage(attrs, "default", "default")
	assert.Contains(t, result, "kind: Stage")
	assert.Contains(t, result, "name: my-db-public-my-stage")
	assert.Contains(t, result, "name: 'MY_STAGE'")
	assert.Contains(t, result, "databaseRef:")
	assert.Contains(t, result, "schemaRef:")
	assert.Contains(t, result, "comment: 'internal stage'")
	assert.NotContains(t, result, "url:")
}

func TestConvertStage_External(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"name":                "S3_STAGE",
		"database":            "MY_DB",
		"schema":              "PUBLIC",
		"url":                 "s3://my-bucket/path/",
		"storage_integration": "MY_S3_INT",
	}

	result := convertStage(attrs, "default", "default")
	assert.Contains(t, result, "url: 's3://my-bucket/path/'")
	assert.Contains(t, result, "storageIntegration: 'MY_S3_INT'")
}

func TestConvertStage_NilName(t *testing.T) {
	t.Parallel()

	result := convertStage(map[string]any{}, "default", "default")
	assert.Equal(t, "", result)
}

func TestApiVersion(t *testing.T) {
	t.Parallel()

	// All converters should emit the correct apiVersion
	attrs := map[string]any{"name": "TEST"}
	for _, fn := range []func(map[string]any, string, string) string{
		convertDatabase, convertWarehouse, convertUser, convertAccountRole,
	} {
		result := fn(attrs, "default", "default")
		assert.Contains(t, result, "apiVersion: snowplane.hupe1980.github.io/v1alpha1")
	}
}

func TestBuildStateAddress_Simple(t *testing.T) {
	t.Parallel()

	res := StateResource{Type: "snowflake_database", Name: "my_db"}
	inst := StateInstance{Attributes: map[string]any{"name": "MY_DB"}}

	addr := buildStateAddress(res, inst)
	assert.Equal(t, "snowflake_database.my_db", addr)
}

func TestBuildStateAddress_WithCountIndex(t *testing.T) {
	t.Parallel()

	res := StateResource{Type: "snowflake_schema", Name: "schemas"}
	inst := StateInstance{IndexKey: float64(2), Attributes: map[string]any{"name": "S"}}

	addr := buildStateAddress(res, inst)
	assert.Equal(t, "snowflake_schema.schemas[2]", addr)
}

func TestBuildStateAddress_WithForEachKey(t *testing.T) {
	t.Parallel()

	res := StateResource{Type: "snowflake_database", Name: "dbs"}
	inst := StateInstance{IndexKey: "analytics", Attributes: map[string]any{"name": "ANALYTICS"}}

	addr := buildStateAddress(res, inst)
	assert.Equal(t, `snowflake_database.dbs["analytics"]`, addr)
}

func TestBuildStateAddress_WithModule(t *testing.T) {
	t.Parallel()

	res := StateResource{Module: "module.snowflake", Type: "snowflake_database", Name: "my_db"}
	inst := StateInstance{Attributes: map[string]any{"name": "MY_DB"}}

	addr := buildStateAddress(res, inst)
	assert.Equal(t, "module.snowflake.snowflake_database.my_db", addr)
}

func TestGenerateRemoveScript(t *testing.T) {
	t.Parallel()

	addrs := []string{
		"snowflake_database.my_db",
		`snowflake_schema.schemas["public"]`,
	}

	script := generateRemoveScript(addrs)

	// Header
	assert.Contains(t, script, "#!/usr/bin/env bash")
	assert.Contains(t, script, "TF=\"${TF_CMD:-terraform}\"")
	assert.Contains(t, script, "--dry-run")

	// Addresses
	assert.Contains(t, script, "snowflake_database.my_db")
	assert.Contains(t, script, `snowflake_schema.schemas["public"]`)
	assert.Contains(t, script, "state rm")

	// Count
	assert.Contains(t, script, "removed 2 resource(s)")
}

func TestGenerateRemoveScript_OpenTofu(t *testing.T) {
	t.Parallel()

	script := generateRemoveScript([]string{"snowflake_database.db"})

	// The script should support TF_CMD=tofu
	assert.Contains(t, script, "TF_CMD")
	assert.Contains(t, script, "${TF}")
}

func TestSliceAttr(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"tags":    []any{"tag1", "tag2", ""},
		"empty":   []any{},
		"notlist": "hello",
	}

	assert.Equal(t, []string{"tag1", "tag2"}, sliceAttr(attrs, "tags"))
	assert.Nil(t, sliceAttr(attrs, "empty"))
	assert.Nil(t, sliceAttr(attrs, "notlist"))
	assert.Nil(t, sliceAttr(attrs, "missing"))
}

func TestFirstBlock(t *testing.T) {
	t.Parallel()

	attrs := map[string]any{
		"as_list": []any{
			map[string]any{"key": "val"},
		},
		"as_map":   map[string]any{"key": "val2"},
		"empty":    []any{},
		"notblock": "string",
	}

	assert.Equal(t, map[string]any{"key": "val"}, firstBlock(attrs, "as_list"))
	assert.Equal(t, map[string]any{"key": "val2"}, firstBlock(attrs, "as_map"))
	assert.Nil(t, firstBlock(attrs, "empty"))
	assert.Nil(t, firstBlock(attrs, "notblock"))
	assert.Nil(t, firstBlock(attrs, "missing"))
}
