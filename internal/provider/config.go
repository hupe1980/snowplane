// Package provider contains shared utilities for resolving ProviderConfig
// resources and building Snowflake client configurations.
package provider

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
)

// AllowedTokenPathPrefixes lists the directory prefixes that tokenFilePath is
// restricted to. This prevents path traversal attacks where a malicious
// ProviderConfig reads arbitrary files from the operator pod filesystem.
var AllowedTokenPathPrefixes = []string{
	"/var/run/secrets/",
}

// BuildSnowflakeConfig constructs a snowflake.Config from a ProviderConfig
// and its referenced Secret. For WorkloadIdentity authentication, the secret
// parameter must be nil — the gosnowflake driver reads the token file natively.
func BuildSnowflakeConfig(pc *snowplanev1alpha1.ProviderConfig, secret *corev1.Secret) (snowflake.Config, error) {
	cfg := snowflake.Config{
		Account:   pc.Spec.Account,
		User:      pc.Spec.User,
		Region:    pc.Spec.Region,
		Role:      pc.Spec.Role,
		Warehouse: pc.Spec.Warehouse,
	}

	switch pc.Spec.AuthenticationType {
	case snowplanev1alpha1.AuthenticationTypeWorkloadIdentity:
		// Native WIF: pass the token file path to the gosnowflake driver.
		// The driver reads the token on each new connection and handles
		// the OIDC/AWS/GCP/Azure attestation exchange automatically.
		if pc.Spec.WorkloadIdentity == nil {
			return snowflake.Config{}, fmt.Errorf("spec.workloadIdentity is required for WorkloadIdentity authentication")
		}

		tokenPath := pc.Spec.WorkloadIdentity.GetTokenFilePath()
		if err := ValidateTokenFilePath(tokenPath); err != nil {
			return snowflake.Config{}, fmt.Errorf("invalid token file path: %w", err)
		}

		cfg.TokenFilePath = tokenPath
		cfg.WorkloadIdentityProvider = string(pc.Spec.WorkloadIdentity.GetProvider())

		return cfg, nil

	case snowplanev1alpha1.AuthenticationTypeKeyPair,
		snowplanev1alpha1.AuthenticationTypeUsernamePassword:
		// Secret-based authentication.
		if pc.Spec.Credentials.SecretRef == nil {
			return snowflake.Config{}, fmt.Errorf("spec.credentials.secretRef is required for %s authentication",
				pc.Spec.AuthenticationType)
		}

		if secret == nil {
			return snowflake.Config{}, fmt.Errorf("secret is required for %s authentication",
				pc.Spec.AuthenticationType)
		}

		key := pc.Spec.Credentials.SecretRef.Key

		data, ok := secret.Data[key]
		if !ok {
			return snowflake.Config{}, fmt.Errorf("secret %s/%s does not contain key %q",
				secret.Namespace, secret.Name, key)
		}

		switch pc.Spec.AuthenticationType {
		case snowplanev1alpha1.AuthenticationTypeKeyPair:
			cfg.PrivateKey = string(data)

			// Optionally read the passphrase for encrypted PKCS#8 keys.
			// The passphrase must be in the same Secret, referenced by key name.
			if ppKey := pc.Spec.Credentials.PassphraseKey; ppKey != "" {
				ppData, ppOK := secret.Data[ppKey]

				if !ppOK {
					return snowflake.Config{}, fmt.Errorf("secret %s/%s does not contain passphrase key %q",
						secret.Namespace, secret.Name, ppKey)
				}

				cfg.Passphrase = string(ppData)
			}
		case snowplanev1alpha1.AuthenticationTypeUsernamePassword:
			cfg.Password = string(data)
		}

		return cfg, nil

	case "":
		return snowflake.Config{}, fmt.Errorf("authenticationType is required (valid values: %q, %q, %q)",
			snowplanev1alpha1.AuthenticationTypeKeyPair,
			snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			snowplanev1alpha1.AuthenticationTypeWorkloadIdentity)
	default:
		return snowflake.Config{}, fmt.Errorf("unsupported authentication type: %q (valid values: %q, %q, %q)",
			pc.Spec.AuthenticationType,
			snowplanev1alpha1.AuthenticationTypeKeyPair,
			snowplanev1alpha1.AuthenticationTypeUsernamePassword,
			snowplanev1alpha1.AuthenticationTypeWorkloadIdentity)
	}
}

// ValidateTokenFilePath returns an error if path is outside the allowed prefixes.
// The path is cleaned (resolving . and ..) before checking.
func ValidateTokenFilePath(path string) error {
	cleaned := filepath.Clean(path)

	for _, prefix := range AllowedTokenPathPrefixes {
		if strings.HasPrefix(cleaned, prefix) {
			return nil
		}
	}

	return fmt.Errorf("tokenFilePath %q is not under an allowed prefix %v", path, AllowedTokenPathPrefixes)
}

// ComputeHash returns a deterministic hex digest of the relevant config fields.
// Uses null byte separators so "ab"+"cd" != "abc"+"d".
// For WorkloadIdentity, the hash includes the token file path and provider
// (not the token itself, which changes on every kubelet refresh).
func ComputeHash(cfg snowflake.Config) string {
	h := sha256.New()

	for _, v := range []string{
		cfg.Account,
		cfg.User,
		cfg.Region,
		cfg.Role,
		cfg.Warehouse,
		cfg.Password,
		cfg.PrivateKey,
		cfg.Passphrase,
		cfg.TokenFilePath,
		cfg.WorkloadIdentityProvider,
	} {
		h.Write([]byte(v))
		h.Write([]byte{'\x00'})
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}
