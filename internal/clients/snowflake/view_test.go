package snowflake

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCreateViewSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     CreateViewOptions
		expected string
	}{
		{
			name: "basic view",
			opts: CreateViewOptions{
				Name:      NewSchemaObjectIdentifier("DB", "S", "V"),
				Statement: "SELECT 1 AS ID",
			},
			expected: `CREATE VIEW IF NOT EXISTS "DB"."S"."V" AS SELECT 1 AS ID`,
		},
		{
			name: "secure view with comment",
			opts: CreateViewOptions{
				Name:      NewSchemaObjectIdentifier("DB", "S", "SECURE_V"),
				Statement: "SELECT * FROM T",
				Secure:    true,
				Comment:   ptrString("my secure view"),
			},
			expected: `CREATE SECURE VIEW IF NOT EXISTS "DB"."S"."SECURE_V" COMMENT = 'my secure view' AS SELECT * FROM T`,
		},
		{
			name: "or replace view",
			opts: CreateViewOptions{
				Name:      NewSchemaObjectIdentifier("DB", "S", "V"),
				Statement: "SELECT 1",
				OrReplace: true,
			},
			expected: `CREATE OR REPLACE VIEW "DB"."S"."V" AS SELECT 1`,
		},
		{
			name: "view with change tracking",
			opts: CreateViewOptions{
				Name:           NewSchemaObjectIdentifier("DB", "S", "V"),
				Statement:      "SELECT 1",
				ChangeTracking: ptrBool(true),
			},
			expected: `CREATE VIEW IF NOT EXISTS "DB"."S"."V" CHANGE_TRACKING = TRUE AS SELECT 1`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildCreateViewSQL(tc.opts)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildAlterViewStatements(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "S", "V")

	tests := []struct {
		name     string
		opts     AlterViewOptions
		expected []string
	}{
		{
			name: "set secure",
			opts: AlterViewOptions{
				Name:   id,
				Secure: ptrBool(true),
			},
			expected: []string{
				`ALTER VIEW "DB"."S"."V" SET SECURE`,
			},
		},
		{
			name: "unset secure",
			opts: AlterViewOptions{
				Name:   id,
				Secure: ptrBool(false),
			},
			expected: []string{
				`ALTER VIEW "DB"."S"."V" UNSET SECURE`,
			},
		},
		{
			name: "set comment",
			opts: AlterViewOptions{
				Name:    id,
				Comment: ptrString("updated"),
			},
			expected: []string{
				`ALTER VIEW "DB"."S"."V" SET COMMENT = 'updated'`,
			},
		},
		{
			name: "unset comment",
			opts: AlterViewOptions{
				Name:        id,
				UnsetFields: []string{"COMMENT"},
			},
			expected: []string{
				`ALTER VIEW "DB"."S"."V" UNSET COMMENT`,
			},
		},
		{
			name: "set change tracking",
			opts: AlterViewOptions{
				Name:           id,
				ChangeTracking: ptrBool(true),
			},
			expected: []string{
				`ALTER VIEW "DB"."S"."V" SET CHANGE_TRACKING = TRUE`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildAlterViewStatements(tc.opts)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestBuildDropViewSQL(t *testing.T) {
	t.Parallel()

	got := buildDropViewSQL(NewSchemaObjectIdentifier("DB", "S", "V"))
	assert.Equal(t, `DROP VIEW IF EXISTS "DB"."S"."V"`, got)
}

func TestBuildShowViewByIDSQL(t *testing.T) {
	t.Parallel()

	got := buildShowViewByIDSQL(NewSchemaObjectIdentifier("MY_DB", "PUBLIC", "MY_VIEW"))
	assert.Equal(t, `SHOW VIEWS LIKE 'MY\_VIEW' IN SCHEMA "MY_DB"."PUBLIC"`, got)
}

func TestCreateViewOptionsValidation(t *testing.T) {
	t.Parallel()

	err := (&CreateViewOptions{
		Name:      NewSchemaObjectIdentifier("DB", "S", "V"),
		Statement: "SELECT 1",
	}).Validate()
	require.NoError(t, err)

	err = (&CreateViewOptions{
		Name: NewSchemaObjectIdentifier("", "", ""),
	}).Validate()
	require.Error(t, err)

	err = (&CreateViewOptions{
		Name: NewSchemaObjectIdentifier("DB", "S", "V"),
	}).Validate()
	require.Error(t, err)
}

func TestAlterViewOptionsHasChanges(t *testing.T) {
	t.Parallel()

	id := NewSchemaObjectIdentifier("DB", "S", "V")
	assert.False(t, (&AlterViewOptions{Name: id}).HasChanges())
	assert.True(t, (&AlterViewOptions{Name: id, Comment: ptrString("x")}).HasChanges())
	assert.True(t, (&AlterViewOptions{Name: id, Secure: ptrBool(true)}).HasChanges())
	assert.True(t, (&AlterViewOptions{Name: id, UnsetFields: []string{"COMMENT"}}).HasChanges())
	assert.True(t, (&AlterViewOptions{
		Name:             id,
		ReplaceStatement: &ReplaceViewStatement{Statement: "SELECT 1"},
	}).HasChanges(), "ReplaceStatement should trigger HasChanges")
}

func TestAlterView_ReplaceStatement(t *testing.T) {
	t.Parallel()

	var captured []string

	mock := &testSQLExec{
		execFn: func(_ context.Context, sql string, _ ...any) error {
			captured = append(captured, sql)
			return nil
		},
	}

	client := NewViewClient(mock)
	err := client.Alter(context.Background(), AlterViewOptions{
		Name: NewSchemaObjectIdentifier("DB", "S", "MY_VIEW"),
		ReplaceStatement: &ReplaceViewStatement{
			Statement:      "SELECT id, name FROM users",
			Secure:         true,
			Comment:        ptrString("replaced"),
			ChangeTracking: ptrBool(true),
		},
	})
	require.NoError(t, err)

	// Should issue a single CREATE OR REPLACE VIEW, NOT ALTER.
	require.Len(t, captured, 1)
	assert.Contains(t, captured[0], "CREATE OR REPLACE")
	assert.Contains(t, captured[0], "SECURE")
	assert.Contains(t, captured[0], "SELECT id, name FROM users")
	assert.Contains(t, captured[0], "COMMENT = 'replaced'")
	assert.NotContains(t, captured[0], "ALTER")
}

// testSQLExec is a minimal SQLExecutor mock for unit tests.
type testSQLExec struct {
	execFn func(ctx context.Context, query string, args ...any) error
}

func (m *testSQLExec) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFn != nil {
		if err := m.execFn(ctx, query, args...); err != nil {
			return nil, err
		}
	}

	return testResult{}, nil
}

func (m *testSQLExec) QueryRow(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}

func (m *testSQLExec) Query(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

// testResult implements sql.Result.
type testResult struct{}

func (testResult) LastInsertId() (int64, error) { return 0, nil }
func (testResult) RowsAffected() (int64, error) { return 0, nil }
