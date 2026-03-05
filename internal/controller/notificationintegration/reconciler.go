// Package notificationintegration implements the reconciler for NotificationIntegration resources.
package notificationintegration

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
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
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewNotificationIntegrationClient(exec)
		}),
	)
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
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for NotificationIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.NotificationIntegration, Service, *snowflake.NotificationIntegrationObservation]{
		ResourceNameVal:  "notificationintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.NotificationIntegration { return &snowplanev1alpha1.NotificationIntegration{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.NotificationIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.NotificationIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.NotificationIntegrationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.NotificationIntegration, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterNotificationIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.NotificationIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.NotificationIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.NotificationIntegration, obs *reconciler.Observation[*snowflake.NotificationIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.NotificationIntegration, obs *reconciler.Observation[*snowflake.NotificationIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not been changed.
func validateImmutableFields(_ context.Context, ni *snowplanev1alpha1.NotificationIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(ni) {
		return nil
	}

	if ni.Status.ShowOutput != nil {
		if ni.Status.ShowOutput.Name != "" && !strings.EqualFold(ni.Spec.Name, ni.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ni.Status.ShowOutput.Name, ni.Spec.Name)
		}

		if ni.Status.ShowOutput.Type != "" && !strings.EqualFold(string(ni.Spec.Type), ni.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", ni.Status.ShowOutput.Type, ni.Spec.Type)
		}
	}

	return nil
}

func applyObservation(ni *snowplanev1alpha1.NotificationIntegration, obs *snowflake.NotificationIntegrationObservation) {
	if obs.ShowOutput != nil {
		ni.Status.FullyQualifiedName = obs.ShowOutput.Name

		ni.Status.ShowOutput = obs.ShowOutput
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
	opts.UnsetFields = tracked.ComputeUnset(&ni.Spec, ni.Status.TrackedParameters)

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
