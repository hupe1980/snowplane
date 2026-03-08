package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QueueNotificationIntegrationSpec defines the desired state of a Snowflake Queue Notification Integration.
//
// +kubebuilder:validation:XValidation:rule="self.name == oldSelf.name",message="spec.name is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="has(oldSelf.useRole) == has(self.useRole) && (!has(self.useRole) || self.useRole == oldSelf.useRole)",message="spec.useRole is immutable (delete and recreate the resource to change)"
// +kubebuilder:validation:XValidation:rule="self.notificationProvider == oldSelf.notificationProvider",message="spec.notificationProvider is immutable (delete and recreate the resource to change)"
type QueueNotificationIntegrationSpec struct {
	CommonSpec `json:",inline"`

	// Name is the Snowflake notification integration name. Immutable after creation.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	Name string `json:"name"`

	// Enabled controls whether the integration is active.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`

	// NotificationProvider is the cloud provider for the queue.
	// Immutable after creation.
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

	// Comment is an optional description for the notification integration.
	// +optional
	// +kubebuilder:validation:MaxLength=10000
	Comment *string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// QueueNotificationIntegrationShowOutput mirrors the SHOW NOTIFICATION INTEGRATIONS output.
type QueueNotificationIntegrationShowOutput struct {
	CreatedOn string `json:"createdOn,omitempty"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Category  string `json:"category,omitempty"`
	Enabled   bool   `json:"enabled,omitempty" snowflake:"ENABLED,nounset"`
	Comment   string `json:"comment,omitempty" snowflake:"COMMENT"`
}

// QueueNotificationIntegrationStatus defines the observed state of a QueueNotificationIntegration.
type QueueNotificationIntegrationStatus struct {
	CommonStatus      `json:",inline"`
	ShowOutput        *QueueNotificationIntegrationShowOutput `json:"showOutput,omitempty"`
	DescribeOutput    map[string]string                       `json:"describeOutput,omitempty"`
	TrackedParameters []string                                `json:"trackedParameters,omitempty"`
}

// QueueNotificationIntegration is the Schema for the queuenotificationintegrations API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=snowplane,shortName=queuenotif
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="SYNCED",type=string,JSONPath=`.status.conditions[?(@.type=="Synced")].status`
// +kubebuilder:printcolumn:name="SNOWFLAKE-NAME",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="NOTIFICATION-PROVIDER",type=string,JSONPath=`.spec.notificationProvider`
// +kubebuilder:printcolumn:name="PROVIDER",type=string,JSONPath=`.spec.providerRef.name`,priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`
type QueueNotificationIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QueueNotificationIntegrationSpec   `json:"spec,omitempty"`
	Status QueueNotificationIntegrationStatus `json:"status,omitempty"`
}

// QueueNotificationIntegrationList contains a list of QueueNotificationIntegration.
// +kubebuilder:object:root=true
type QueueNotificationIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []QueueNotificationIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&QueueNotificationIntegration{}, &QueueNotificationIntegrationList{})
}
