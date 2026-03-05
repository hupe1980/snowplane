// Package sqlstatement provides a Snowflake client for SQLStatement resources.
// Unlike other Snowflake clients that map to specific DDL objects, this client executes
// user-provided SQL verbatim. The observe/execute/revert SQL strings come
// directly from the CRD spec.
package sqlstatement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// Observation carries the result of running the observe SQL.
type Observation struct {
	// Exists is true when the observe query returned rows matching all expectations.
	Exists bool

	// RowCount is the number of rows returned by the observe query.
	RowCount int32

	// Matched indicates whether all expectations were satisfied.
	Matched bool
}

// Expectation mirrors the CRD's SQLStatementExpectation type.
type Expectation struct {
	Column string
	Value  string
}

// Client wraps a SQLExecutor to provide execute/revert/observe operations
// for SQLStatement resources.
type Client struct {
	exec snowflake.SQLExecutor
}

// NewClient creates a new SQLStatement client.
func NewClient(exec snowflake.SQLExecutor) *Client {
	return &Client{exec: exec}
}

// Execute runs the execute SQL. For multi-statement SQL (semicolon-separated),
// each statement is executed individually.
func (c *Client) Execute(ctx context.Context, sql string) error {
	stmts := splitStatements(sql)

	for _, stmt := range stmts {
		if _, err := c.exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("executing SQL statement: %w", err)
		}
	}

	return nil
}

// Revert runs the revert SQL.
func (c *Client) Revert(ctx context.Context, revertSQL string) error {
	stmts := splitStatements(revertSQL)

	for _, stmt := range stmts {
		if _, err := c.exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("reverting SQL statement: %w", err)
		}
	}

	return nil
}

// Observe runs the observe SQL and checks expectations against the result set.
// When observeSQL is empty, returns a non-existing observation.
func (c *Client) Observe(ctx context.Context, observeSQL string, expectations []Expectation) (*Observation, error) {
	if observeSQL == "" {
		return &Observation{Exists: false}, nil
	}

	rows, err := c.exec.Query(ctx, observeSQL)
	if err != nil {
		return nil, fmt.Errorf("observe query: %w", err)
	}

	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading observe query columns: %w", err)
	}

	var rowCount int32

	matched := len(expectations) == 0 // no expectations → match on any row

	for rows.Next() {
		rowCount++

		// Build a map of column → value for this row.
		rowValues, err := scanRowToMap(columns, rows)
		if err != nil {
			return nil, fmt.Errorf("scanning observe row %d: %w", rowCount, err)
		}

		// Check if this row satisfies all expectations.
		if !matched && matchesAllExpectations(rowValues, expectations) {
			matched = true
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating observe rows: %w", err)
	}

	return &Observation{
		Exists:   rowCount > 0 && matched,
		RowCount: rowCount,
		Matched:  matched,
	}, nil
}

// scanRowToMap scans a single row into a map[column]value.
func scanRowToMap(columns []string, rows *sql.Rows) (map[string]string, error) {
	vals := make([]sql.NullString, len(columns))
	ptrs := make([]any, len(columns))

	for i := range vals {
		ptrs[i] = &vals[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	m := make(map[string]string, len(columns))

	for i, col := range columns {
		if vals[i].Valid {
			m[strings.ToUpper(col)] = vals[i].String
		}
	}

	return m, nil
}

// matchesAllExpectations checks whether a row satisfies all expectations.
func matchesAllExpectations(row map[string]string, expectations []Expectation) bool {
	for _, exp := range expectations {
		val, ok := row[strings.ToUpper(exp.Column)]
		if !ok {
			return false
		}

		if !strings.EqualFold(val, exp.Value) {
			return false
		}
	}

	return true
}

// splitStatements splits a multi-statement SQL string by semicolons,
// respecting single-quoted string literals so that semicolons inside
// 'values' are not treated as statement separators. Escaped quotes
// (”) inside literals are handled correctly.
//
// Limitations: does not handle double-quoted identifiers, $$ blocks,
// or SQL comments containing semicolons. For those cases, use a single
// statement per SQLStatement resource.
func splitStatements(sqlInput string) []string {
	var stmts []string
	var current strings.Builder

	inQuote := false

	for i := 0; i < len(sqlInput); i++ {
		ch := sqlInput[i]

		switch {
		case ch == '\'' && !inQuote:
			inQuote = true
			current.WriteByte(ch)
		case ch == '\'' && inQuote:
			// Check for escaped quote ('')
			if i+1 < len(sqlInput) && sqlInput[i+1] == '\'' {
				current.WriteByte(ch)
				current.WriteByte(ch)
				i++ // skip next quote
			} else {
				inQuote = false
				current.WriteByte(ch)
			}
		case ch == ';' && !inQuote:
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}

			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	// Flush final segment.
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}

	return stmts
}

// HashSQL returns the hex-encoded SHA-256 hash of a SQL string.
// Used to detect spec.execute changes that require re-execution.
func HashSQL(sql string) string {
	h := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(h[:])
}
