package snowflake

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildGrantSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicGrant_AccountObject", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
			ToClause:  `TO ROLE "ANALYST"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT USAGE ON DATABASE "MY_DB" TO ROLE "ANALYST"`, got)
	})

	t.Run("WithGrantOption", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege:       "SELECT",
			OnClause:        `ON TABLE "MY_DB"."PUBLIC"."MY_TABLE"`,
			ToClause:        `TO ROLE "ANALYST"`,
			WithGrantOption: true,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT SELECT ON TABLE "MY_DB"."PUBLIC"."MY_TABLE" TO ROLE "ANALYST" WITH GRANT OPTION`, got)
	})

	t.Run("OnAccount", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "CREATE DATABASE",
			OnClause:  "ON ACCOUNT",
			ToClause:  `TO ROLE "SYSADMIN"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT CREATE DATABASE ON ACCOUNT TO ROLE "SYSADMIN"`, got)
	})

	t.Run("FutureGrant", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "SELECT",
			OnClause:  `ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC"`,
			ToClause:  `TO ROLE "ANALYST"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT SELECT ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC" TO ROLE "ANALYST"`, got)
	})

	t.Run("AllTablesGrant", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "SELECT",
			OnClause:  `ON ALL TABLES IN DATABASE "MY_DB"`,
			ToClause:  `TO ROLE "ANALYST"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT SELECT ON ALL TABLES IN DATABASE "MY_DB" TO ROLE "ANALYST"`, got)
	})

	t.Run("GrantToShare", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
			ToClause:  `TO SHARE "my_share"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT USAGE ON DATABASE "MY_DB" TO SHARE "my_share"`, got)
	})

	t.Run("DatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
			ToClause:  `TO DATABASE ROLE "MY_DB"."DR1"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT USAGE ON DATABASE "MY_DB" TO DATABASE ROLE "MY_DB"."DR1"`, got)
	})

	t.Run("SchemaPrivilege", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "CREATE TABLE",
			OnClause:  `ON SCHEMA "MY_DB"."PUBLIC"`,
			ToClause:  `TO ROLE "DEVELOPER"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT CREATE TABLE ON SCHEMA "MY_DB"."PUBLIC" TO ROLE "DEVELOPER"`, got)
	})

	t.Run("FutureSchemasInDatabase", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON FUTURE SCHEMAS IN DATABASE "MY_DB"`,
			ToClause:  `TO ROLE "ANALYST"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT USAGE ON FUTURE SCHEMAS IN DATABASE "MY_DB" TO ROLE "ANALYST"`, got)
	})

	t.Run("AllSchemasGrant", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "MODIFY",
			OnClause:  `ON ALL SCHEMAS IN DATABASE "MY_DB"`,
			ToClause:  `TO ROLE "ADMIN"`,
		}
		got := buildGrantSQL(opts)
		assert.Equal(t, `GRANT MODIFY ON ALL SCHEMAS IN DATABASE "MY_DB" TO ROLE "ADMIN"`, got)
	})
}

func TestBuildRevokeSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicRevoke", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege:  "USAGE",
			OnClause:   `ON DATABASE "MY_DB"`,
			FromClause: `FROM ROLE "ANALYST"`,
		}
		got := buildRevokeSQL(opts)
		assert.Equal(t, `REVOKE USAGE ON DATABASE "MY_DB" FROM ROLE "ANALYST"`, got)
	})

	t.Run("FutureRevoke", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege:  "SELECT",
			OnClause:   `ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC"`,
			FromClause: `FROM ROLE "ANALYST"`,
		}
		got := buildRevokeSQL(opts)
		assert.Equal(t, `REVOKE SELECT ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC" FROM ROLE "ANALYST"`, got)
	})

	t.Run("RevokeFromShare", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege:  "USAGE",
			OnClause:   `ON DATABASE "MY_DB"`,
			FromClause: `FROM SHARE "my_share"`,
		}
		got := buildRevokeSQL(opts)
		assert.Equal(t, `REVOKE USAGE ON DATABASE "MY_DB" FROM SHARE "my_share"`, got)
	})

	t.Run("DatabaseRoleRevoke", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege:  "SELECT",
			OnClause:   `ON TABLE "DB"."SCH"."T1"`,
			FromClause: `FROM DATABASE ROLE "DB"."DR1"`,
		}
		got := buildRevokeSQL(opts)
		assert.Equal(t, `REVOKE SELECT ON TABLE "DB"."SCH"."T1" FROM DATABASE ROLE "DB"."DR1"`, got)
	})
}

// --------------------------------------------------------------------------
// BuildOnClause tests
// --------------------------------------------------------------------------

func TestBuildOnClause(t *testing.T) {
	t.Parallel()

	t.Run("OnAccount", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{Account: true})
		assert.Equal(t, "ON ACCOUNT", got)
	})

	t.Run("OnAccountObject", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{AccountObjectType: "DATABASE", AccountObjectName: "MY_DB"})
		assert.Equal(t, `ON DATABASE "MY_DB"`, got)
	})

	t.Run("OnSchema", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{SchemaName: "MY_DB.PUBLIC"})
		assert.Equal(t, `ON SCHEMA "MY_DB"."PUBLIC"`, got)
	})

	t.Run("AllSchemasInDatabase", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{AllSchemasInDB: "MY_DB"})
		assert.Equal(t, `ON ALL SCHEMAS IN DATABASE "MY_DB"`, got)
	})

	t.Run("FutureSchemasInDatabase", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{FutureSchemasInDB: "MY_DB"})
		assert.Equal(t, `ON FUTURE SCHEMAS IN DATABASE "MY_DB"`, got)
	})

	t.Run("OnSchemaObject", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{SchemaObjectType: "TABLE", SchemaObjectName: "DB.SCH.T1"})
		assert.Equal(t, `ON TABLE "DB"."SCH"."T1"`, got)
	})

	t.Run("AllTablesInDatabase", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{AllObjectsTypePlural: "TABLES", AllObjectsInDB: "MY_DB"})
		assert.Equal(t, `ON ALL TABLES IN DATABASE "MY_DB"`, got)
	})

	t.Run("AllTablesInSchema", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{AllObjectsTypePlural: "TABLES", AllObjectsInSchema: "MY_DB.PUBLIC"})
		assert.Equal(t, `ON ALL TABLES IN SCHEMA "MY_DB"."PUBLIC"`, got)
	})

	t.Run("FutureTablesInDatabase", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{FutureObjectsTypePlural: "TABLES", FutureObjectsInDB: "MY_DB"})
		assert.Equal(t, `ON FUTURE TABLES IN DATABASE "MY_DB"`, got)
	})

	t.Run("FutureTablesInSchema", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{FutureObjectsTypePlural: "TABLES", FutureObjectsInSchema: "MY_DB.PUBLIC"})
		assert.Equal(t, `ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC"`, got)
	})

	t.Run("SQLInjectionPrevented", func(t *testing.T) {
		t.Parallel()
		got := BuildOnClause(OnClauseParams{
			AccountObjectType: "DATABASE",
			AccountObjectName: `MY_DB"; DROP DATABASE PROD; --`,
		})
		// The embedded double-quote is escaped to "" by QuoteIdentifier,
		// producing a single safe identifier token.
		assert.Equal(t, `ON DATABASE "MY_DB""; DROP DATABASE PROD; --"`, got)
		// Verify the injection payload is contained within the quoted identifier.
		assert.True(t, strings.HasPrefix(got, `ON DATABASE "`))
		assert.True(t, strings.HasSuffix(got, `"`))
	})
}

// --------------------------------------------------------------------------
// BuildToClause / BuildFromClause tests
// --------------------------------------------------------------------------

func TestBuildToClause(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `TO ROLE "ANALYST"`, BuildToClause("ANALYST", "", ""))
	})

	t.Run("DatabaseRole", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `TO DATABASE ROLE "MY_DB"."DR1"`, BuildToClause("", "MY_DB.DR1", ""))
	})

	t.Run("Share", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `TO SHARE "my_share"`, BuildToClause("", "", "my_share"))
	})

	t.Run("DatabaseRoleSQLInjection", func(t *testing.T) {
		t.Parallel()
		got := BuildToClause("", `DB"; DROP DATABASE PROD; --.ROLE`, "")
		assert.Equal(t, `TO DATABASE ROLE "DB""; DROP DATABASE PROD; --"."ROLE"`, got)
	})
}

func TestBuildFromClause(t *testing.T) {
	t.Parallel()

	t.Run("AccountRole", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `FROM ROLE "ANALYST"`, BuildFromClause("ANALYST", "", ""))
	})

	t.Run("DatabaseRole", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `FROM DATABASE ROLE "MY_DB"."DR1"`, BuildFromClause("", "MY_DB.DR1", ""))
	})

	t.Run("Share", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, `FROM SHARE "my_share"`, BuildFromClause("", "", "my_share"))
	})
}

// --------------------------------------------------------------------------
// BuildShowGrantsTarget tests
// --------------------------------------------------------------------------

func TestBuildShowGrantsTarget(t *testing.T) {
	t.Parallel()

	t.Run("OnDatabase", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{AccountObjectType: "DATABASE", AccountObjectName: "MY_DB"}, "")
		assert.Equal(t, `ON DATABASE "MY_DB"`, target)
		assert.False(t, future)
	})

	t.Run("OnAccount", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{Account: true}, "")
		assert.Equal(t, "ON ACCOUNT", target)
		assert.False(t, future)
	})

	t.Run("ToShare", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{}, "my_share")
		assert.Equal(t, `TO SHARE "my_share"`, target)
		assert.False(t, future)
	})

	t.Run("FutureTablesInDatabase", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{FutureObjectsTypePlural: "TABLES", FutureObjectsInDB: "MY_DB"}, "")
		assert.Equal(t, `IN DATABASE "MY_DB"`, target)
		assert.True(t, future)
	})

	t.Run("FutureSchemasInDatabase", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{FutureSchemasInDB: "MY_DB"}, "")
		assert.Equal(t, `IN DATABASE "MY_DB"`, target)
		assert.True(t, future)
	})

	t.Run("FutureTablesInSchema", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{FutureObjectsTypePlural: "TABLES", FutureObjectsInSchema: "MY_DB.PUBLIC"}, "")
		assert.Equal(t, `IN SCHEMA "MY_DB"."PUBLIC"`, target)
		assert.True(t, future)
	})

	t.Run("OnSchema", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{SchemaName: "MY_DB.PUBLIC"}, "")
		assert.Equal(t, `ON SCHEMA "MY_DB"."PUBLIC"`, target)
		assert.False(t, future)
	})

	t.Run("AllSchemasInDB", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{AllSchemasInDB: "MY_DB"}, "")
		assert.Equal(t, `ON DATABASE "MY_DB"`, target)
		assert.False(t, future)
	})

	t.Run("AllObjectsInSchema", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{AllObjectsTypePlural: "TABLES", AllObjectsInSchema: "MY_DB.PUBLIC"}, "")
		assert.Equal(t, `ON SCHEMA "MY_DB"."PUBLIC"`, target)
		assert.False(t, future)
	})

	t.Run("AllObjectsInDB", func(t *testing.T) {
		t.Parallel()
		target, future := BuildShowGrantsTarget(OnClauseParams{AllObjectsTypePlural: "TABLES", AllObjectsInDB: "MY_DB"}, "")
		assert.Equal(t, `ON DATABASE "MY_DB"`, target)
		assert.False(t, future)
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateGrantOptionsValidation(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
			ToClause:  `TO ROLE "ANALYST"`,
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingPrivilege", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			OnClause: `ON DATABASE "MY_DB"`,
			ToClause: `TO ROLE "ANALYST"`,
		}
		assert.ErrorContains(t, opts.Validate(), "privilege is required")
	})

	t.Run("MissingOnClause", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			ToClause:  `TO ROLE "ANALYST"`,
		}
		assert.ErrorContains(t, opts.Validate(), "onClause is required")
	})

	t.Run("MissingToClause", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
		}
		assert.ErrorContains(t, opts.Validate(), "toClause is required")
	})
}

func TestRevokeGrantOptionsValidation(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege:  "USAGE",
			OnClause:   `ON DATABASE "MY_DB"`,
			FromClause: `FROM ROLE "ANALYST"`,
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingPrivilege", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			OnClause:   `ON DATABASE "MY_DB"`,
			FromClause: `FROM ROLE "ANALYST"`,
		}
		assert.ErrorContains(t, opts.Validate(), "privilege is required")
	})

	t.Run("MissingFromClause", func(t *testing.T) {
		t.Parallel()
		opts := RevokeGrantOptions{
			Privilege: "USAGE",
			OnClause:  `ON DATABASE "MY_DB"`,
		}
		assert.ErrorContains(t, opts.Validate(), "fromClause is required")
	})
}

// --------------------------------------------------------------------------
// GrantIdentifier tests
// --------------------------------------------------------------------------

func TestGrantIdentifier(t *testing.T) {
	t.Parallel()

	t.Run("FullyQualifiedName", func(t *testing.T) {
		t.Parallel()
		id := GrantIdentifier{
			Kind:             GrantKindRegular,
			Privilege:        "USAGE",
			OnClause:         `ON DATABASE "MY_DB"`,
			ToClause:         `TO ROLE "ANALYST"`,
			GranteeName:      "ANALYST",
			ShowGrantsTarget: `ON DATABASE "MY_DB"`,
		}
		assert.Equal(t, `GRANT USAGE ON DATABASE "MY_DB" TO ROLE "ANALYST"`, id.FullyQualifiedName())
	})

	t.Run("FutureGrant", func(t *testing.T) {
		t.Parallel()
		id := GrantIdentifier{
			Kind:             GrantKindFuture,
			Privilege:        "SELECT",
			OnClause:         `ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC"`,
			ToClause:         `TO ROLE "ANALYST"`,
			GranteeName:      "ANALYST",
			ShowGrantsTarget: `IN SCHEMA "MY_DB"."PUBLIC"`,
		}
		assert.Equal(t, `GRANT SELECT ON FUTURE TABLES IN SCHEMA "MY_DB"."PUBLIC" TO ROLE "ANALYST"`, id.FullyQualifiedName())
	})

	t.Run("ShareGrant", func(t *testing.T) {
		t.Parallel()
		id := GrantIdentifier{
			Kind:             GrantKindShare,
			Privilege:        "USAGE",
			OnClause:         `ON DATABASE "MY_DB"`,
			ToClause:         `TO SHARE "my_share"`,
			GranteeName:      "my_share",
			ShowGrantsTarget: `TO SHARE "my_share"`,
		}
		assert.Equal(t, `GRANT USAGE ON DATABASE "MY_DB" TO SHARE "my_share"`, id.FullyQualifiedName())
	})
}
