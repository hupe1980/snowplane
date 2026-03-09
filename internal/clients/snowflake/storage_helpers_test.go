package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildLocationList(t *testing.T) {
	t.Run("single location", func(t *testing.T) {
		got := buildLocationList([]string{"s3://bucket/path/"})
		assert.Equal(t, "('s3://bucket/path/')", got)
	})

	t.Run("multiple locations", func(t *testing.T) {
		got := buildLocationList([]string{"s3://a/", "s3://b/", "s3://c/"})
		assert.Equal(t, "('s3://a/', 's3://b/', 's3://c/')", got)
	})

	t.Run("empty list", func(t *testing.T) {
		got := buildLocationList([]string{})
		assert.Equal(t, "()", got)
	})

	t.Run("escapes single quotes", func(t *testing.T) {
		got := buildLocationList([]string{"s3://bucket/it's/"})
		assert.Equal(t, "('s3://bucket/it''s/')", got)
	})
}

func TestBuildStringListClause(t *testing.T) {
	t.Run("single value", func(t *testing.T) {
		got := buildStringListClause("ALLOWED_LOCATIONS", []string{"s3://bucket/"})
		assert.Equal(t, "ALLOWED_LOCATIONS = ('s3://bucket/')", got)
	})

	t.Run("multiple values", func(t *testing.T) {
		got := buildStringListClause("BLOCKED_LOCATIONS", []string{"a", "b", "c"})
		assert.Equal(t, "BLOCKED_LOCATIONS = ('a', 'b', 'c')", got)
	})

	t.Run("empty values", func(t *testing.T) {
		got := buildStringListClause("KEYWORD", []string{})
		assert.Equal(t, "KEYWORD = ()", got)
	})
}

func TestBuildEmailListClause(t *testing.T) {
	t.Run("delegates to buildStringListClause", func(t *testing.T) {
		got := buildEmailListClause("ALLOWED_RECIPIENTS", []string{"a@b.com", "c@d.com"})
		assert.Equal(t, "ALLOWED_RECIPIENTS = ('a@b.com', 'c@d.com')", got)
	})
}
