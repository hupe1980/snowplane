package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FieldExportTargetKind specifies the kind of target resource to write to.
// +kubebuilder:validation:Enum=ConfigMap;Secret
type FieldExportTargetKind string

const (
	// FieldExportTargetConfigMap writes the exported value to a ConfigMap.
	FieldExportTargetConfigMap FieldExportTargetKind = "ConfigMap"

	// FieldExportTargetSecret writes the exported value to a Secret.
	FieldExportTargetSecret FieldExportTargetKind = "Secret"
)

// FieldExportSource specifies the source resource and the field to export.
type FieldExportSource struct {
	// Resource references the source Snowplane managed resource.
	Resource FieldExportResourceRef `json:"resource"`

	// Path is a dot-notation path to the field in the source resource's status.
	// Examples: ".status.showOutput.createdOn", ".status.fullyQualifiedName"
	//
	// Only dot-separated field names are supported. Array indexing
	// (e.g. ".status.conditions[0].message") is not supported.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// FieldExportResourceRef identifies a Snowplane managed resource.
// The source resource must be in the same namespace as the FieldExport.
type FieldExportResourceRef struct {
	// Kind is the resource kind (e.g., "Database", "Warehouse", "Schema").
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the resource name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// FieldExportTarget specifies where to write the exported value.
// The target ConfigMap or Secret must be in the same namespace as the FieldExport.
// This prevents cross-namespace privilege escalation via Secret/ConfigMap writes.
type FieldExportTarget struct {
	// Kind is the target resource kind: "ConfigMap" or "Secret".
	Kind FieldExportTargetKind `json:"kind"`

	// Name is the target ConfigMap or Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key within the ConfigMap data or Secret data to write to.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// FieldExportSpec defines the desired state of FieldExport.
//
// +kubebuilder:validation:XValidation:rule="self.from.path.startsWith('.status.')",message="spec.from.path must start with '.status.'"
// +kubebuilder:validation:XValidation:rule="!self.from.path.contains('[')",message="spec.from.path does not support array indexing"
// +kubebuilder:validation:XValidation:rule="self.from.resource.kind in ['Alert','Database','Schema','Warehouse','User','AccountRole','DatabaseRole','AccountRoleGrant','DatabaseRoleGrant','ShareGrant','AccountRoleAssignment','DatabaseRoleAssignment','Table','View','Stage','Task','Stream','Tag','NetworkPolicy','ResourceMonitor','MaskingPolicy','RowAccessPolicy','GrantOwnership','StorageIntegration','SecurityIntegration','FileFormat','Pipe','DynamicTable','PasswordPolicy','NetworkRule']",message="spec.from.resource.kind must be a supported Snowplane resource kind"
// +kubebuilder:validation:XValidation:rule="self.from.resource.kind == oldSelf.from.resource.kind",message="spec.from.resource.kind is immutable"
// +kubebuilder:validation:XValidation:rule="self.from.resource.name == oldSelf.from.resource.name",message="spec.from.resource.name is immutable"
// +kubebuilder:validation:XValidation:rule="self.to.kind == oldSelf.to.kind",message="spec.to.kind is immutable"
// +kubebuilder:validation:XValidation:rule="self.to.name == oldSelf.to.name",message="spec.to.name is immutable"
// +kubebuilder:validation:XValidation:rule="self.to.key == oldSelf.to.key",message="spec.to.key is immutable"
type FieldExportSpec struct {
	// From specifies the source resource and field path.
	From FieldExportSource `json:"from"`

	// To specifies the target ConfigMap or Secret.
	To FieldExportTarget `json:"to"`
}

// FieldExportStatus defines the observed state of FieldExport.
type FieldExportStatus struct {
	// Conditions represent the latest available observations.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastExportedValueHash is a SHA-256 hash of the last exported value.
	// For Secrets, the raw value is never exposed in status.
	LastExportedValueHash string `json:"lastExportedValueHash,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fexp,categories=snowplane
// +kubebuilder:printcolumn:name="Source Kind",type=string,JSONPath=`.spec.from.resource.kind`
// +kubebuilder:printcolumn:name="Source Name",type=string,JSONPath=`.spec.from.resource.name`
// +kubebuilder:printcolumn:name="Target Kind",type=string,JSONPath=`.spec.to.kind`
// +kubebuilder:printcolumn:name="Target Name",type=string,JSONPath=`.spec.to.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// FieldExport copies a field from a Snowplane managed resource's status into a
// ConfigMap or Secret, enabling cross-resource data passing in Kubernetes.
type FieldExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FieldExportSpec   `json:"spec,omitempty"`
	Status FieldExportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FieldExportList contains a list of FieldExport.
type FieldExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FieldExport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FieldExport{}, &FieldExportList{})
}

// GetConditions returns the resource's conditions.
func (f *FieldExport) GetConditions() []metav1.Condition {
	return f.Status.Conditions
}

// SetConditions sets the resource's conditions.
func (f *FieldExport) SetConditions(conditions []metav1.Condition) {
	f.Status.Conditions = conditions
}
