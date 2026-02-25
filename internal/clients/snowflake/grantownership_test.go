package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildGrantOwnershipSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicGrant", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"`,
			ToRole:     "ROLE DATA_ADMIN",
		}
		got := buildGrantOwnershipSQL(opts)
		assert.Equal(t, `GRANT OWNERSHIP ON DATABASE "MY_DB" TO ROLE DATA_ADMIN`, got)
	})

	t.Run("WithCopyGrants", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType:            "TABLE",
			ObjectName:            `"DB"."SCH"."MY_TABLE"`,
			ToRole:                "ROLE ADMIN",
			CurrentGrantsBehavior: "COPY",
		}
		got := buildGrantOwnershipSQL(opts)
		assert.Equal(t, `GRANT OWNERSHIP ON TABLE "DB"."SCH"."MY_TABLE" TO ROLE ADMIN COPY CURRENT GRANTS`, got)
	})

	t.Run("WithRevokeGrants", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType:            "SCHEMA",
			ObjectName:            `"DB"."MY_SCHEMA"`,
			ToRole:                "ROLE SCH_ADMIN",
			CurrentGrantsBehavior: "REVOKE",
		}
		got := buildGrantOwnershipSQL(opts)
		assert.Equal(t, `GRANT OWNERSHIP ON SCHEMA "DB"."MY_SCHEMA" TO ROLE SCH_ADMIN REVOKE CURRENT GRANTS`, got)
	})

	t.Run("DatabaseRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "TABLE",
			ObjectName: `"DB"."SCH"."T"`,
			ToRole:     "DATABASE ROLE DB.MY_DB_ROLE",
		}
		got := buildGrantOwnershipSQL(opts)
		assert.Contains(t, got, "TO DATABASE ROLE DB.MY_DB_ROLE")
	})

	t.Run("NoBehavior", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "VIEW",
			ObjectName: `"DB"."SCH"."V"`,
			ToRole:     "ROLE VIEWER",
		}
		got := buildGrantOwnershipSQL(opts)
		assert.NotContains(t, got, "CURRENT GRANTS")
	})
}

func TestGrantOwnershipIdentifier(t *testing.T) {
	t.Parallel()

	t.Run("FullyQualifiedName", func(t *testing.T) {
		t.Parallel()
		id := GrantOwnershipIdentifier{
			ObjectType:  "DATABASE",
			ObjectName:  `"MY_DB"`,
			GranteeName: "DATA_ADMIN",
		}
		assert.Equal(t, `OWNERSHIP ON DATABASE "MY_DB" -> DATA_ADMIN`, id.FullyQualifiedName())
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		id := GrantOwnershipIdentifier{
			ObjectType:  "TABLE",
			ObjectName:  `"DB"."SCH"."T"`,
			GranteeName: "ADMIN",
		}
		assert.Equal(t, id.FullyQualifiedName(), id.String())
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestCreateGrantOwnershipOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"`,
			ToRole:     "ROLE ADMIN",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingObjectType", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectName: `"MY_DB"`,
			ToRole:     "ROLE ADMIN",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingObjectName", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ToRole:     "ROLE ADMIN",
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("MissingToRole", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"`,
		}
		assert.Error(t, opts.Validate())
	})

	t.Run("InvalidObjectType", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "TABLE; DROP DATABASE X--",
			ObjectName: `"MY_DB"`,
			ToRole:     "ROLE ADMIN",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid object type")
	})

	t.Run("ObjectNameWithSemicolon", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"; DROP TABLE X--`,
			ToRole:     "ROLE ADMIN",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid object name")
	})

	t.Run("ObjectNameWithDollarQuoting", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType: "DATABASE",
			ObjectName: `"MY_DB"$$injection$$`,
			ToRole:     "ROLE ADMIN",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid object name")
	})

	t.Run("InvalidCurrentGrantsBehavior", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType:            "TABLE",
			ObjectName:            `"DB"."SCH"."T"`,
			ToRole:                "ROLE ADMIN",
			CurrentGrantsBehavior: "COPY; DROP",
		}
		err := opts.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid current grants behavior")
	})

	t.Run("ValidWithCopyBehavior", func(t *testing.T) {
		t.Parallel()
		opts := CreateGrantOwnershipOptions{
			ObjectType:            "TABLE",
			ObjectName:            `"DB"."SCH"."T"`,
			ToRole:                "ROLE ADMIN",
			CurrentGrantsBehavior: "COPY",
		}
		assert.NoError(t, opts.Validate())
	})
}
