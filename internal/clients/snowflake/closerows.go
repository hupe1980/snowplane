package snowflake

import (
	"database/sql"
	"log/slog"
)

// closeRows closes a *sql.Rows and logs any error at debug level.
// This replaces the `_ = rows.Close()` pattern throughout the Snowflake
// client layer, ensuring Close errors are observable for diagnostics
// while remaining non-blocking (rows.Close errors are rarely actionable).
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Debug("error closing SQL rows", "error", err)
	}
}
