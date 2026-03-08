package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateGitRepositorySQL(t *testing.T) {
	t.Parallel()

	t.Run("BasicCreate", func(t *testing.T) {
		t.Parallel()

		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"),
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "GIT_API_INT",
		}

		sql, err := buildCreateGitRepositorySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE GIT REPOSITORY "DB"."SCH"."MY_REPO"`)
		assert.Contains(t, sql, `ORIGIN = 'https://github.com/example/repo.git'`)
		assert.Contains(t, sql, `API_INTEGRATION = "GIT_API_INT"`)
		assert.NotContains(t, sql, "GIT_CREDENTIALS")
		assert.NotContains(t, sql, "COMMENT")
	})

	t.Run("WithGitCredentials", func(t *testing.T) {
		t.Parallel()

		creds := "DB.SCH.MY_SECRET"
		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"),
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "GIT_API_INT",
			GitCredentials: &creds,
		}

		sql, err := buildCreateGitRepositorySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `GIT_CREDENTIALS = "DB"."SCH"."MY_SECRET"`)
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		comment := "My Git repository"
		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "REPO"),
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "GIT_INT",
			Comment:        &comment,
		}

		sql, err := buildCreateGitRepositorySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `COMMENT = 'My Git repository'`)
	})

	t.Run("FullOptions", func(t *testing.T) {
		t.Parallel()

		creds := "DB.SCH.GIT_SECRET"
		comment := "Full options repo"
		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("ANALYTICS", "PUBLIC", "SOURCE_REPO"),
			Origin:         "https://github.com/myorg/analytics.git",
			APIIntegration: "GIT_HTTPS_INT",
			GitCredentials: &creds,
			Comment:        &comment,
		}

		sql, err := buildCreateGitRepositorySQL(opts)
		require.NoError(t, err)
		assert.Contains(t, sql, `CREATE GIT REPOSITORY "ANALYTICS"."PUBLIC"."SOURCE_REPO"`)
		assert.Contains(t, sql, `ORIGIN = 'https://github.com/myorg/analytics.git'`)
		assert.Contains(t, sql, `API_INTEGRATION = "GIT_HTTPS_INT"`)
		assert.Contains(t, sql, `GIT_CREDENTIALS = "DB"."SCH"."GIT_SECRET"`)
		assert.Contains(t, sql, `COMMENT = 'Full options repo'`)
	})
}

func TestCreateGitRepositoryOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "REPO"),
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "GIT_INT",
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := CreateGitRepositoryOptions{
			Origin:         "https://github.com/example/repo.git",
			APIIntegration: "GIT_INT",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingOrigin", func(t *testing.T) {
		t.Parallel()

		opts := CreateGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "REPO"),
			APIIntegration: "GIT_INT",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin")
	})

	t.Run("MissingAPIIntegration", func(t *testing.T) {
		t.Parallel()

		opts := CreateGitRepositoryOptions{
			Name:   NewSchemaObjectIdentifier("DB", "SCH", "REPO"),
			Origin: "https://github.com/example/repo.git",
		}
		err := opts.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API integration")
	})
}

func TestBuildAlterGitRepositoryStatements(t *testing.T) {
	t.Parallel()

	name := NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO")

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{Name: name}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		assert.Empty(t, stmts)
	})

	t.Run("SetAPIIntegration", func(t *testing.T) {
		t.Parallel()

		api := "NEW_GIT_INT"
		opts := AlterGitRepositoryOptions{Name: name, APIIntegration: &api}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `SET API_INTEGRATION = "NEW_GIT_INT"`)
	})

	t.Run("SetGitCredentials", func(t *testing.T) {
		t.Parallel()

		creds := "DB.SCH.NEW_SECRET"
		opts := AlterGitRepositoryOptions{Name: name, GitCredentials: &creds}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], `SET GIT_CREDENTIALS = "DB"."SCH"."NEW_SECRET"`)
	})

	t.Run("SetComment", func(t *testing.T) {
		t.Parallel()

		comment := "Updated description"
		opts := AlterGitRepositoryOptions{Name: name, Comment: &comment}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "SET COMMENT = 'Updated description'")
	})

	t.Run("UnsetComment", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{Name: name, UnsetFields: []string{"COMMENT"}}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "UNSET COMMENT")
	})

	t.Run("Fetch", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{Name: name, Fetch: true}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		require.Len(t, stmts, 1)
		assert.Contains(t, stmts[0], "FETCH")
		assert.NotContains(t, stmts[0], "SET")
	})

	t.Run("MixedOperations", func(t *testing.T) {
		t.Parallel()

		api := "NEW_INT"
		comment := "new comment"
		opts := AlterGitRepositoryOptions{
			Name:           name,
			APIIntegration: &api,
			Comment:        &comment,
			UnsetFields:    []string{"GIT_CREDENTIALS"},
			Fetch:          true,
		}
		stmts, err := buildAlterGitRepositoryStatements(opts)
		require.NoError(t, err)
		assert.Len(t, stmts, 4)
	})
}

func TestAlterGitRepositoryOptions_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("NoChanges", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{Name: NewSchemaObjectIdentifier("DB", "SCH", "REPO")}
		assert.False(t, opts.HasChanges())
	})

	t.Run("WithAPIIntegration", func(t *testing.T) {
		t.Parallel()

		api := "INT"
		opts := AlterGitRepositoryOptions{APIIntegration: &api}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithGitCredentials", func(t *testing.T) {
		t.Parallel()

		creds := "DB.SCH.SECRET"
		opts := AlterGitRepositoryOptions{GitCredentials: &creds}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithComment", func(t *testing.T) {
		t.Parallel()

		comment := "test"
		opts := AlterGitRepositoryOptions{Comment: &comment}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithUnsetFields", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{UnsetFields: []string{"COMMENT"}}
		assert.True(t, opts.HasChanges())
	})

	t.Run("WithFetch", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{Fetch: true}
		assert.True(t, opts.HasChanges())
	})
}

func TestAlterGitRepositoryOptions_Validate(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		api := "INT"
		opts := AlterGitRepositoryOptions{
			Name:           NewSchemaObjectIdentifier("DB", "SCH", "REPO"),
			APIIntegration: &api,
		}
		assert.NoError(t, opts.Validate())
	})

	t.Run("MissingName", func(t *testing.T) {
		t.Parallel()

		opts := AlterGitRepositoryOptions{}
		assert.Error(t, opts.Validate())
	})
}
