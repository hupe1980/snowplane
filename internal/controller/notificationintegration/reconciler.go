// Package notificationintegration implements the reconciler for NotificationIntegration resources.
package notificationintegration

import (
	"context"
	"sort"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/notificationintegration"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake notification integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateNotificationIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterNotificationIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new NotificationIntegration reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation] {
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation] {
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewNotificationIntegrationClient(sfC), cleanup, nil
}

func applyObservation(ni *snowplanev1alpha1.NotificationIntegration, obs *snowflake.NotificationIntegrationObservation) {
	if obs.ShowOutput != nil {
		ni.Status.FullyQualifiedName = obs.ShowOutput.Name

		ni.Status.ShowOutput = &snowplanev1alpha1.NotificationIntegrationShowOutput{
			CreatedOn: obs.ShowOutput.CreatedOn,
			Name:      obs.ShowOutput.Name,
			Type:      obs.ShowOutput.Type,
			Category:  obs.ShowOutput.Category,
			Enabled:   obs.ShowOutput.Enabled,
			Comment:   obs.ShowOutput.Comment,
		}
	}

	if obs.DescribeOutput != nil {
		ni.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(ni *snowplanev1alpha1.NotificationIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateNotificationIntegrationOptions {
	opts := snowflake.CreateNotificationIntegrationOptions{
		Name:    id,
		Type:    string(ni.Spec.Type),
		Enabled: ni.Spec.Enabled,
		Comment: ni.Spec.Comment,
	}

	switch ni.Spec.Type {
	case snowplanev1alpha1.NotificationIntegrationTypeEmail:
		if c := ni.Spec.Email; c != nil {
			opts.AllowedRecipients = c.AllowedRecipients
			opts.DefaultRecipients = c.DefaultRecipients
			opts.DefaultSubject = c.DefaultSubject
		}
	case snowplanev1alpha1.NotificationIntegrationTypeQueue:
		if c := ni.Spec.Queue; c != nil {
			opts.NotificationProvider = &c.NotificationProvider
			opts.Direction = &c.Direction
			opts.AWSSNSTopicARN = c.AWSSNSTopicARN
			opts.AWSSNSRoleARN = c.AWSSNSRoleARN
			opts.AWSSQSArn = c.AWSSQSArn
			opts.AWSSQSRoleARN = c.AWSSQSRoleARN
			opts.GCPPubSubTopicName = c.GCPPubSubTopicName
			opts.GCPPubSubSubscriptionName = c.GCPPubSubSubscriptionName
			opts.AzureStorageQueuePrimaryURI = c.AzureStorageQueuePrimaryURI
			opts.AzureTenantID = c.AzureTenantID
			opts.AzureEventGridTopicEndpoint = c.AzureEventGridTopicEndpoint
		}
	case snowplanev1alpha1.NotificationIntegrationTypeWebhook:
		if c := ni.Spec.Webhook; c != nil {
			opts.WebhookURL = &c.WebhookURL
			opts.WebhookSecret = c.WebhookSecret
			opts.WebhookBodyTemplate = c.WebhookBodyTemplate
			opts.WebhookHeaders = c.WebhookHeaders
		}
	}

	return opts
}

func buildAlterOptions(ni *snowplanev1alpha1.NotificationIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.NotificationIntegrationObservation) snowflake.AlterNotificationIntegrationOptions {
	opts := snowflake.AlterNotificationIntegrationOptions{
		Name: id,
		Type: string(ni.Spec.Type),
	}
	opts.UnsetFields = computeUnsetFields(ni)

	// Compare Enabled and Comment against observed values.
	if ni.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *ni.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = ni.Spec.Enabled
		}
	}

	if ni.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *ni.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = ni.Spec.Comment
		}
	}

	switch ni.Spec.Type {
	case snowplanev1alpha1.NotificationIntegrationTypeEmail:
		if c := ni.Spec.Email; c != nil {
			if len(c.AllowedRecipients) > 0 {
				list := make([]string, len(c.AllowedRecipients))
				copy(list, c.AllowedRecipients)
				opts.AllowedRecipients = &list
			}

			if len(c.DefaultRecipients) > 0 {
				list := make([]string, len(c.DefaultRecipients))
				copy(list, c.DefaultRecipients)
				opts.DefaultRecipients = &list
			}

			opts.DefaultSubject = c.DefaultSubject
		}
	case snowplanev1alpha1.NotificationIntegrationTypeWebhook:
		if c := ni.Spec.Webhook; c != nil {
			url := c.WebhookURL
			opts.WebhookURL = &url
			opts.WebhookSecret = c.WebhookSecret
			opts.WebhookBodyTemplate = c.WebhookBodyTemplate
			opts.WebhookHeaders = c.WebhookHeaders
		}
	case snowplanev1alpha1.NotificationIntegrationTypeQueue:
		if c := ni.Spec.Queue; c != nil {
			provider := c.NotificationProvider
			opts.NotificationProvider = &provider
			direction := c.Direction
			opts.Direction = &direction
			opts.AWSSNSTopicARN = c.AWSSNSTopicARN
			opts.AWSSNSRoleARN = c.AWSSNSRoleARN
			opts.AWSSQSArn = c.AWSSQSArn
			opts.AWSSQSRoleARN = c.AWSSQSRoleARN
			opts.GCPPubSubTopicName = c.GCPPubSubTopicName
			opts.GCPPubSubSubscriptionName = c.GCPPubSubSubscriptionName
			opts.AzureStorageQueuePrimaryURI = c.AzureStorageQueuePrimaryURI
			opts.AzureTenantID = c.AzureTenantID
			opts.AzureEventGridTopicEndpoint = c.AzureEventGridTopicEndpoint
		}
	}

	return opts
}

func computeUnsetFields(ni *snowplanev1alpha1.NotificationIntegration) []string {
	if len(ni.Status.TrackedParameters) == 0 {
		return nil
	}

	managed := make(map[string]bool, len(ni.Status.TrackedParameters))
	for _, f := range ni.Status.TrackedParameters {
		managed[f] = true
	}

	var unset []string

	if ni.Spec.Comment == nil && managed["COMMENT"] {
		unset = append(unset, "COMMENT")
	}

	switch ni.Spec.Type {
	case snowplanev1alpha1.NotificationIntegrationTypeEmail:
		if c := ni.Spec.Email; c != nil {
			if len(c.DefaultRecipients) == 0 && managed["DEFAULT_RECIPIENTS"] {
				unset = append(unset, "DEFAULT_RECIPIENTS")
			}

			if c.DefaultSubject == nil && managed["DEFAULT_SUBJECT"] {
				unset = append(unset, "DEFAULT_SUBJECT")
			}
		}
	case snowplanev1alpha1.NotificationIntegrationTypeWebhook:
		if c := ni.Spec.Webhook; c != nil {
			if c.WebhookSecret == nil && managed["WEBHOOK_SECRET"] {
				unset = append(unset, "WEBHOOK_SECRET")
			}

			if c.WebhookBodyTemplate == nil && managed["WEBHOOK_BODY_TEMPLATE"] {
				unset = append(unset, "WEBHOOK_BODY_TEMPLATE")
			}

			// Unset individually removed headers.
			for key := range managed {
				if strings.HasPrefix(key, "WEBHOOK_HEADER_") {
					headerKey := strings.TrimPrefix(key, "WEBHOOK_HEADER_")
					if _, ok := c.WebhookHeaders[headerKey]; !ok {
						unset = append(unset, key)
					}
				}
			}
		}
	case snowplanev1alpha1.NotificationIntegrationTypeQueue:
		if c := ni.Spec.Queue; c != nil {
			if c.AWSSNSTopicARN == nil && managed["AWS_SNS_TOPIC_ARN"] {
				unset = append(unset, "AWS_SNS_TOPIC_ARN")
			}

			if c.AWSSNSRoleARN == nil && managed["AWS_SNS_ROLE_ARN"] {
				unset = append(unset, "AWS_SNS_ROLE_ARN")
			}

			if c.AWSSQSArn == nil && managed["AWS_SQS_ARN"] {
				unset = append(unset, "AWS_SQS_ARN")
			}

			if c.AWSSQSRoleARN == nil && managed["AWS_SQS_ROLE_ARN"] {
				unset = append(unset, "AWS_SQS_ROLE_ARN")
			}

			if c.GCPPubSubTopicName == nil && managed["GCP_PUBSUB_TOPIC_NAME"] {
				unset = append(unset, "GCP_PUBSUB_TOPIC_NAME")
			}

			if c.GCPPubSubSubscriptionName == nil && managed["GCP_PUBSUB_SUBSCRIPTION_NAME"] {
				unset = append(unset, "GCP_PUBSUB_SUBSCRIPTION_NAME")
			}

			if c.AzureStorageQueuePrimaryURI == nil && managed["AZURE_STORAGE_QUEUE_PRIMARY_URI"] {
				unset = append(unset, "AZURE_STORAGE_QUEUE_PRIMARY_URI")
			}

			if c.AzureTenantID == nil && managed["AZURE_TENANT_ID"] {
				unset = append(unset, "AZURE_TENANT_ID")
			}

			if c.AzureEventGridTopicEndpoint == nil && managed["AZURE_EVENT_GRID_TOPIC_ENDPOINT"] {
				unset = append(unset, "AZURE_EVENT_GRID_TOPIC_ENDPOINT")
			}
		}
	}

	return unset
}

func computeTrackedParameters(spec *snowplanev1alpha1.NotificationIntegrationSpec) []string {
	var fields []string

	if spec.Enabled != nil {
		fields = append(fields, "ENABLED")
	}

	if spec.Comment != nil {
		fields = append(fields, "COMMENT")
	}

	switch spec.Type {
	case snowplanev1alpha1.NotificationIntegrationTypeEmail:
		if c := spec.Email; c != nil {
			if len(c.AllowedRecipients) > 0 {
				fields = append(fields, "ALLOWED_RECIPIENTS")
			}

			if len(c.DefaultRecipients) > 0 {
				fields = append(fields, "DEFAULT_RECIPIENTS")
			}

			if c.DefaultSubject != nil {
				fields = append(fields, "DEFAULT_SUBJECT")
			}
		}
	case snowplanev1alpha1.NotificationIntegrationTypeQueue:
		if c := spec.Queue; c != nil {
			fields = append(fields, "NOTIFICATION_PROVIDER")

			if c.AWSSNSTopicARN != nil {
				fields = append(fields, "AWS_SNS_TOPIC_ARN")
			}

			if c.AWSSNSRoleARN != nil {
				fields = append(fields, "AWS_SNS_ROLE_ARN")
			}

			if c.AWSSQSArn != nil {
				fields = append(fields, "AWS_SQS_ARN")
			}

			if c.AWSSQSRoleARN != nil {
				fields = append(fields, "AWS_SQS_ROLE_ARN")
			}

			if c.GCPPubSubTopicName != nil {
				fields = append(fields, "GCP_PUBSUB_TOPIC_NAME")
			}

			if c.GCPPubSubSubscriptionName != nil {
				fields = append(fields, "GCP_PUBSUB_SUBSCRIPTION_NAME")
			}

			if c.AzureStorageQueuePrimaryURI != nil {
				fields = append(fields, "AZURE_STORAGE_QUEUE_PRIMARY_URI")
			}

			if c.AzureTenantID != nil {
				fields = append(fields, "AZURE_TENANT_ID")
			}

			if c.AzureEventGridTopicEndpoint != nil {
				fields = append(fields, "AZURE_EVENT_GRID_TOPIC_ENDPOINT")
			}
		}
	case snowplanev1alpha1.NotificationIntegrationTypeWebhook:
		if c := spec.Webhook; c != nil {
			fields = append(fields, "WEBHOOK_URL")

			if c.WebhookSecret != nil {
				fields = append(fields, "WEBHOOK_SECRET")
			}

			if c.WebhookBodyTemplate != nil {
				fields = append(fields, "WEBHOOK_BODY_TEMPLATE")
			}

			// Track individual header keys for UNSET support.
			if len(c.WebhookHeaders) > 0 {
				keys := make([]string, 0, len(c.WebhookHeaders))
				for k := range c.WebhookHeaders {
					keys = append(keys, k)
				}

				sort.Strings(keys)

				for _, k := range keys {
					fields = append(fields, "WEBHOOK_HEADER_"+k)
				}
			}
		}
	}

	return fields
}

func detectDrift(ni *snowplanev1alpha1.NotificationIntegration, obs *snowflake.NotificationIntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", ni.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("TYPE", string(ni.Spec.Type), obs.ShowOutput.Type, true)

		// Mutable fields.
		d.CompareString("COMMENT", ni.Spec.Comment, obs.ShowOutput.Comment, false)

		if ni.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", ni.Spec.Enabled, &obsEnabled, false)
		}
	}

	// Compare sub-type fields from DESCRIBE output.
	if obs.DescribeOutput != nil {
		switch ni.Spec.Type {
		case snowplanev1alpha1.NotificationIntegrationTypeEmail:
			if c := ni.Spec.Email; c != nil {
				compareListFromDescribe(d, "ALLOWED_RECIPIENTS", c.AllowedRecipients, obs)
				compareListFromDescribe(d, "DEFAULT_RECIPIENTS", c.DefaultRecipients, obs)
				d.CompareStringValue("DEFAULT_SUBJECT", stringValueOrEmpty(c.DefaultSubject), describeValue(obs, "DEFAULT_SUBJECT"), false)
			}
		case snowplanev1alpha1.NotificationIntegrationTypeQueue:
			if c := ni.Spec.Queue; c != nil {
				d.CompareStringValueFold("NOTIFICATION_PROVIDER", c.NotificationProvider, describeValue(obs, "NOTIFICATION_PROVIDER"), false)
			}
		case snowplanev1alpha1.NotificationIntegrationTypeWebhook:
			if c := ni.Spec.Webhook; c != nil {
				d.CompareStringValue("WEBHOOK_URL", c.WebhookURL, describeValue(obs, "WEBHOOK_URL"), false)
			}
		}
	}

	return d.Result()
}

// describeValue extracts a DESCRIBE output value by key.
func describeValue(obs *snowflake.NotificationIntegrationObservation, key string) string {
	if obs.DescribeOutput == nil {
		return ""
	}

	return obs.DescribeOutput[key]
}

// stringValueOrEmpty returns the value of a string pointer, or "" if nil.
func stringValueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// compareListFromDescribe compares a spec list against a comma-separated DESCRIBE output value.
func compareListFromDescribe(d *drift.Detector, key string, specList []string, obs *snowflake.NotificationIntegrationObservation) {
	descList := parseCommaList(obs, key)
	specJoined := strings.Join(specList, ",")
	descJoined := strings.Join(descList, ",")
	d.CompareStringValueFold(key, specJoined, descJoined, false)
}

// parseCommaList extracts a list from DESCRIBE output by key.
func parseCommaList(obs *snowflake.NotificationIntegrationObservation, key string) []string {
	if obs.DescribeOutput == nil {
		return nil
	}

	raw, ok := obs.DescribeOutput[key]
	if !ok || raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}
