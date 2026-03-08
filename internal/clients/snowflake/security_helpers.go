package snowflake

import (
	"database/sql"
	"strings"

	v1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

// validExternalOAuthTypes is the allowlist of EXTERNAL_OAUTH_TYPE values.
var validExternalOAuthTypes = map[string]bool{
	"OKTA":          true,
	"AZURE":         true,
	"PING_FEDERATE": true,
	"CUSTOM":        true,
}

// validExternalOAuthAnyRoleModes is the allowlist of EXTERNAL_OAUTH_ANY_ROLE_MODE values.
var validExternalOAuthAnyRoleModes = map[string]bool{
	"DISABLE":              true,
	"ENABLE":               true,
	"ENABLE_FOR_PRIVILEGE": true,
}

// scanSecurityIntegrationShowOutput scans SHOW SECURITY INTEGRATIONS results.
// This is shared between the SecurityIntegration and APIAuthenticationIntegration clients.
func scanSecurityIntegrationShowOutput(rows *sql.Rows, name string) (*v1alpha1.SecurityIntegrationShowOutput, error) {
	return ScanShowOutput(rows, name, func(m map[string]string) (*v1alpha1.SecurityIntegrationShowOutput, error) {
		return &v1alpha1.SecurityIntegrationShowOutput{
			CreatedOn: m["created_on"],
			Name:      m["name"],
			Type:      m["type"],
			Category:  m["category"],
			Enabled:   strings.EqualFold(m["enabled"], "true"),
			Comment:   m["comment"],
		}, nil
	})
}
