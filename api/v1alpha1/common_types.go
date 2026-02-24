package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalObjectReference contains enough information to locate the referenced
// Kubernetes resource within the same namespace.
type LocalObjectReference struct {
	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretKeyReference contains enough information to locate the referenced
// Kubernetes Secret and the specific key within it.
type SecretKeyReference struct {
	// Name of the Kubernetes Secret.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the Secret. Defaults to the namespace of the referring resource.
	Namespace string `json:"namespace,omitempty"`

	// Key within the Secret data to select.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ProviderReference selects a ProviderConfig by name and optionally by namespace.
type ProviderReference struct {
	// Name of the ProviderConfig to use.
	// +kubebuilder:validation:MinLength=1
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
	UseRole *string `json:"useRole,omitempty"`
}

// CommonStatus contains fields shared by all managed resource statuses.
type CommonStatus struct {
	// ObservedGeneration is the most recent metadata.generation observed by the controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
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
