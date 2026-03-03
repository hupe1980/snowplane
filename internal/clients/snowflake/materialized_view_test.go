package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateMaterializedViewSQL(t *testing.T) {
	t.Parallel()

	t.Run("Basic", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1, col2 FROM my_table",
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Equal(t, `CREATE MATERIALIZED VIEW IF NOT EXISTS "DB"."SCH"."MV1" AS SELECT col1, col2 FROM my_table`, sql)
	})

	t.Run("WithOrReplace", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1 FROM my_table",
			OrReplace: true,
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "CREATE OR REPLACE")
		assert.NotContains(t, sql, "IF NOT EXISTS")
	})

	t.Run("WithSecure", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1 FROM my_table",
			Secure:    true,
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "SECURE MATERIALIZED VIEW")
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		comment := "test comment"
		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1 FROM my_table",
			Comment:   &comment,
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "COMMENT = 'test comment'")
	})

	t.Run("WithClusterBy", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1, col2 FROM my_table",
			ClusterBy: []string{"col1", "col2"},
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "CLUSTER BY (col1, col2)")
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		t.Parallel()

		comment := "full options"
		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1, col2 FROM my_table",
			Secure:    true,
			Comment:   &comment,
			ClusterBy: []string{"col1"},
			OrReplace: true,
		}
		sql, err := buildCreateMaterializedViewSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, "CREATE OR REPLACE SECURE MATERIALIZED VIEW")
		assert.Contains(t, sql, "COMMENT = 'full options'")
		assert.Contains(t, sql, "CLUSTER BY (col1)")
		assert.Contains(t, sql, "AS SELECT col1, col2 FROM my_table")
	})
}

func TestCreateMaterializedViewOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name:      NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Statement: "SELECT col1 FROM my_table",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Statement: "SELECT col1 FROM my_table",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingStatement", func(t *testing.T) {
		t.Parallel()

		opts := CreateMaterializedViewOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "statement")
	})
}

func TestBuildAlterMaterializedViewStatements(t *testing.T) {
	t.Parallel()

	t.Run("SetSecureTrue", func(t *testing.T) {
		t.Parallel()

		secure := true
		opts := AlterMaterializedViewOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Secure: &secure,
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER MATERIALIZED VIEW "DB"."SCH"."MV1" SET SECURE`, stmts[0])
	})

	t.Run("UnsetSecure", func(t *testing.T) {
		t.Parallel()

		secure := false
		opts := AlterMaterializedViewOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Secure: &secure,
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Equal(t, `ALTER MATERIALIZED VIEW "DB"."SCH"."MV1" UNSET SECURE`, stmts[0])
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		comment := "new comment"
		opts := AlterMaterializedViewOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Comment: &comment,
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'new comment'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			UnsetFields: []string{"COMMENT"},
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SecureAndComment", func(t *testing.T) {
		t.Parallel()

		secure := true
		comment := "secured"
		opts := AlterMaterializedViewOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Secure:  &secure,
			Comment: &comment,
		}
		stmts, err := buildAlterMaterializedViewStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET SECURE")
		assert.Contains(t, stmts[1], "SET COMMENT = 'secured'")
	})
}

func TestAlterMaterializedViewOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

func TestAlterMaterializedViewOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
		}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithSecure", func(t *testing.T) {
		t.Parallel()

		secure := true
		opts := AlterMaterializedViewOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Secure: &secure,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		comment := "test"
		opts := AlterMaterializedViewOptions{
			Name:    NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			Comment: &comment,
		}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterMaterializedViewOptions{
			Name:        NewSchemaObjectIdentifier("DB", "SCH", "MV1"),
			UnsetFields: []string{"COMMENT"},
		}
		assert.True(t, opts.HasChanges())
	})
}

func TestBuildDropMaterializedViewSQL(t *testing.T) {
	t.Parallel()

	sql := buildDropMaterializedViewSQL(NewSchemaObjectIdentifier("DB", "SCH", "MV1"))
	assert.Equal(t, `DROP MATERIALIZED VIEW IF EXISTS "DB"."SCH"."MV1"`, sql)
}

func TestBuildShowMaterializedViewByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowMaterializedViewByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MV1"))
	assert.Contains(t, sql, "SHOW MATERIALIZED VIEWS LIKE")
	assert.Contains(t, sql, `IN SCHEMA "DB"."SCH"`)
}
