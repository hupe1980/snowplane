package sqlstatement

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStatementDenylist_Single(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"DROP DATABASE"})
	require.NoError(t, err)
	assert.Equal(t, 1, dl.Len())
	assert.False(t, dl.IsEmpty())
}

func TestNewStatementDenylist_Multiple(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"DROP DATABASE", "ALTER USER", "TRUNCATE TABLE"})
	require.NoError(t, err)
	assert.Equal(t, 3, dl.Len())
}

func TestNewStatementDenylist_SkipsEmpty(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"DROP DATABASE", "", "  "})
	require.NoError(t, err)
	assert.Equal(t, 1, dl.Len())
}

func TestNewStatementDenylist_Empty(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist(nil)
	require.NoError(t, err)
	assert.True(t, dl.IsEmpty())
}

func TestParseStatementDenylist_CSV(t *testing.T) {
	t.Parallel()

	dl, err := ParseStatementDenylist("DROP DATABASE, ALTER USER, TRUNCATE TABLE")
	require.NoError(t, err)
	assert.Equal(t, 3, dl.Len())
}

func TestParseStatementDenylist_Empty(t *testing.T) {
	t.Parallel()

	dl, err := ParseStatementDenylist("")
	require.NoError(t, err)
	assert.True(t, dl.IsEmpty())
}

func TestStatementDenylist_Check_Blocked(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"DROP DATABASE", "ALTER USER"})
	require.NoError(t, err)

	tests := []struct {
		name string
		sql  string
	}{
		{"exact DROP DATABASE", "DROP DATABASE mydb"},
		{"lowercase drop database", "drop database mydb"},
		{"mixed case", "Drop Database mydb"},
		{"with IF EXISTS", "DROP DATABASE IF EXISTS mydb"},
		{"ALTER USER", "ALTER USER admin SET DEFAULT_ROLE = 'PUBLIC'"},
		{"alter user lower", "alter user admin set default_role = 'PUBLIC'"},
		{"multi-line", "DROP\n  DATABASE\n  mydb"},
		{"tab-separated", "DROP\tDATABASE\tmydb"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := dl.Check(tc.sql)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "statement denied")
		})
	}
}

func TestStatementDenylist_Check_Allowed(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"DROP DATABASE", "ALTER USER"})
	require.NoError(t, err)

	tests := []struct {
		name string
		sql  string
	}{
		{"CREATE TABLE", "CREATE TABLE test (id INT)"},
		{"DROP TABLE allowed", "DROP TABLE test"},
		{"CREATE USER allowed", "CREATE USER newuser"},
		{"SELECT", "SELECT 1"},
		{"DROP SCHEMA allowed", "DROP SCHEMA public"},
		{"GRANT", "GRANT SELECT ON TABLE test TO ROLE analyst"},
		{"ALTER TABLE allowed", "ALTER TABLE test ADD COLUMN name VARCHAR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := dl.Check(tc.sql)
			assert.NoError(t, err)
		})
	}
}

func TestStatementDenylist_Check_NilSafe(t *testing.T) {
	t.Parallel()

	var dl *StatementDenylist
	assert.NoError(t, dl.Check("DROP DATABASE mydb"))
}

func TestStatementDenylist_Check_EmptyAllowsAll(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist(nil)
	require.NoError(t, err)
	assert.NoError(t, dl.Check("DROP DATABASE mydb"))
}

func TestStatementDenylist_SingleKeyword(t *testing.T) {
	t.Parallel()

	dl, err := NewStatementDenylist([]string{"TRUNCATE"})
	require.NoError(t, err)

	assert.Error(t, dl.Check("TRUNCATE TABLE mydb.public.events"))
	assert.Error(t, dl.Check("truncate table mydb.public.events"))
	assert.NoError(t, dl.Check("SELECT * FROM information_schema.tables"))
}

func TestStatementDenylist_NoFalsePositive_Substring(t *testing.T) {
	t.Parallel()

	// "DROP DATABASE" should not match "BACKDROP" or "DATABASE_NAME"
	dl, err := NewStatementDenylist([]string{"DROP DATABASE"})
	require.NoError(t, err)
	assert.NoError(t, dl.Check("SELECT 'BACKDROP DATABASE' FROM t"))
}

func TestParseStatementDenylist_DefaultRecommended(t *testing.T) {
	t.Parallel()

	// The recommended default denylist from the FINDINGS.md
	dl, err := ParseStatementDenylist("DROP DATABASE, DROP SCHEMA, ALTER USER")
	require.NoError(t, err)
	assert.Equal(t, 3, dl.Len())

	// Should block dangerous operations
	assert.Error(t, dl.Check("DROP DATABASE production"))
	assert.Error(t, dl.Check("DROP SCHEMA public"))
	assert.Error(t, dl.Check("ALTER USER admin SET DEFAULT_ROLE = 'SYSADMIN'"))

	// Should allow normal operations
	assert.NoError(t, dl.Check("CREATE TABLE test (id INT)"))
	assert.NoError(t, dl.Check("DROP TABLE temp_staging"))
	assert.NoError(t, dl.Check("ALTER TABLE test ADD COLUMN name VARCHAR"))
	assert.NoError(t, dl.Check("CREATE SCHEMA analytics"))
	assert.NoError(t, dl.Check("CREATE DATABASE sandbox"))
}
