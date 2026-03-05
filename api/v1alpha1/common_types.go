package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObjectReference contains enough information to locate the referenced
// Kubernetes resource. If Namespace is omitted, the resource is assumed
// to be in the same namespace as the referencing object.
type ObjectReference struct {
	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Namespace of the referent. Defaults to the namespace of the
	// referencing resource when omitted, enabling cross-namespace
	// references in platform/project-team enterprise topologies.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SecretKeyReference contains enough information to locate the referenced
// Kubernetes Secret and the specific key within it.
type SecretKeyReference struct {
	// Name of the Kubernetes Secret.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Namespace of the Secret. Defaults to the namespace of the referring resource.
	Namespace string `json:"namespace,omitempty"`

	// Key within the Secret data to select.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Key string `json:"key"`
}

// SecretBinding binds a Snowflake secret to a variable name for use in procedure/function handler code.
// Secrets must be allowed by an external access integration specified in the same resource.
type SecretBinding struct {
	// SecretName is the fully qualified name of the Snowflake secret
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_SECRET").
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	SecretName string `json:"secretName"`

	// VariableName is the variable name used to reference the secret in handler code.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	VariableName string `json:"variableName"`
}

// ProviderReference selects a ProviderConfig by name and optionally by namespace.
type ProviderReference struct {
	// Name of the ProviderConfig to use.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:default="default"
	Name string `json:"name,omitempty"`

	// Namespace of the ProviderConfig. Defaults to the namespace of the
	// referring resource. Set this to reference a ProviderConfig in a
	// different namespace (e.g. a shared system namespace).
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// DeletionPolicy specifies what happens to the external resource when the
// Kubernetes CR is deleted.
type DeletionPolicy string

const (
	// DeletionPolicyDelete drops the Snowflake resource on CR deletion.
	DeletionPolicyDelete DeletionPolicy = "Delete"

	// DeletionPolicyOrphan leaves the Snowflake resource intact on CR deletion.
	DeletionPolicyOrphan DeletionPolicy = "Orphan"
)

// AdoptionPolicy controls how the reconciler handles pre-existing
// Snowflake resources on first reconciliation.
// +kubebuilder:validation:Enum=adopt;"fail-if-exists"
type AdoptionPolicy string

const (
	// AdoptionPolicyTypeAdopt allows the reconciler to adopt an existing
	// Snowflake resource, populating status from the current state.
	AdoptionPolicyTypeAdopt AdoptionPolicy = "adopt"

	// AdoptionPolicyTypeFailIfExists (the default) causes the reconciler to
	// return a terminal error when the Snowflake resource already exists.
	AdoptionPolicyTypeFailIfExists AdoptionPolicy = "fail-if-exists"
)

// DriftPolicy controls whether detected drift is corrected or only reported.
// +kubebuilder:validation:Enum=correct;"detect-only"
type DriftPolicy string

const (
	// DriftPolicyCorrect is the default: the reconciler issues ALTER
	// statements to bring Snowflake state in line with the spec.
	DriftPolicyCorrect DriftPolicy = "correct"

	// DriftPolicyDetectOnly reports drift via conditions and events but
	// does not issue any ALTER statements.
	DriftPolicyDetectOnly DriftPolicy = "detect-only"
)

// ManagementPolicies specifies lifecycle-level policies for a managed
// resource. These control adoption, drift correction, and CREATE OR ALTER
// behaviour. All fields are optional and default to production-safe values.
type ManagementPolicies struct {
	// AdoptionPolicy controls how the reconciler handles a pre-existing
	// Snowflake resource on first reconciliation.
	// Defaults to "fail-if-exists".
	// +kubebuilder:default="fail-if-exists"
	// +optional
	AdoptionPolicy AdoptionPolicy `json:"adoptionPolicy,omitempty"`

	// DriftPolicy controls whether detected drift is corrected or only
	// reported via conditions and events.
	// Defaults to "correct".
	// +kubebuilder:default=correct
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`

	// CreateOrAlter controls whether CREATE OR ALTER is used instead of
	// the legacy CREATE + ALTER two-step flow for supported resource types.
	// Defaults to true.
	// +kubebuilder:default=true
	// +optional
	CreateOrAlter *bool `json:"createOrAlter,omitempty"`
}

// IsCreateOrAlter returns true when CREATE OR ALTER should be used.
// Returns the value of CreateOrAlter if explicitly set, otherwise defaults to true.
func (m ManagementPolicies) IsCreateOrAlter() bool {
	if m.CreateOrAlter == nil {
		return true
	}

	return *m.CreateOrAlter
}

// IsDetectOnly returns true when the drift policy is set to detect-only.
func (m ManagementPolicies) IsDetectOnly() bool {
	return m.DriftPolicy == DriftPolicyDetectOnly
}

// CommonSpec contains fields shared by all managed resource specs.
type CommonSpec struct {
	// DeletionPolicy specifies what happens to the Snowflake resource when the CR is deleted.
	// Defaults to "Delete".
	// +kubebuilder:default=Delete
	// +kubebuilder:validation:Enum=Delete;Orphan
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// ProviderRef references the ProviderConfig to use for Snowflake connectivity.
	ProviderRef ProviderReference `json:"providerRef"`

	// UseRole specifies the Snowflake role to activate via USE ROLE before
	// CREATE/ALTER/DROP operations. In Snowflake's DAC model the role active
	// at CREATE time becomes the object owner, so this field indirectly
	// controls initial ownership.
	// This field is immutable after the resource has been created in Snowflake.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	UseRole *string `json:"useRole,omitempty"`

	// Paused suspends reconciliation of this resource. When true, the
	// controller skips all Snowflake operations and sets Synced=False
	// with reason ReconcilePaused. The Snowflake resource is not modified
	// or deleted while paused.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// ManagementPolicies specifies lifecycle policies for this resource.
	// +optional
	ManagementPolicies ManagementPolicies `json:"managementPolicies,omitempty"`
}

// CommonStatus contains fields shared by all managed resource statuses.
type CommonStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// FullyQualifiedName is the Snowflake fully qualified identifier for this resource.
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`

	// LastAppliedSpecHash is the SHA-256 hash of the spec as of the last
	// successful reconciliation. The reconciler uses this to distinguish
	// spec changes (user edited the CR) from external drift (Snowflake
	// state diverged without a spec change). This is more reliable than
	// comparing ObservedGeneration vs Generation because metadata-only
	// changes (labels, annotations) increment Generation without changing
	// the spec.
	LastAppliedSpecHash string `json:"lastAppliedSpecHash,omitempty"`

	// LastReconcileTime is the timestamp of the most recent successful
	// reconciliation. Useful for SLO monitoring and diagnosing whether
	// reconciliation is running for a specific resource.
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// ComputeSpecHash returns the hex-encoded SHA-256 hash of a spec struct.
// The spec is JSON-serialised for deterministic output. This is used to
// detect spec changes independent of the metadata generation counter.
func ComputeSpecHash(spec interface{}) (string, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("ComputeSpecHash: json.Marshal failed: %w", err)
	}

	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:]), nil
}
