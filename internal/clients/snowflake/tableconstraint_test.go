package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// SQL generation tests
// --------------------------------------------------------------------------

func TestBuildAddConstraintSQL(t *testing.T) {
	t.Parallel()

	t.Run("PrimaryKey", func(t *testing.T) {
		t.Parallel()
		opts := AddConstraintOptions{
			ConstraintName: "pk_orders",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"MY_DB"."MY_SCHEMA"."ORDERS"`,
			Columns:        []string{"ORDER_ID"},
		}
		got := buildAddConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "MY_DB"."MY_SCHEMA"."ORDERS" ADD CONSTRAINT "pk_orders" PRIMARY KEY ("ORDER_ID")`, got)
	})

	t.Run("UniqueMultiColumn", func(t *testing.T) {
		t.Parallel()
		opts := AddConstraintOptions{
			ConstraintName: "uq_email_tenant",
			ConstraintType: "UNIQUE",
			TableName:      `"DB"."S"."USERS"`,
			Columns:        []string{"EMAIL", "TENANT_ID"},
		}
		got := buildAddConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."USERS" ADD CONSTRAINT "uq_email_tenant" UNIQUE ("EMAIL", "TENANT_ID")`, got)
	})

	t.Run("ForeignKey_Basic", func(t *testing.T) {
		t.Parallel()
		opts := AddConstraintOptions{
			ConstraintName:      "fk_order_customer",
			ConstraintType:      "FOREIGN KEY",
			TableName:           `"DB"."S"."ORDERS"`,
			Columns:             []string{"CUSTOMER_ID"},
			ReferencesTableName: `"DB"."S"."CUSTOMERS"`,
			ReferencesColumns:   []string{"ID"},
		}
		got := buildAddConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."ORDERS" ADD CONSTRAINT "fk_order_customer" FOREIGN KEY ("CUSTOMER_ID") REFERENCES "DB"."S"."CUSTOMERS" ("ID")`, got)
	})

	t.Run("ForeignKey_WithActions", func(t *testing.T) {
		t.Parallel()
		match := "FULL"
		onUpdate := "CASCADE"
		onDelete := "SET NULL"
		opts := AddConstraintOptions{
			ConstraintName:      "fk_order_customer",
			ConstraintType:      "FOREIGN KEY",
			TableName:           `"DB"."S"."ORDERS"`,
			Columns:             []string{"CUSTOMER_ID"},
			ReferencesTableName: `"DB"."S"."CUSTOMERS"`,
			ReferencesColumns:   []string{"ID"},
			Match:               &match,
			OnUpdate:            &onUpdate,
			OnDelete:            &onDelete,
		}
		got := buildAddConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."ORDERS" ADD CONSTRAINT "fk_order_customer" FOREIGN KEY ("CUSTOMER_ID") REFERENCES "DB"."S"."CUSTOMERS" ("ID") MATCH FULL ON UPDATE CASCADE ON DELETE SET NULL`, got)
	})

	t.Run("WithProperties", func(t *testing.T) {
		t.Parallel()
		enforced := false
		deferrable := true
		initially := "DEFERRED"
		rely := true
		validate := false
		opts := AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
			Enforced:       &enforced,
			Deferrable:     &deferrable,
			Initially:      &initially,
			Rely:           &rely,
			ShouldValidate: &validate,
		}
		got := buildAddConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" ADD CONSTRAINT "pk_test" PRIMARY KEY ("ID") NOT ENFORCED DEFERRABLE INITIALLY DEFERRED RELY NOVALIDATE`, got)
	})

	t.Run("Enforced_True", func(t *testing.T) {
		t.Parallel()
		enforced := true
		opts := AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
			Enforced:       &enforced,
		}
		got := buildAddConstraintSQL(opts)
		assert.Contains(t, got, "ENFORCED")
		assert.NotContains(t, got, "NOT ENFORCED")
	})

	t.Run("NotDeferrable", func(t *testing.T) {
		t.Parallel()
		deferrable := false
		opts := AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
			Deferrable:     &deferrable,
		}
		got := buildAddConstraintSQL(opts)
		assert.Contains(t, got, "NOT DEFERRABLE")
	})
}

func TestBuildAlterConstraintSQL(t *testing.T) {
	t.Parallel()

	t.Run("Enforced", func(t *testing.T) {
		t.Parallel()
		enforced := true
		opts := AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Enforced:       &enforced,
		}
		got := buildAlterConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" ALTER CONSTRAINT "pk_test" ENFORCED`, got)
	})

	t.Run("NotEnforced_Rely_NoValidate", func(t *testing.T) {
		t.Parallel()
		enforced := false
		rely := true
		validate := false
		opts := AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Enforced:       &enforced,
			Rely:           &rely,
			ShouldValidate: &validate,
		}
		got := buildAlterConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" ALTER CONSTRAINT "pk_test" NOT ENFORCED RELY NOVALIDATE`, got)
	})

	t.Run("NoRelyValidate", func(t *testing.T) {
		t.Parallel()
		rely := false
		validate := true
		opts := AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Rely:           &rely,
			ShouldValidate: &validate,
		}
		got := buildAlterConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" ALTER CONSTRAINT "pk_test" NORELY VALIDATE`, got)
	})
}

func TestBuildDropConstraintSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		opts := DropConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
		}
		got := buildDropConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" DROP CONSTRAINT "pk_test"`, got)
	})

	t.Run("Cascade", func(t *testing.T) {
		t.Parallel()
		opts := DropConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Cascade:        true,
		}
		got := buildDropConstraintSQL(opts)
		assert.Equal(t, `ALTER TABLE "DB"."S"."T" DROP CONSTRAINT "pk_test" CASCADE`, got)
	})
}

func TestBuildCommentOnConstraintSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		got := buildCommentOnConstraintSQL(`"DB"."S"."T"`, "pk_test", "Primary key")
		assert.Equal(t, `COMMENT ON CONSTRAINT "pk_test" ON "DB"."S"."T" IS 'Primary key'`, got)
	})

	t.Run("EscapesSingleQuotes", func(t *testing.T) {
		t.Parallel()
		got := buildCommentOnConstraintSQL(`"DB"."S"."T"`, "pk_test", "It's a comment")
		assert.Contains(t, got, "It''s a comment")
	})
}

func TestBuildShowConstraintSQL(t *testing.T) {
	t.Parallel()

	t.Run("PrimaryKey", func(t *testing.T) {
		t.Parallel()
		got := buildShowConstraintSQL(`"DB"."S"."T"`, "PRIMARY KEY")
		assert.Equal(t, `SHOW PRIMARY KEYS IN TABLE "DB"."S"."T"`, got)
	})

	t.Run("Unique", func(t *testing.T) {
		t.Parallel()
		got := buildShowConstraintSQL(`"DB"."S"."T"`, "UNIQUE")
		assert.Equal(t, `SHOW UNIQUE KEYS IN TABLE "DB"."S"."T"`, got)
	})

	t.Run("ForeignKey", func(t *testing.T) {
		t.Parallel()
		got := buildShowConstraintSQL(`"DB"."S"."T"`, "FOREIGN KEY")
		assert.Equal(t, `SHOW IMPORTED KEYS IN TABLE "DB"."S"."T"`, got)
	})

	t.Run("DefaultFallback", func(t *testing.T) {
		t.Parallel()
		got := buildShowConstraintSQL(`"DB"."S"."T"`, "UNKNOWN")
		assert.Equal(t, `SHOW PRIMARY KEYS IN TABLE "DB"."S"."T"`, got)
	})
}

func TestTableConstraintIdentifier_FullyQualifiedName(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()
		id := TableConstraintIdentifier{
			ConstraintName: "pk_orders",
			TableName:      `"MY_DB"."MY_SCHEMA"."ORDERS"`,
		}
		assert.Equal(t, `CONSTRAINT "pk_orders" ON "MY_DB"."MY_SCHEMA"."ORDERS"`, id.FullyQualifiedName())
	})
}

// --------------------------------------------------------------------------
// Validation tests
// --------------------------------------------------------------------------

func TestAddConstraintOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid_PrimaryKey", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("Valid_ForeignKey", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName:      "fk_test",
			ConstraintType:      "FOREIGN KEY",
			TableName:           `"DB"."S"."T"`,
			Columns:             []string{"ID"},
			ReferencesTableName: `"DB"."S"."R"`,
			ReferencesColumns:   []string{"ID"},
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingTableName", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			Columns:        []string{"ID"},
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingType", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Columns:        []string{"ID"},
		}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingColumns", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName: "pk_test",
			ConstraintType: "PRIMARY KEY",
			TableName:      `"DB"."S"."T"`,
		}
		require.Error(t, opts.Validate())
	})

	t.Run("FK_MissingReferencesTable", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName:    "fk_test",
			ConstraintType:    "FOREIGN KEY",
			TableName:         `"DB"."S"."T"`,
			Columns:           []string{"ID"},
			ReferencesColumns: []string{"ID"},
		}
		require.Error(t, opts.Validate())
	})

	t.Run("FK_MissingReferencesColumns", func(t *testing.T) {
		t.Parallel()
		opts := &AddConstraintOptions{
			ConstraintName:      "fk_test",
			ConstraintType:      "FOREIGN KEY",
			TableName:           `"DB"."S"."T"`,
			Columns:             []string{"ID"},
			ReferencesTableName: `"DB"."S"."R"`,
		}
		require.Error(t, opts.Validate())
	})
}

func TestAlterConstraintOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		opts := &AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
		}
		require.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()
		opts := &AlterConstraintOptions{TableName: `"DB"."S"."T"`}
		require.Error(t, opts.Validate())
	})

	t.Run("MissingTableName", func(t *testing.T) {
		t.Parallel()
		opts := &AlterConstraintOptions{ConstraintName: "pk_test"}
		require.Error(t, opts.Validate())
	})
}

func TestAlterConstraintOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()
		opts := &AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("HasEnforced", func(t *testing.T) {
		t.Parallel()
		enforced := true
		opts := &AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Enforced:       &enforced,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("HasComment", func(t *testing.T) {
		t.Parallel()
		comment := "test"
		opts := &AlterConstraintOptions{
			ConstraintName: "pk_test",
			TableName:      `"DB"."S"."T"`,
			Comment:        &comment,
		}
		assert.True(t, opts.HasChanges())
	})
}
