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
// +kubebuilder:validation:XValidation:rule="self.account == oldSelf.account",message="spec.account is immutable (changing would redirect to a different Snowflake account)"
// +kubebuilder:validation:XValidation:rule="self.user == oldSelf.user",message="spec.user is immutable (changing would redirect to a different Snowflake user)"
type ProviderConfigSpec struct {
	// Account is the Snowflake account identifier (e.g. "xy12345" or "orgname-accountname").
	// +kubebuilder:validation:MinLength=1
	Account string `json:"account"`

	// Region is the Snowflake cloud region (e.g. "us-east-1", "eu-west-1").
	// Modern Snowflake account identifiers (orgname-accountname format) encode
	// the region, making this field optional. Legacy account identifiers (xy12345)
	// still require an explicit region.
	// +optional
	Region string `json:"region,omitempty"`

	// User is the Snowflake user for authentication.
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// Role is the Snowflake role to assume. Defaults to the user's default role.
	Role string `json:"role,omitempty"`

	// Warehouse is the default warehouse for the session.
	Warehouse string `json:"warehouse,omitempty"`

	// AuthenticationType selects the authentication method.
	// +kubebuilder:validation:Enum=KeyPair;UsernamePassword;WorkloadIdentity;""
	AuthenticationType AuthenticationType `json:"authenticationType,omitempty"`

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
	// An empty list means all namespaces are allowed.
	// A list containing "*" means all namespaces are allowed.
	// If set to specific namespace names, only resources in those namespaces can use this ProviderConfig.
	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// ProviderCredentials references the Secret(s) holding authentication data.
type ProviderCredentials struct {
	// SecretRef references a Secret containing Snowflake credentials.
	// For KeyPair auth, the Secret must contain a PEM-encoded RSA private key.
	// For UsernamePassword auth, the Secret must contain a password.
	// +optional
	SecretRef *SecretKeyReference `json:"secretRef,omitempty"`
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
const DefaultTokenFilePath = "/var/run/secrets/snowflake/token"

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
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ProviderConfig configures the Snowflake connection for the Snowplane operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
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
		AuthenticationTypeKeyPair, AuthenticationTypeUsernamePassword, AuthenticationTypeWorkloadIdentity, ""); err != nil {
		errs = append(errs, err)
	}

	// Auth-type-specific credential validation.
	switch s.AuthenticationType {
	case AuthenticationTypeKeyPair:
		if s.Credentials.SecretRef == nil || s.Credentials.SecretRef.Name == "" || s.Credentials.SecretRef.Key == "" {
			errs = append(errs, errors.New("spec.credentials.secretRef (name and key) is required for KeyPair authentication"))
		}
	case AuthenticationTypeUsernamePassword:
		if s.Credentials.SecretRef == nil || s.Credentials.SecretRef.Name == "" || s.Credentials.SecretRef.Key == "" {
			errs = append(errs, errors.New("spec.credentials.secretRef (name and key) is required for UsernamePassword authentication"))
		}
	case AuthenticationTypeWorkloadIdentity:
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

// IsNamespaceAllowed returns true if the given namespace is allowed to reference
// this ProviderConfig. An empty AllowedNamespaces list or a list containing "*"
// means all namespaces are allowed.
func (s *ProviderConfigSpec) IsNamespaceAllowed(namespace string) bool {
	if len(s.AllowedNamespaces) == 0 {
		return true
	}

	for _, ns := range s.AllowedNamespaces {
		if ns == "*" || ns == namespace {
			return true
		}
	}

	return false
}

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
}
