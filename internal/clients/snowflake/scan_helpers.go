package snowflake

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// parseInt32 parses a string to int32.
func parseInt32(s string) (int32, bool) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, false
	}

	return int32(v), true
}

// sqlString extracts a string from a sql.Rows column value.
func sqlString(v interface{}) string {
	if v == nil {
		return ""
	}

	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case *sql.NullString:
		if s.Valid {
			return s.String
		}

		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// scanDescribeKeyValue scans DESCRIBE output into a key-value map.
// It dynamically reads column names and looks for common patterns:
//   - "property" + "property_value" (DESCRIBE INTEGRATION)
//   - "name" + "value" (DESCRIBE NETWORK RULE, DESCRIBE PASSWORD POLICY)
//
// This is column-count agnostic and works for any DESCRIBE result format.
func scanDescribeKeyValue(rows *sql.Rows) (map[string]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading describe columns: %w", err)
	}

	// Determine key/value column indices from column names.
	keyIdx, valIdx := -1, -1

	for i, col := range cols {
		lc := strings.ToLower(col)

		switch lc {
		case "property", "name":
			if keyIdx == -1 {
				keyIdx = i
			}
		case "property_value", "value":
			if valIdx == -1 {
				valIdx = i
			}
		}
	}

	// Require explicit column name matches — no silent fallback.
	if keyIdx == -1 || valIdx == -1 {
		return nil, fmt.Errorf("cannot determine key/value columns from: %v", cols)
	}

	result := make(map[string]string)

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning describe row: %w", err)
		}

		if values[keyIdx].Valid && values[valIdx].Valid {
			result[values[keyIdx].String] = values[valIdx].String
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating describe rows: %w", err)
	}

	return result, nil
}

// ScanShowOutput is a generic helper that scans SHOW ... output rows for a
// single matching row by name and maps it to the target type using mapFn.
// It eliminates the repeated boilerplate of reading columns, scanning into
// NullStrings, building a colMap, filtering by name, and handling row errors.
//
// The 30 standard scanXxxShowOutput functions all share this exact pattern.
func ScanShowOutput[T any](rows *sql.Rows, name string, mapFn func(map[string]string) (*T, error)) (*T, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		if !strings.EqualFold(colMap["name"], name) {
			continue
		}

		return mapFn(colMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return nil, ErrObjectNotFound
}

// ScanParameters is a generic helper that scans SHOW PARAMETERS output rows
// into a target struct. Each row represents a key/value parameter pair.
// The processFn receives the target struct pointer along with the uppercase
// key and string value, allowing resource-specific field assignments.
//
// This eliminates the repeated boilerplate of reading columns, scanning into
// NullStrings, building a colMap, extracting key/value, and handling row errors
// across the 4 scanXxxParameters functions (database, schema, warehouse, task).
func ScanParameters[T any](rows *sql.Rows, processFn func(params *T, key, value string)) (*T, error) {
	var params T

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading columns: %w", err)
	}

	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))

		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scanning parameter row: %w", err)
		}

		colMap := make(map[string]string, len(cols))
		for i, col := range cols {
			if values[i].Valid {
				colMap[col] = values[i].String
			}
		}

		key := strings.ToUpper(colMap["key"])
		val := colMap["value"]

		processFn(&params, key, val)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating parameter rows: %w", err)
	}

	return &params, nil
}
