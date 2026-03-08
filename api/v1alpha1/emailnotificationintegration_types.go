package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EmailNotificationIntegrationSpec defines the desired state of a Snowflake Email Notification Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type EmailNotificationIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake notification integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// AllowedRecipients is the list of email addresses that can receive notifications.
	// +kubebuilder:validation:MinItems=1
	AllowedRecipients []string `json:"allowedRecipients" snowflake:"ALLOWED_RECIPIENTS,nounset"`

	// DefaultRecipients is the default list of email recipients.
	// +optional
	DefaultRecipients []string `json:"defaultRecipients,omitempty" snowflake:"DEFAULT_RECIPIENTS"`

	// DefaultSubject is the default email subject line.
	// +optional
	DefaultSubject *string `json:"defaultSubject,omitempty" snowflake:"DEFAULT_SUBJECT"`

	// Comment is an optional description for the notification integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// EmailNotificationIntegrationShowOutput mirrors the SHOW NOTIFICATION INTEGRATIONS output.
type EmailNotificationIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`
	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`
	// Type is the integration type.
	Type string `json:"type,omitempty"`
	// Category is the integration category.
	Category string `json:"category,omitempty"`
	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`
	// Comment is the integration description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// EmailNotificationIntegrationStatus defines the observed state of an EmailNotificationIntegration.
type EmailNotificationIntegrationStatus struct {
	CommonStatus `json:",inline"`
	// ShowOutput contains the raw SHOW NOTIFICATION INTEGRATIONS output.
	ShowOutput *EmailNotificationIntegrationShowOutput `json:"showOutput,omitempty"`
	// DescribeOutput is a map of DESCRIBE INTEGRATION key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`
	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// EmailNotificationIntegration is the Schema for the emailnotificationintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=emailnotif
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type EmailNotificationIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmailNotificationIntegrationSpec   `json:"spec,omitempty"`
	Status EmailNotificationIntegrationStatus `json:"status,omitempty"`
}

// EmailNotificationIntegrationList contains a list of EmailNotificationIntegration.
// +kubebuilder:object:root=true
type EmailNotificationIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EmailNotificationIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EmailNotificationIntegration{}, &EmailNotificationIntegrationList{})
}
