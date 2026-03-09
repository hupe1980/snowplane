package snowflake

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidExternalOAuthTypes(t *testing.T) {
	for _, valid := range []string{"OKTA", "AZURE", "PING_FEDERATE", "CUSTOM"} {
		assert.True(t, validExternalOAuthTypes[valid], "expected %q to be valid", valid)
	}

	for _, invalid := range []string{"okta", "OTHER", "", "SAML"} {
		assert.False(t, validExternalOAuthTypes[invalid], "expected %q to be invalid", invalid)
	}
}

func TestValidExternalOAuthAnyRoleModes(t *testing.T) {
	for _, valid := range []string{"DISABLE", "ENABLE", "ENABLE_FOR_PRIVILEGE"} {
		assert.True(t, validExternalOAuthAnyRoleModes[valid], "expected %q to be valid", valid)
	}

	for _, invalid := range []string{"disable", "ENABLED", "", "ALLOW"} {
		assert.False(t, validExternalOAuthAnyRoleModes[invalid], "expected %q to be invalid", invalid)
	}
}
