package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsKnownPrivilege(t *testing.T) {
	t.Parallel()

	tests := []struct {
		priv string
		want bool
	}{
		{"SELECT", true},
		{"select", true},
		{"  SELECT  ", true},
		{"INSERT", true},
		{"ALL PRIVILEGES", true},
		{"ALL", true},
		{"CREATE DATABASE", true},
		{"USAGE", true},
		{"OWNERSHIP", true},
		{"MANAGE GRANTS", true},
		{"CREATE SCHEMA", true},
		{"EVOLVE SCHEMA", true},
		{"REFERENCE_USAGE", true},
		{"SELCET", false},    // typo
		{"DLEETE", false},    // typo
		{"SUPERUSER", false}, // not a Snowflake privilege
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.priv, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsKnownPrivilege(tt.priv))
		})
	}
}
