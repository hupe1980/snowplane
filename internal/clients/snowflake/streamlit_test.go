package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateStreamlitSQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicCreate", func(t *testing.T) {
		t.Parallel()

		opts := CreateStreamlitOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_APP"),
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE STREAMLIT "DB"."SCH"."MY_APP"`)
		assert.NotContains(t, sql, "FROM")
		assert.NotContains(t, sql, "MAIN_FILE")
		assert.NotContains(t, sql, "QUERY_WAREHOUSE")
	})

	t.Run("WithFrom", func(t *testing.T) {
		t.Parallel()

		from := "@my_stage/streamlit"
		opts := CreateStreamlitOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			From: &from,
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `FROM '@my_stage/streamlit'`)
	})

	t.Run("WithMainFile", func(t *testing.T) {
		t.Parallel()

		mainFile := "app.py"
		opts := CreateStreamlitOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			MainFile: &mainFile,
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `MAIN_FILE = 'app.py'`)
	})

	t.Run("WithQueryWarehouse", func(t *testing.T) {
		t.Parallel()

		wh := "MY_WH"
		opts := CreateStreamlitOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			QueryWarehouse: &wh,
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `QUERY_WAREHOUSE = "MY_WH"`)
	})

	t.Run("WithTitle", func(t *testing.T) {
		t.Parallel()

		title := "My Dashboard"
		opts := CreateStreamlitOptions{
			Name:  NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			Title: &title,
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `TITLE = 'My Dashboard'`)
	})

	t.Run("WithExternalAccessIntegrations", func(t *testing.T) {
		t.Parallel()

		opts := CreateStreamlitOptions{
			Name:                       NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			ExternalAccessIntegrations: []string{"INT_A", "INT_B"},
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `EXTERNAL_ACCESS_INTEGRATIONS = ("INT_A", "INT_B")`)
	})

	t.Run("FullOptions", func(t *testing.T) {
		t.Parallel()

		from := "@db.sch.stage/dir"
		mainFile := "streamlit_main.py"
		wh := "COMPUTE_WH"
		comment := "Production dashboard"
		title := "Revenue Dashboard"
		opts := CreateStreamlitOptions{
			Name:                       NewSchemaObjectIdentifier("PROD", "PUBLIC", "REVENUE_APP"),
			From:                       &from,
			MainFile:                   &mainFile,
			QueryWarehouse:             &wh,
			Comment:                    &comment,
			Title:                      &title,
			ExternalAccessIntegrations: []string{"PYPI_INT"},
		}

		sql, err := buildCreateStreamlitSQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE STREAMLIT "PROD"."PUBLIC"."REVENUE_APP"`)
		assert.Contains(t, sql, `FROM '@db.sch.stage/dir'`)
		assert.Contains(t, sql, `MAIN_FILE = 'streamlit_main.py'`)
		assert.Contains(t, sql, `QUERY_WAREHOUSE = "COMPUTE_WH"`)
		assert.Contains(t, sql, `COMMENT = 'Production dashboard'`)
		assert.Contains(t, sql, `TITLE = 'Revenue Dashboard'`)
		assert.Contains(t, sql, `EXTERNAL_ACCESS_INTEGRATIONS = ("PYPI_INT")`)
	})
}

func TestCreateStreamlitOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateStreamlitOptions{
			Name: NewSchemaObjectIdentifier("DB", "SCH", "APP"),
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateStreamlitOptions{}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})
}

func TestBuildAlterStreamlitStatements(t *testing.T) {
	t.Parallel()

	name := NewSchemaObjectIdentifier("DB", "SCH", "MY_APP")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterStreamlitOptions{Name: name}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetMainFile", func(t *testing.T) {
		t.Parallel()

		mf := "new_app.py"
		opts := AlterStreamlitOptions{Name: name, MainFile: &mf}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[0], "MAIN_FILE = 'new_app.py'")
	})

	t.Run("SetQueryWarehouse", func(t *testing.T) {
		t.Parallel()

		wh := "NEW_WH"
		opts := AlterStreamlitOptions{Name: name, QueryWarehouse: &wh}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `QUERY_WAREHOUSE = "NEW_WH"`)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		c := "Updated description"
		opts := AlterStreamlitOptions{Name: name, Comment: &c}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "COMMENT = 'Updated description'")
	})

	t.Run("SetTitle", func(t *testing.T) {
		t.Parallel()

		title := "New Title"
		opts := AlterStreamlitOptions{Name: name, Title: &title}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "TITLE = 'New Title'")
	})

	t.Run("SetExternalAccessIntegrations", func(t *testing.T) {
		t.Parallel()

		eais := []string{"INT_X", "INT_Y"}
		opts := AlterStreamlitOptions{Name: name, ExternalAccessIntegrations: &eais}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `EXTERNAL_ACCESS_INTEGRATIONS = ("INT_X", "INT_Y")`)
	})

	t.Run("MixedOperations", func(t *testing.T) {
		t.Parallel()

		mf := "main.py"
		wh := "WH"
		comment := "new comment"
		title := "New Title"
		opts := AlterStreamlitOptions{
			Name:           name,
			MainFile:       &mf,
			QueryWarehouse: &wh,
			Comment:        &comment,
			Title:          &title,
			UnsetFields:    []string{"TITLE"},
		}
		stmts, err := buildAlterStreamlitStatements(opts)
		require.NoError(t, err)
		// 1 SET statement + 1 UNSET statement
		assert.Len(t, stmts, 2)
		assert.Contains(t, stmts[0], "SET")
		assert.Contains(t, stmts[1], "UNSET TITLE")
		assert.NotContains(t, stmts[1], "NULL")
	})
}

func TestAlterStreamlitOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterStreamlitOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "APP")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithMainFile", func(t *testing.T) {
		t.Parallel()

		mf := "app.py"
		opts := AlterStreamlitOptions{MainFile: &mf}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithQueryWarehouse", func(t *testing.T) {
		t.Parallel()

		wh := "WH"
		opts := AlterStreamlitOptions{QueryWarehouse: &wh}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		c := "test"
		opts := AlterStreamlitOptions{Comment: &c}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithTitle", func(t *testing.T) {
		t.Parallel()

		title := "t"
		opts := AlterStreamlitOptions{Title: &title}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithExternalAccessIntegrations", func(t *testing.T) {
		t.Parallel()

		eais := []string{"INT"}
		opts := AlterStreamlitOptions{ExternalAccessIntegrations: &eais}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterStreamlitOptions{UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterStreamlitOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		mf := "app.py"
		opts := AlterStreamlitOptions{
			Name:     NewSchemaObjectIdentifier("DB", "SCH", "APP"),
			MainFile: &mf,
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := AlterStreamlitOptions{}
		assert.Error(t, opts.Validate())
	})
}
