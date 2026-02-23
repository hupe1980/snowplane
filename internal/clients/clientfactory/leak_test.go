package clientfactory

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies there are no goroutine leaks after all tests in this
// package complete. This catches leaked goroutines from connection pool usage.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// gosnowflake v1.19.0 spawns a background goroutine to load
		// its native "minicore" shared library via dlopen. This is a
		// one-time, driver-internal initialisation goroutine that we
		// cannot control — ignore it in the leak checker.
		goleak.IgnoreTopFunction("github.com/snowflakedb/gosnowflake._Cfunc_dlOpen"),
	)
}
