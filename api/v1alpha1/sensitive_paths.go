package v1alpha1

import "strings"

// SensitiveStatusPaths maps CRD kinds to status path prefixes that may contain
// sensitive information (OAuth tokens, passwords, keys, endpoints, PII, etc.).
//
// When a FieldExport targets a ConfigMap (unencrypted at rest), paths that match
// any prefix listed here are rejected. Exporting to a Secret is always allowed.
//
// Keys are CRD Kind strings; values are path prefixes relative to ".status."
// (e.g. "describeOutput" blocks ".status.describeOutput" and all sub-paths).
//
// A wildcard entry "*" applies to all kinds that are not explicitly listed.
var SensitiveStatusPaths = map[string][]string{
	// --- Secrets (OAuth/basic-auth credentials) ---
	"SecretWithClientCredentials":      {"describeOutput", "showOutput.oauthScopes", "showOutput.secretType"},
	"SecretWithAuthorizationCodeGrant": {"describeOutput", "showOutput.oauthScopes", "showOutput.secretType"},
	"SecretWithBasicAuthentication":    {"describeOutput", "showOutput.secretType"},
	"SecretWithGenericString":          {"describeOutput", "showOutput.secretType"},

	// --- API authentication integrations (OAuth client IDs, token endpoints) ---
	"APIAuthenticationIntegrationWithClientCredentials":      {"describeOutput"},
	"APIAuthenticationIntegrationWithAuthorizationCodeGrant": {"describeOutput"},
	"APIAuthenticationIntegrationWithJWTBearer":              {"describeOutput"},

	// --- Integrations with untyped describe maps ---
	"SAML2Integration":         {"describeOutput"},
	"ExternalOAuthIntegration": {"describeOutput"},
	"APIIntegration":           {"describeOutput"},
	"PasswordPolicy":           {"describeOutput"},
	"AuthenticationPolicy":     {"describeOutput"},

	// --- User (PII, key fingerprints, credential hashes) ---
	"User": {
		"describeOutput.rsaPublicKeyFp",
		"describeOutput.rsaPublicKey2Fp",
		"describeOutput.networkPolicy",
		"showOutput.email",
		"showOutput.loginName",
		"lastAppliedPasswordHash",
		"lastAppliedRSAPublicKeyHash",
		"lastAppliedRSAPublicKey2Hash",
	},

	// --- Infrastructure secrets ---
	"StorageIntegrationAWS":   {"storageAWSIAMUserARN", "storageAWSExternalID"},
	"StorageIntegrationGCS":   {"storageGCPServiceAccount"},
	"StorageIntegrationAzure": {"azureConsentURL", "azureMultiTenantAppName"},
	"ExternalStage":           {"showOutput.url"},

	"ExternalFunction": {"url"},
	"Pipe": {
		"showOutput.notificationChannel",
		"showOutput.definition",
		"showOutput.awsSnsTopic",
		"notificationChannel",
	},
	"ImageRepository": {"showOutput.repositoryUrl"},
}

// IsSensitiveStatusPath reports whether path is a sensitive status path for the
// given CRD kind. path must start with ".status." (same format used in
// FieldExportSpec.From.Path).
//
// The match is prefix-based: if "describeOutput" is listed for a kind, then
// ".status.describeOutput", ".status.describeOutput.oauthClientId", etc. all
// match.
func IsSensitiveStatusPath(kind, path string) bool {
	// Strip ".status." prefix so we can match against the registered sub-paths.
	stripped := strings.TrimPrefix(path, ".status.")
	if stripped == path {
		// Path doesn't start with ".status." — nothing to guard.
		return false
	}

	prefixes, ok := SensitiveStatusPaths[kind]
	if !ok {
		return false
	}

	for _, prefix := range prefixes {
		if stripped == prefix || strings.HasPrefix(stripped, prefix+".") {
			return true
		}
	}

	return false
}
