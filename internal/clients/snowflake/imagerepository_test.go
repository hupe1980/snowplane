package snowflake

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------
// Tests: CreateImageRepositoryOptions.Validate
// --------------------------------------------------------------------------

func TestCreateImageRepositoryOptions_Validate_Valid(t *testing.T) {
	t.Parallel()

	opts := CreateImageRepositoryOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"),
	}
	assert.NoError(t, opts.Validate())
}

func TestCreateImageRepositoryOptions_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	opts := CreateImageRepositoryOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	}
	assert.Error(t, opts.Validate())
}

// --------------------------------------------------------------------------
// Tests: buildShowImageRepositoryByIDSQL
// --------------------------------------------------------------------------

func TestBuildShowImageRepositoryByIDSQL(t *testing.T) {
	t.Parallel()

	sql := buildShowImageRepositoryByIDSQL(NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"))
	assert.Contains(t, sql, "SHOW IMAGE REPOSITORIES")
	assert.Contains(t, sql, "LIKE")
}

// --------------------------------------------------------------------------
// Tests: ImageRepositoryClient (validation only, no DB)
// --------------------------------------------------------------------------

func TestImageRepositoryClient_Create_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewImageRepositoryClient(nil)
	err := client.Create(t.Context(), CreateImageRepositoryOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", ""),
	})
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestImageRepositoryClient_Drop_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewImageRepositoryClient(nil)
	err := client.Drop(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

func TestImageRepositoryClient_ShowByID_InvalidName(t *testing.T) {
	t.Parallel()

	client := NewImageRepositoryClient(nil)
	_, err := client.ShowByID(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", ""))
	require.Error(t, err)
	assert.True(t, IsTerminalError(err))
}

// --------------------------------------------------------------------------
// Tests: Create SQL generation
// --------------------------------------------------------------------------

func TestImageRepositoryClient_Create_SQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	client := NewImageRepositoryClient(mock)
	err := client.Create(t.Context(), CreateImageRepositoryOptions{
		Name: NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"),
	})
	require.NoError(t, err)
	assert.Equal(t, `CREATE IMAGE REPOSITORY "DB"."SCH"."MY_REPO"`, captured)
}

// --------------------------------------------------------------------------
// Tests: Drop SQL generation
// --------------------------------------------------------------------------

func TestImageRepositoryClient_Drop_SQL(t *testing.T) {
	t.Parallel()

	var captured string
	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = sql
			return nil
		},
	}

	client := NewImageRepositoryClient(mock)
	err := client.Drop(t.Context(), NewSchemaObjectIdentifier("DB", "SCH", "MY_REPO"))
	require.NoError(t, err)
	assert.Contains(t, captured, "DROP IMAGE REPOSITORY IF EXISTS")
	assert.Contains(t, captured, `"DB"."SCH"."MY_REPO"`)
}
