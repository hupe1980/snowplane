package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebhookNotificationIntegrationSpec defines the desired state of a Snowflake Webhook Notification Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
type WebhookNotificationIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake notification integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// WebhookURL is the URL endpoint for the webhook.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	WebhookURL string `json:"webhookURL" snowflake:"WEBHOOK_URL,always,nounset"`

	// WebhookSecret is the secret used to authenticate with the webhook.
	// +optional
	WebhookSecret *string `json:"webhookSecret,omitempty" snowflake:"WEBHOOK_SECRET"`

	// WebhookBodyTemplate is the template for the webhook body.
	// +optional
	WebhookBodyTemplate *string `json:"webhookBodyTemplate,omitempty" snowflake:"WEBHOOK_BODY_TEMPLATE"`

	// WebhookHeaders are additional HTTP headers sent with the webhook request.
	// +optional
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty" snowflake:"WEBHOOK_HEADERS,prefix"`

	// Comment is an optional description for the notification integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// WebhookNotificationIntegrationShowOutput mirrors the SHOW NOTIFICATION INTEGRATIONS output.
type WebhookNotificationIntegrationShowOutput struct {
	CreatedOn string `json:"createdOn,omitempty"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Category  string `json:"category,omitempty"`
	Enabled   bool   `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`
	Comment   string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// WebhookNotificationIntegrationStatus defines the observed state of a WebhookNotificationIntegration.
type WebhookNotificationIntegrationStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *WebhookNotificationIntegrationShowOutput `json:"showOutput,omitempty"`
	DescribeOutput    map[string]string                         `json:"describeOutput,omitempty"`
	TrackedParameters []string                                  `json:"trackedParameters,omitempty"`
}

// WebhookNotificationIntegration is the Schema for the webhooknotificationintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=webhooknotif
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type WebhookNotificationIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebhookNotificationIntegrationSpec   `json:"spec,omitempty"`
	Status WebhookNotificationIntegrationStatus `json:"status,omitempty"`
}

// WebhookNotificationIntegrationList contains a list of WebhookNotificationIntegration.
// +kubebuilder:object:root=true
type WebhookNotificationIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebhookNotificationIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebhookNotificationIntegration{}, &WebhookNotificationIntegrationList{})
}
