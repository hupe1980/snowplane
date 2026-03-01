package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TagAssociationSpec defines the desired state of a TagAssociation.
// Associates a Snowflake tag with an object:
//
//	ALTER <object_type> <object_name> SET TAG <tag_name> = '<tag_value>'
//
// Identity fields (tagName/tagRef, objectType, objectName) are immutable
// after creation — changing any of them requires deleting and recreating
// the resource. Only tagValue may be updated in place.
//
// +kubebuilder:validation:XValidation:rule="has(oldSelf.tagName) == has(self.tagName) && (!has(self.tagName) || self.tagName == oldSelf.tagName)",message="spec.tagName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.tagRef) == has(self.tagRef) && (!has(self.tagRef) || self.tagRef == oldSelf.tagRef)",message="spec.tagRef is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.objectType == oldSelf.objectType",message="spec.objectType is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.objectName == oldSelf.objectName",message="spec.objectName is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
//
// Mutual exclusivity rules:
// +kubebuilder:validation:XValidation:rule="(has(self.tagName) ? 1 : 0) + (has(self.tagRef) ? 1 : 0) == 1",message="exactly one of spec.tagName or spec.tagRef must be set"
type TagAssociationSpec struct {
	CommonSpec `json:",inline"`

	// TagName is the fully qualified tag name (e.g. "MY_DB"."MY_SCHEMA"."MY_TAG").
	// Mutually exclusive with TagRef.
	// +optional
	TagName string `json:"tagName,omitempty"`

	// TagRef references a Tag CR in the same namespace.
	// When set, the tag name is resolved from the CR's fullyQualifiedName.
	// Mutually exclusive with TagName.
	// +optional
	TagRef *LocalObjectReference `json:"tagRef,omitempty"`

	// TagValue is the string value to assign to the tag on the object.
	// This is the only mutable field — changing it triggers an ALTER SET TAG
	// with the new value.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	TagValue string `json:"tagValue"`

	// ObjectType is the Snowflake object type to tag (e.g. "TABLE", "WAREHOUSE", "DATABASE").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=ACCOUNT;DATABASE;SCHEMA;TABLE;VIEW;COLUMN;WAREHOUSE;ROLE;USER;STAGE;STREAM;TASK;ALERT;PIPE;FUNCTION;PROCEDURE;INTEGRATION;"NETWORK POLICY";"DATABASE ROLE"
	ObjectType string `json:"objectType"`

	// ObjectName is the fully qualified Snowflake object name to tag
	// (e.g. "MY_DB"."MY_SCHEMA"."MY_TABLE").
	// Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	ObjectName string `json:"objectName"`
}

// TagAssociationObservedValue contains the observed tag value from Snowflake.
type TagAssociationObservedValue struct {
	// TagValue is the tag value currently set on the object in Snowflake,
	// as returned by SYSTEM$GET_TAG.
	TagValue string `json:"tagValue,omitempty"`
}

// TagAssociationStatus defines the observed state of a TagAssociation.
type TagAssociationStatus struct {
	CommonStatus `json:",inline"`

	// TagName is the resolved fully qualified tag name.
	TagName string `json:"tagName,omitempty"`

	// ObservedValue contains the tag value observed from Snowflake.
	ObservedValue *TagAssociationObservedValue `json:"observedValue,omitempty"`
}

// TagAssociation is the Schema for the tagassociations API.
// It associates a Snowflake tag with an object, setting a tag value.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ta,categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="TAG",type=string,JSONPath=`.status.tagName`,priority=0
// +kubebuilder:printcolumn:name="OBJECT-TYPE",type=string,JSONPath=`.spec.objectType`,priority=0
// +kubebuilder:printcolumn:name="OBJECT",type=string,JSONPath=`.spec.objectName`,priority=0
// +kubebuilder:printcolumn:name="VALUE",type=string,JSONPath=`.spec.tagValue`,priority=0
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type TagAssociation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TagAssociationSpec   `json:"spec,omitempty"`
	Status TagAssociationStatus `json:"status,omitempty"`
}

// TagAssociationList contains a list of TagAssociation.
// +kubebuilder:object:root=true
type TagAssociationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TagAssociation `json:"items"`
}

// GetSpecName returns a human-readable composite name for the tag association.
func (r *TagAssociation) GetSpecName() string {
	tag := r.Spec.TagName
	if tag == "" && r.Spec.TagRef != nil {
		tag = "(ref: " + r.Spec.TagRef.Name + ")"
	}

	return fmt.Sprintf("TAG %s ON %s %s = %q", tag, r.Spec.ObjectType, r.Spec.ObjectName, r.Spec.TagValue)
}

func init() {
	SchemeBuilder.Register(&TagAssociation{}, &TagAssociationList{})
}
