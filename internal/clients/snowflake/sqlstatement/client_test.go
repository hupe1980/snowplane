package sqlstatement

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// mockExecutor implements snowflake.SQLExecutor for testing.
type mockExecutor struct {
	execFn  func(ctx context.Context, query string) (sql.Result, error)
	queryFn func(ctx context.Context, query string) (*sql.Rows, error)
}

func (m *mockExecutor) Exec(ctx context.Context, query string, _ ...any) (sql.Result, error) {
	if m.execFn != nil {
		return m.execFn(ctx, query)
	}

	return nil, nil
}

func (m *mockExecutor) Query(ctx context.Context, query string, _ ...any) (*sql.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, query)
	}

	return nil, nil
}

func (m *mockExecutor) QueryRow(_ context.Context, _ string, _ ...any) *snowflake.Row {
	return snowflake.NewErrorRow(nil)
}

// --- splitStatements ---

func TestSplitStatements_Single(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("CREATE TABLE t1 (id INT)")
	assert.Equal(t, []string{"CREATE TABLE t1 (id INT)"}, stmts)
}

func TestSplitStatements_Multiple(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("CREATE TABLE t1 (id INT); INSERT INTO t1 VALUES (1)")
	assert.Equal(t, []string{"CREATE TABLE t1 (id INT)", "INSERT INTO t1 VALUES (1)"}, stmts)
}

func TestSplitStatements_TrailingSemicolon(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("SELECT 1;")
	assert.Equal(t, []string{"SELECT 1"}, stmts)
}

func TestSplitStatements_EmptySegments(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("SELECT 1; ; ; SELECT 2")
	assert.Equal(t, []string{"SELECT 1", "SELECT 2"}, stmts)
}

func TestSplitStatements_Whitespace(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("  SELECT 1  ;  SELECT 2  ")
	assert.Equal(t, []string{"SELECT 1", "SELECT 2"}, stmts)
}

func TestSplitStatements_QuotedSemicolon(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("INSERT INTO t VALUES ('hello; world')")
	assert.Equal(t, []string{"INSERT INTO t VALUES ('hello; world')"}, stmts)
}

func TestSplitStatements_EscapedQuote(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("INSERT INTO t VALUES ('it''s a test'); SELECT 1")
	assert.Equal(t, []string{"INSERT INTO t VALUES ('it''s a test')", "SELECT 1"}, stmts)
}

func TestSplitStatements_MultipleQuotedSegments(t *testing.T) {
	t.Parallel()

	stmts := splitStatements("SELECT 'a;b'; SELECT 'c;d'")
	assert.Equal(t, []string{"SELECT 'a;b'", "SELECT 'c;d'"}, stmts)
}

// --- matchesAllExpectations ---

func TestMatchesAllExpectations_Empty(t *testing.T) {
	t.Parallel()

	row := map[string]string{"NAME": "test"}
	assert.True(t, matchesAllExpectations(row, nil))
}

func TestMatchesAllExpectations_SingleMatch(t *testing.T) {
	t.Parallel()

	row := map[string]string{"NAME": "test", "STATUS": "active"}
	exps := []Expectation{{Column: "name", Value: "test"}}
	assert.True(t, matchesAllExpectations(row, exps))
}

func TestMatchesAllExpectations_CaseInsensitive(t *testing.T) {
	t.Parallel()

	row := map[string]string{"STATUS": "ACTIVE"}
	exps := []Expectation{{Column: "status", Value: "active"}}
	assert.True(t, matchesAllExpectations(row, exps))
}

func TestMatchesAllExpectations_Mismatch(t *testing.T) {
	t.Parallel()

	row := map[string]string{"STATUS": "inactive"}
	exps := []Expectation{{Column: "status", Value: "active"}}
	assert.False(t, matchesAllExpectations(row, exps))
}

func TestMatchesAllExpectations_MissingColumn(t *testing.T) {
	t.Parallel()

	row := map[string]string{"NAME": "test"}
	exps := []Expectation{{Column: "status", Value: "active"}}
	assert.False(t, matchesAllExpectations(row, exps))
}

func TestMatchesAllExpectations_MultipleExpectations(t *testing.T) {
	t.Parallel()

	row := map[string]string{"NAME": "test", "STATUS": "active"}
	exps := []Expectation{
		{Column: "name", Value: "test"},
		{Column: "status", Value: "active"},
	}
	assert.True(t, matchesAllExpectations(row, exps))
}

func TestMatchesAllExpectations_PartialMatch(t *testing.T) {
	t.Parallel()

	row := map[string]string{"NAME": "test", "STATUS": "inactive"}
	exps := []Expectation{
		{Column: "name", Value: "test"},
		{Column: "status", Value: "active"},
	}
	assert.False(t, matchesAllExpectations(row, exps))
}

// --- HashSQL ---

func TestHashSQL_Deterministic(t *testing.T) {
	t.Parallel()

	h1 := HashSQL("SELECT 1")
	h2 := HashSQL("SELECT 1")
	assert.Equal(t, h1, h2)
}

func TestHashSQL_DifferentInputs(t *testing.T) {
	t.Parallel()

	h1 := HashSQL("SELECT 1")
	h2 := HashSQL("SELECT 2")
	assert.NotEqual(t, h1, h2)
}

func TestHashSQL_HexEncoded(t *testing.T) {
	t.Parallel()

	h := HashSQL("test")
	assert.Len(t, h, 64) // SHA-256 => 32 bytes => 64 hex chars
}

// --- Execute ---

func TestExecute_SingleStatement(t *testing.T) {
	t.Parallel()

	var execCalls []string

	mock := &mockExecutor{
		execFn: func(_ context.Context, query string) (sql.Result, error) {
			execCalls = append(execCalls, query)
			return nil, nil
		},
	}

	c := NewClient(mock)
	err := c.Execute(context.Background(), "CREATE TABLE t1 (id INT)")
	require.NoError(t, err)
	assert.Equal(t, []string{"CREATE TABLE t1 (id INT)"}, execCalls)
}

func TestExecute_MultiStatement(t *testing.T) {
	t.Parallel()

	var execCalls []string

	mock := &mockExecutor{
		execFn: func(_ context.Context, query string) (sql.Result, error) {
			execCalls = append(execCalls, query)
			return nil, nil
		},
	}

	c := NewClient(mock)
	err := c.Execute(context.Background(), "CREATE TABLE t1 (id INT); INSERT INTO t1 VALUES (1)")
	require.NoError(t, err)
	assert.Len(t, execCalls, 2)
	assert.Equal(t, "CREATE TABLE t1 (id INT)", execCalls[0])
	assert.Equal(t, "INSERT INTO t1 VALUES (1)", execCalls[1])
}

func TestExecute_Error(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		execFn: func(_ context.Context, _ string) (sql.Result, error) {
			return nil, assert.AnError
		},
	}

	c := NewClient(mock)
	err := c.Execute(context.Background(), "BAD SQL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executing SQL statement")
}

// --- Revert ---

func TestRevert_SingleStatement(t *testing.T) {
	t.Parallel()

	var execCalls []string

	mock := &mockExecutor{
		execFn: func(_ context.Context, query string) (sql.Result, error) {
			execCalls = append(execCalls, query)
			return nil, nil
		},
	}

	c := NewClient(mock)
	err := c.Revert(context.Background(), "DROP TABLE t1")
	require.NoError(t, err)
	assert.Equal(t, []string{"DROP TABLE t1"}, execCalls)
}

func TestRevert_Error(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		execFn: func(_ context.Context, _ string) (sql.Result, error) {
			return nil, assert.AnError
		},
	}

	c := NewClient(mock)
	err := c.Revert(context.Background(), "DROP TABLE t1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reverting SQL statement")
}

// --- Observe ---

func TestObserve_EmptySQL(t *testing.T) {
	t.Parallel()

	c := NewClient(&mockExecutor{})

	obs, err := c.Observe(context.Background(), "", nil)
	require.NoError(t, err)
	assert.False(t, obs.Exists)
	assert.Equal(t, int32(0), obs.RowCount)
}

func TestObserve_QueryError(t *testing.T) {
	t.Parallel()

	mock := &mockExecutor{
		queryFn: func(_ context.Context, _ string) (*sql.Rows, error) {
			return nil, assert.AnError
		},
	}

	c := NewClient(mock)
	_, err := c.Observe(context.Background(), "SELECT 1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "observe query")
}
