package snowflake

import (
	"database/sql"
	"fmt"
	"strconv"
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
