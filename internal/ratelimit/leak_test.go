package ratelimit

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain verifies there are no goroutine leaks after all tests in this
// package complete. This catches leaked goroutines from rate limiter usage.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
