package v1alpha1

import (
	"errors"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthenticationType specifies how the operator authenticates to Snowflake.
type AuthenticationType string

// Supported authentication types.
const (
	AuthenticationTypeKeyPair          AuthenticationType = "KeyPair"
	AuthenticationTypeUsernamePassword AuthenticationType = "UsernamePassword"
	AuthenticationTypeWorkloadIdentity AuthenticationType = "WorkloadIdentity"
)

// WorkloadIdentityProvider specifies the cloud provider for WIF attestation.
type WorkloadIdentityProvider string

// Supported workload identity providers.
const (
	// WIFProviderOIDC uses a projected ServiceAccount OIDC token directly.
	WIFProviderOIDC WorkloadIdentityProvider = "OIDC"
	// WIFProviderAWS uses AWS IAM credentials (IRSA / Pod Identity).
	WIFProviderAWS WorkloadIdentityProvider = "AWS"
	// WIFProviderGCP uses GCP metadata service / service account impersonation.
	WIFProviderGCP WorkloadIdentityProvider = "GCP"
	// WIFProviderAzure uses Azure VM IMDS / managed identity.
	WIFProviderAzure WorkloadIdentityProvider = "Azure"
)

// ProviderConfigSpec defines the desired state of ProviderConfig.
//
// +kubebuilder:validation:XValidation:rule="self.authenticationType != 'KeyPair' || (has(self.credentials) && has(self.credentials.secretRef) && size(self.credentials.secretRef.name) > 0 && size(self.credentials.secretRef.key) > 0)",message="spec.credentials.secretRef (name and key) is required for KeyPair authentication"
// +kubebuilder:validation:XValidation:rule="self.authenticationType != 'UsernamePassword' || (has(self.credentials) && has(self.credentials.secretRef) && size(self.credentials.secretRef.name) > 0 && size(self.credentials.secretRef.key) > 0)",message="spec.credentials.secretRef (name and key) is required for UsernamePassword authentication"
// +kubebuilder:validation:XValidation:rule="self.authenticationType != 'WorkloadIdentity' || has(self.workloadIdentity)",message="spec.workloadIdentity is required for WorkloadIdentity authentication"
// +kubebuilder:validation:XValidation:rule="self.authenticationType != 'WorkloadIdentity' || !has(self.credentials) || !has(self.credentials.secretRef) || size(self.credentials.secretRef.name) == 0",message="spec.credentials.secretRef must not be set for WorkloadIdentity authentication"
// +kubebuilder:validation:XValidation:rule="!has(self.credentials) || !has(self.credentials.passphraseKey) || size(self.credentials.passphraseKey) == 0 || self.authenticationType == 'KeyPair'",message="spec.credentials.passphraseKey is only valid for KeyPair authentication"
// +kubebuilder:validation:XValidation:rule="self.account == oldSelf.account",message="spec.account is immutable (changing would redirect to a different Snowflake account)"
// +kubebuilder:validation:XValidation:rule="self.user == oldSelf.user",message="spec.user is immutable (changing would redirect to a different Snowflake user)"
type ProviderConfigSpec struct {
	// Account is the Snowflake account identifier (e.g. "xy12345" or "orgname-accountname").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Account string `json:"account"`

	// Region is the Snowflake cloud region (e.g. "us-east-1", "eu-west-1").
	// Modern Snowflake account identifiers (orgname-accountname format) encode
	// the region, making this field optional. Legacy account identifiers (xy12345)
	// still require an explicit region.
	// +optional
	// +kubebuilder:validation:MaxLength=255
	Region string `json:"region,omitempty"`

	// User is the Snowflake user for authentication.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	User string `json:"user"`

	// Role is the Snowflake role to assume. Defaults to the user's default role.
	// +kubebuilder:validation:MaxLength=255
	Role string `json:"role,omitempty"`

	// Warehouse is the default warehouse for the session.
	// +kubebuilder:validation:MaxLength=255
	Warehouse string `json:"warehouse,omitempty"`

	// AuthenticationType selects the authentication method.
	// +kubebuilder:validation:Enum=KeyPair;UsernamePassword;WorkloadIdentity
	AuthenticationType AuthenticationType `json:"authenticationType"`

	// Credentials references a Kubernetes Secret containing authentication material.
	// Required for KeyPair and UsernamePassword authentication.
	// Must not be set for WorkloadIdentity authentication.
	// +optional
	Credentials ProviderCredentials `json:"credentials,omitempty"`

	// WorkloadIdentity configures Workload Identity Federation for passwordless authentication.
	// Required when authenticationType is set to WorkloadIdentity.
	// +optional
	WorkloadIdentity *WorkloadIdentitySpec `json:"workloadIdentity,omitempty"`

	// AllowedNamespaces restricts which namespaces may reference this ProviderConfig.
	// An empty list means all namespaces are allowed (unless AllowedNamespaceSelector is set).
	// A list containing "*" means all namespaces are allowed.
	// If set to specific namespace names, only resources in those namespaces can use this ProviderConfig.
	// When both AllowedNamespaces and AllowedNamespaceSelector are set, a namespace is
	// allowed if it matches either the static list or the label selector (OR semantics).
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	// AllowedNamespaceSelector restricts which namespaces may reference this ProviderConfig
	// using Kubernetes label matching. A namespace is allowed if its labels match this selector.
	// This field uses OR semantics with AllowedNamespaces — a namespace is allowed if it
	// matches either the static list or the label selector.
	// If both AllowedNamespaces and AllowedNamespaceSelector are nil/empty, all namespaces
	// are allowed.
	// +optional
	AllowedNamespaceSelector *metav1.LabelSelector `json:"allowedNamespaceSelector,omitempty"`

	// AllowedDatabases restricts which Snowflake databases may be targeted by resources
	// using this ProviderConfig. An empty list means all databases are allowed.
	// Entries are matched case-insensitively against the resolved database name.
	// A list containing "*" means all databases are allowed.
	// +optional
	AllowedDatabases []string `json:"allowedDatabases,omitempty"`

	// AllowedSchemas restricts which Snowflake schemas may be targeted by resources
	// using this ProviderConfig. Entries use "DATABASE.SCHEMA" format for fully-qualified
	// matching, or just "SCHEMA" for name-only matching across all databases.
	// A list containing "*" means all schemas are allowed.
	// An empty list means all schemas are allowed.
	// Entries are matched case-insensitively.
	// +optional
	AllowedSchemas []string `json:"allowedSchemas,omitempty"`

	// AllowedRefNamespaces restricts which namespaces may be referenced via
	// cross-namespace ObjectReference fields (e.g. databaseRef.namespace,
	// schemaRef.namespace, secretRef.namespace).
	// An empty list means cross-namespace references are unrestricted.
	// A list containing "*" means all namespaces are allowed.
	// A list containing only the special value "SAME" means only same-namespace
	// references are permitted (cross-namespace references are blocked entirely).
	// +optional
	AllowedRefNamespaces []string `json:"allowedRefNamespaces,omitempty"`
}

// ProviderCredentials references the Secret(s) holding authentication data.
type ProviderCredentials struct {
	// SecretRef references a Secret containing Snowflake credentials.
	// For KeyPair auth, the Secret must contain a PEM-encoded RSA private key.
	// For UsernamePassword auth, the Secret must contain a password.
	// +optional
	SecretRef *SecretKeyReference `json:"secretRef,omitempty"`

	// PassphraseKey optionally specifies the key within the same Secret
	// (referenced by SecretRef) that contains the passphrase for an encrypted
	// PKCS#8 private key. Only valid with KeyPair authentication when the
	// private key is encrypted (PEM type "ENCRYPTED PRIVATE KEY").
	// The passphrase must be stored in the same Secret as the private key.
	// +optional
	PassphraseKey string `json:"passphraseKey,omitempty"`
}

// WorkloadIdentitySpec configures Workload Identity Federation (WIF) for passwordless
// Snowflake authentication. The gosnowflake driver natively supports WIF via
// AuthTypeWorkloadIdentityFederation — the operator passes the token file path
// and provider to the driver, which handles token reading and refresh automatically.
type WorkloadIdentitySpec struct {
	// Provider selects the cloud-specific WIF attestation provider.
	// "OIDC" uses projected ServiceAccount tokens (works on any cluster).
	// "AWS" uses IAM credentials from EKS IRSA or Pod Identity.
	// "GCP" uses GCE metadata service or service account impersonation.
	// "Azure" uses Azure IMDS or managed identity.
	// Defaults to "OIDC" if not set.
	// +kubebuilder:validation:Enum=OIDC;AWS;GCP;Azure;""
	// +optional
	Provider WorkloadIdentityProvider `json:"provider,omitempty"`

	// TokenFilePath is the path to the projected ServiceAccount token file.
	// Only used when provider is "OIDC" (or empty, which defaults to OIDC).
	// Defaults to /var/run/secrets/snowflake/token if not set.
	// Must be under /var/run/secrets/ for security.
	// +optional
	TokenFilePath string `json:"tokenFilePath,omitempty"`

	// Audience is the intended audience for the projected ServiceAccount token.
	// This should match the audience configured in the Snowflake security integration.
	// Defaults to the Snowflake account URL if not set.
	// +optional
	Audience string `json:"audience,omitempty"`
}

// DefaultTokenFilePath is the default path for projected SA tokens.
const DefaultTokenFilePath = "/var/run/secrets/snowflake/token" //nolint:gosec // G101: not a credential, it's a fixed mount path

// GetTokenFilePath returns the effective token file path, applying the default.
func (w *WorkloadIdentitySpec) GetTokenFilePath() string {
	if w.TokenFilePath != "" {
		return w.TokenFilePath
	}

	return DefaultTokenFilePath
}

// GetProvider returns the effective WIF provider, defaulting to OIDC.
func (w *WorkloadIdentitySpec) GetProvider() WorkloadIdentityProvider {
	if w.Provider != "" {
		return w.Provider
	}

	return WIFProviderOIDC
}

// ProviderConfigStatus defines the observed state of ProviderConfig.
type ProviderConfigStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the ProviderConfig's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ProviderConfig configures the Snowflake connection for the Snowplane operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=pc
// +kubebuilder:printcolumn:name="ACCOUNT",type=string,JSONPath=`.spec.account`
// +kubebuilder:printcolumn:name="AUTH",type=string,JSONPath=`.spec.authenticationType`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec,omitempty"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// ProviderConfigList contains a list of ProviderConfig.
// +kubebuilder:object:root=true
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// Validate checks the ProviderConfigSpec for configuration errors.
// Returns an errors.Join aggregate of all validation issues found.
func (s *ProviderConfigSpec) Validate() error {
	var errs []error

	if s.Account == "" {
		errs = append(errs, errors.New("spec.account is required"))
	}

	if s.User == "" {
		errs = append(errs, errors.New("spec.user is required"))
	}

	if err := validateEnum("spec.authenticationType", &s.AuthenticationType,
		AuthenticationTypeKeyPair, AuthenticationTypeUsernamePassword, AuthenticationTypeWorkloadIdentity); err != nil {
		errs = append(errs, err)
	}

	// Auth-type-specific credential validation.
	switch s.AuthenticationType {
	case AuthenticationTypeKeyPair:
		if s.Credentials.SecretRef == nil || s.Credentials.SecretRef.Name == "" || s.Credentials.SecretRef.Key == "" {
			errs = append(errs, errors.New("spec.credentials.secretRef (name and key) is required for KeyPair authentication"))
		}
	case AuthenticationTypeUsernamePassword:
		if s.Credentials.PassphraseKey != "" {
			errs = append(errs, errors.New("spec.credentials.passphraseKey is only valid for KeyPair authentication"))
		}
		if s.Credentials.SecretRef == nil || s.Credentials.SecretRef.Name == "" || s.Credentials.SecretRef.Key == "" {
			errs = append(errs, errors.New("spec.credentials.secretRef (name and key) is required for UsernamePassword authentication"))
		}
	case AuthenticationTypeWorkloadIdentity:
		if s.Credentials.PassphraseKey != "" {
			errs = append(errs, errors.New("spec.credentials.passphraseKey is only valid for KeyPair authentication"))
		}

		if s.WorkloadIdentity == nil {
			errs = append(errs, errors.New("spec.workloadIdentity is required for WorkloadIdentity authentication"))
		} else {
			// Validate token file path for OIDC provider.
			if s.WorkloadIdentity.GetProvider() == WIFProviderOIDC {
				tokenPath := s.WorkloadIdentity.GetTokenFilePath()
				cleaned := filepath.Clean(tokenPath)
				if !strings.HasPrefix(cleaned, "/var/run/secrets/") {
					errs = append(errs, errors.New("spec.workloadIdentity.tokenFilePath must be under /var/run/secrets/"))
				}
			}

			// Validate provider enum.
			p := s.WorkloadIdentity.Provider
			if p != "" && p != WIFProviderOIDC && p != WIFProviderAWS && p != WIFProviderGCP && p != WIFProviderAzure {
				errs = append(errs, errors.New("spec.workloadIdentity.provider must be one of: OIDC, AWS, GCP, Azure"))
			}
		}

		// WorkloadIdentity and credentials.secretRef are mutually exclusive.
		if s.Credentials.SecretRef != nil && s.Credentials.SecretRef.Name != "" {
			errs = append(errs, errors.New("spec.credentials.secretRef must not be set for WorkloadIdentity authentication"))
		}
	}

	return errors.Join(errs...)
}

// GetConditions returns the conditions of the ProviderConfig.
func (pc *ProviderConfig) GetConditions() []metav1.Condition {
	return pc.Status.Conditions
}

// SetConditions sets the conditions of the ProviderConfig.
func (pc *ProviderConfig) SetConditions(conditions []metav1.Condition) {
	pc.Status.Conditions = conditions
}

// IsNamespaceAllowed checks the static AllowedNamespaces list.
// It returns true if the static list is unset/empty and no label selector is configured
// (backward compatible: no restrictions = all allowed).
// It does NOT evaluate AllowedNamespaceSelector — use the provider resolver's
// isNamespacePermitted for the full check including label selectors.
func (s *ProviderConfigSpec) IsNamespaceAllowed(namespace string) bool {
	if len(s.AllowedNamespaces) == 0 {
		// No static list: allow-all only if no selector is configured either.
		return s.AllowedNamespaceSelector == nil
	}

	for _, ns := range s.AllowedNamespaces {
		if ns == "*" || ns == namespace {
			return true
		}
	}

	return false
}

// IsDatabaseAllowed returns true if the given database name is permitted by
// this ProviderConfig's AllowedDatabases restriction. An empty list or a list
// containing "*" means all databases are allowed. Matching is case-insensitive.
func (s *ProviderConfigSpec) IsDatabaseAllowed(database string) bool {
	if len(s.AllowedDatabases) == 0 {
		return true
	}

	for _, db := range s.AllowedDatabases {
		if db == "*" || strings.EqualFold(db, database) {
			return true
		}
	}

	return false
}

// IsSchemaAllowed returns true if the given schema is permitted by this
// ProviderConfig's AllowedSchemas restriction. An empty list or a list
// containing "*" means all schemas are allowed. Entries may be:
//   - "SCHEMA" — matches the schema name in any database (case-insensitive)
//   - "DATABASE.SCHEMA" — matches the fully-qualified name (case-insensitive)
func (s *ProviderConfigSpec) IsSchemaAllowed(database, schemaName string) bool {
	if len(s.AllowedSchemas) == 0 {
		return true
	}

	for _, entry := range s.AllowedSchemas {
		if entry == "*" {
			return true
		}

		if parts := strings.SplitN(entry, ".", 2); len(parts) == 2 {
			// Fully-qualified: DATABASE.SCHEMA
			if strings.EqualFold(parts[0], database) && strings.EqualFold(parts[1], schemaName) {
				return true
			}
		} else {
			// Schema-name-only: matches in any database
			if strings.EqualFold(entry, schemaName) {
				return true
			}
		}
	}

	return false
}

// IsRefNamespaceAllowed checks whether a cross-namespace reference target is
// permitted by this ProviderConfig's AllowedRefNamespaces restriction.
//
// Rules:
//   - Empty list → all namespaces allowed (no restriction)
//   - "*" in list → all namespaces allowed
//   - "SAME" in list → only sourceNamespace is allowed (blocks cross-namespace)
//   - Specific namespace names → only those namespaces are allowed
//
// The sourceNamespace is the namespace of the resource performing the reference.
// The targetNamespace is the namespace being referenced (the ref.Namespace override).
// If targetNamespace is empty (same-namespace ref), it is always allowed.
func (s *ProviderConfigSpec) IsRefNamespaceAllowed(sourceNamespace, targetNamespace string) bool {
	// Same-namespace references are always allowed.
	if targetNamespace == "" || targetNamespace == sourceNamespace {
		return true
	}

	// No restriction configured — allow all.
	if len(s.AllowedRefNamespaces) == 0 {
		return true
	}

	for _, ns := range s.AllowedRefNamespaces {
		if ns == "*" {
			return true
		}

		// "SAME" means only same-namespace refs are allowed.
		// Since we already handled the same-namespace case above,
		// reaching here means the ref IS cross-namespace → deny.
		if ns == "SAME" {
			continue
		}

		if ns == targetNamespace {
			return true
		}
	}

	return false
}

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
}
