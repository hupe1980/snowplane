package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NotificationIntegrationType specifies the type of notification integration.
// +kubebuilder:validation:Enum=EMAIL;QUEUE;WEBHOOK
type NotificationIntegrationType string

// Valid NotificationIntegrationType values.
const (
	NotificationIntegrationTypeEmail   NotificationIntegrationType = "EMAIL"
	NotificationIntegrationTypeQueue   NotificationIntegrationType = "QUEUE"
	NotificationIntegrationTypeWebhook NotificationIntegrationType = "WEBHOOK"
)

// EmailNotificationConfig holds configuration for EMAIL notification integrations.
type EmailNotificationConfig struct {
	// AllowedRecipients is the list of email addresses that can receive notifications.
	// +kubebuilder:validation:MinItems=1
	AllowedRecipients []string `json:"allowedRecipients" snowflake:"ALLOWED_RECIPIENTS,nounset"`

	// DefaultRecipients is the default list of email recipients.
	// +optional
	DefaultRecipients []string `json:"defaultRecipients,omitempty" snowflake:"DEFAULT_RECIPIENTS"`

	// DefaultSubject is the default email subject line.
	// +optional
	DefaultSubject *string `json:"defaultSubject,omitempty" snowflake:"DEFAULT_SUBJECT"`
}

// QueueNotificationConfig holds configuration for QUEUE (cloud messaging) notification integrations.
type QueueNotificationConfig struct {
	// NotificationProvider is the cloud provider for the queue.
	// +kubebuilder:validation:Enum=AWS_SNS;AWS_SQS;GCP_PUBSUB;AZURE_STORAGE_QUEUE;AZURE_EVENT_GRID
	NotificationProvider string `json:"notificationProvider" snowflake:"NOTIFICATION_PROVIDER,always,nounset"`

	// Direction is the message direction (OUTBOUND or INBOUND).
	// +kubebuilder:validation:Enum=OUTBOUND;INBOUND
	// +kubebuilder:default=OUTBOUND
	Direction string `json:"direction"`

	// AWSSNSTopicARN is the ARN of the SNS topic (required for AWS_SNS).
	// +optional
	AWSSNSTopicARN *string `json:"awsSNSTopicARN,omitempty" snowflake:"AWS_SNS_TOPIC_ARN"`

	// AWSSNSRoleARN is the ARN of the IAM role for SNS access (required for AWS_SNS).
	// +optional
	AWSSNSRoleARN *string `json:"awsSNSRoleARN,omitempty" snowflake:"AWS_SNS_ROLE_ARN"`

	// AWSSQSArn is the ARN of the SQS queue (required for AWS_SQS).
	// +optional
	AWSSQSArn *string `json:"awsSQSArn,omitempty" snowflake:"AWS_SQS_ARN"`

	// AWSSQSRoleARN is the ARN of the IAM role for SQS access (required for AWS_SQS).
	// +optional
	AWSSQSRoleARN *string `json:"awsSQSRoleARN,omitempty" snowflake:"AWS_SQS_ROLE_ARN"`

	// GCPPubSubTopicName is the Pub/Sub topic name (required for GCP_PUBSUB).
	// +optional
	GCPPubSubTopicName *string `json:"gcpPubSubTopicName,omitempty" snowflake:"GCP_PUBSUB_TOPIC_NAME"`

	// GCPPubSubSubscriptionName is the Pub/Sub subscription name (optional for GCP_PUBSUB).
	// +optional
	GCPPubSubSubscriptionName *string `json:"gcpPubSubSubscriptionName,omitempty" snowflake:"GCP_PUBSUB_SUBSCRIPTION_NAME"`

	// AzureStorageQueuePrimaryURI is the Azure Storage queue endpoint (required for AZURE_STORAGE_QUEUE).
	// +optional
	AzureStorageQueuePrimaryURI *string `json:"azureStorageQueuePrimaryURI,omitempty" snowflake:"AZURE_STORAGE_QUEUE_PRIMARY_URI"`

	// AzureTenantID is the Azure AD tenant ID (required for Azure providers).
	// +optional
	AzureTenantID *string `json:"azureTenantID,omitempty" snowflake:"AZURE_TENANT_ID"`

	// AzureEventGridTopicEndpoint is the Event Grid topic endpoint (required for AZURE_EVENT_GRID).
	// +optional
	AzureEventGridTopicEndpoint *string `json:"azureEventGridTopicEndpoint,omitempty" snowflake:"AZURE_EVENT_GRID_TOPIC_ENDPOINT"`
}

// WebhookNotificationConfig holds configuration for WEBHOOK notification integrations.
type WebhookNotificationConfig struct {
	// WebhookURL is the endpoint URL for the webhook.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	WebhookURL string `json:"webhookURL" snowflake:"WEBHOOK_URL,always,nounset"`

	// WebhookSecret is the secret used to sign webhook payloads.
	// +optional
	WebhookSecret *string `json:"webhookSecret,omitempty" snowflake:"WEBHOOK_SECRET"`

	// WebhookBodyTemplate is a custom body template for the webhook payload.
	// +optional
	WebhookBodyTemplate *string `json:"webhookBodyTemplate,omitempty" snowflake:"WEBHOOK_BODY_TEMPLATE"`

	// WebhookHeaders is a map of custom HTTP headers to include in webhook requests.
	// +optional
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty" snowflake:"WEBHOOK_HEADER_,prefix"`
}

// NotificationIntegrationSpec defines the desired state of a Snowflake Notification Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="spec.type is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.type != 'EMAIL' || has(self.email)",message="spec.email is required when type is EMAIL"
// +kubebuilder:validation:XValidation:rule="self.type != 'QUEUE' || has(self.queue)",message="spec.queue is required when type is QUEUE"
// +kubebuilder:validation:XValidation:rule="self.type != 'WEBHOOK' || has(self.webhook)",message="spec.webhook is required when type is WEBHOOK"
type NotificationIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake notification integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Type specifies the integration type (EMAIL, QUEUE, WEBHOOK).
	// Immutable after creation.
	Type NotificationIntegrationType `json:"type"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Email holds configuration for EMAIL integrations.
	// +optional
	Email *EmailNotificationConfig `json:"email,omitempty"`

	// Queue holds configuration for QUEUE integrations.
	// +optional
	Queue *QueueNotificationConfig `json:"queue,omitempty"`

	// Webhook holds configuration for WEBHOOK integrations.
	// +optional
	Webhook *WebhookNotificationConfig `json:"webhook,omitempty"`

	// Comment is an optional description for the notification integration.
	// +optional
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// NotificationIntegrationShowOutput mirrors the SHOW NOTIFICATION INTEGRATIONS output stored in status.
type NotificationIntegrationShowOutput struct {
	// CreatedOn is the timestamp when the integration was created.
	CreatedOn string `json:"createdOn,omitempty"`

	// Name is the integration name as returned by Snowflake.
	Name string `json:"name,omitempty"`

	// Type is the integration type.
	Type string `json:"type,omitempty"`

	// Category is the integration category (NOTIFICATION).
	Category string `json:"category,omitempty"`

	// Enabled indicates whether the integration is active.
	Enabled bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// Comment is the integration description.
	Comment string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// NotificationIntegrationStatus defines the observed state of a NotificationIntegration.
type NotificationIntegrationStatus struct {
	CommonStatus `json:",inline"`

	// ShowOutput contains the raw SHOW NOTIFICATION INTEGRATIONS output.
	ShowOutput *NotificationIntegrationShowOutput `json:"showOutput,omitempty"`

	// DescribeOutput is a map of DESCRIBE INTEGRATION key-value pairs.
	DescribeOutput map[string]string `json:"describeOutput,omitempty"`

	// TrackedParameters tracks which optional spec fields have been actively SET.
	TrackedParameters []string `json:"trackedParameters,omitempty"`
}

// NotificationIntegration is the Schema for the notificationintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type NotificationIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NotificationIntegrationSpec   `json:"spec,omitempty"`
	Status NotificationIntegrationStatus `json:"status,omitempty"`
}

// NotificationIntegrationList contains a list of NotificationIntegration.
// +kubebuilder:object:root=true
type NotificationIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NotificationIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NotificationIntegration{}, &NotificationIntegrationList{})
}
