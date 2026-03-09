package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hupe1980/snowplane/internal/clients/snowflake/sqlbuilder"
)

func TestSetWebhookHeaders(t *testing.T) {
	t.Run("nil map is no-op", func(t *testing.T) {
		var b sqlbuilder.Builder
		setWebhookHeaders(&b, nil)
		assert.Empty(t, b.String())
	})

	t.Run("empty map is no-op", func(t *testing.T) {
		var b sqlbuilder.Builder
		setWebhookHeaders(&b, map[string]string{})
		assert.Empty(t, b.String())
	})

	t.Run("single header", func(t *testing.T) {
		var b sqlbuilder.Builder
		setWebhookHeaders(&b, map[string]string{"Content-Type": "application/json"})
		assert.Contains(t, b.String(), "WEBHOOK_HEADER_Content-Type = 'application/json'")
	})

	t.Run("multiple headers sorted by key", func(t *testing.T) {
		var b sqlbuilder.Builder
		setWebhookHeaders(&b, map[string]string{
			"Z-Header": "last",
			"A-Header": "first",
			"M-Header": "middle",
		})
		s := b.String()
		aIdx := findIndex(s, "WEBHOOK_HEADER_A-Header")
		mIdx := findIndex(s, "WEBHOOK_HEADER_M-Header")
		zIdx := findIndex(s, "WEBHOOK_HEADER_Z-Header")
		assert.True(t, aIdx < mIdx, "A-Header should appear before M-Header")
		assert.True(t, mIdx < zIdx, "M-Header should appear before Z-Header")
	})
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}
