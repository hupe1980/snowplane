// Package queuenotificationintegration implements the reconciler for QueueNotificationIntegration resources.
package queuenotificationintegration

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
	finalizerName = "snowplane.hupe1980.github.io/queuenotificationintegration"
)

// SnowflakeClient is an alias for the client factory's SnowflakeClient type.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines the Snowflake operations for queue notification integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.QueueNotificationIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateQueueNotificationIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterQueueNotificationIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.QueueNotificationIntegration, Service, *snowflake.QueueNotificationIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewQueueNotificationIntegrationClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory creates a new reconciler with a custom service factory.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.QueueNotificationIntegration, Service, *snowflake.QueueNotificationIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for QueueNotificationIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.QueueNotificationIntegration, Service, *snowflake.QueueNotificationIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.QueueNotificationIntegration, Service, *snowflake.QueueNotificationIntegrationObservation]{
		ResourceNameVal:  "queuenotificationintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.QueueNotificationIntegration {
			return &snowplanev1alpha1.QueueNotificationIntegration{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.QueueNotificationIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.QueueNotificationIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.QueueNotificationIntegrationObservation) bool {
				return obs.Exists
			},
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.QueueNotificationIntegration, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterQueueNotificationIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.QueueNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.QueueNotificationIntegration, obs *reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.QueueNotificationIntegration, obs *reconciler.Observation[*snowflake.QueueNotificationIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.QueueNotificationIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Name != "" {
		if !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	if obj.Status.DescribeOutput != nil {
		if v, ok := obj.Status.DescribeOutput["NOTIFICATION_PROVIDER"]; ok && v != "" {
			if !strings.EqualFold(obj.Spec.NotificationProvider, v) {
				return fmt.Errorf("spec.notificationProvider is immutable after creation (current: %q, desired: %q)", v, obj.Spec.NotificationProvider)
			}
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.QueueNotificationIntegration, obs *snowflake.QueueNotificationIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.QueueNotificationIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateQueueNotificationIntegrationOptions {
	return snowflake.CreateQueueNotificationIntegrationOptions{
		Name:                        id,
		Enabled:                     obj.Spec.Enabled,
		NotificationProvider:        obj.Spec.NotificationProvider,
		Direction:                   obj.Spec.Direction,
		AWSSNSTopicARN:              obj.Spec.AWSSNSTopicARN,
		AWSSNSRoleARN:               obj.Spec.AWSSNSRoleARN,
		AWSSQSArn:                   obj.Spec.AWSSQSArn,
		AWSSQSRoleARN:               obj.Spec.AWSSQSRoleARN,
		GCPPubSubTopicName:          obj.Spec.GCPPubSubTopicName,
		GCPPubSubSubscriptionName:   obj.Spec.GCPPubSubSubscriptionName,
		AzureStorageQueuePrimaryURI: obj.Spec.AzureStorageQueuePrimaryURI,
		AzureTenantID:               obj.Spec.AzureTenantID,
		AzureEventGridTopicEndpoint: obj.Spec.AzureEventGridTopicEndpoint,
		Comment:                     obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.QueueNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.QueueNotificationIntegrationObservation) snowflake.AlterQueueNotificationIntegrationOptions {
	opts := snowflake.AlterQueueNotificationIntegrationOptions{
		Name: id,
	}

	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = obj.Spec.Enabled
		}
	}

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// Always send provider and direction on alter (they are part of a full reconciliation).
	provider := obj.Spec.NotificationProvider
	opts.NotificationProvider = &provider

	direction := obj.Spec.Direction
	opts.Direction = &direction

	// Cloud-provider-specific fields.
	opts.AWSSNSTopicARN = obj.Spec.AWSSNSTopicARN
	opts.AWSSNSRoleARN = obj.Spec.AWSSNSRoleARN
	opts.AWSSQSArn = obj.Spec.AWSSQSArn
	opts.AWSSQSRoleARN = obj.Spec.AWSSQSRoleARN
	opts.GCPPubSubTopicName = obj.Spec.GCPPubSubTopicName
	opts.GCPPubSubSubscriptionName = obj.Spec.GCPPubSubSubscriptionName
	opts.AzureStorageQueuePrimaryURI = obj.Spec.AzureStorageQueuePrimaryURI
	opts.AzureTenantID = obj.Spec.AzureTenantID
	opts.AzureEventGridTopicEndpoint = obj.Spec.AzureEventGridTopicEndpoint

	return opts
}

func detectDrift(obj *snowplanev1alpha1.QueueNotificationIntegration, obs *snowflake.QueueNotificationIntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)

		if obj.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", obj.Spec.Enabled, &obsEnabled, false)
		}
	}

	if obs.DescribeOutput != nil {
		d.CompareStringValueFold("NOTIFICATION_PROVIDER", obj.Spec.NotificationProvider, describeValue(obs.DescribeOutput, "NOTIFICATION_PROVIDER"), true)
	}

	return d.Result()
}

func describeValue(desc map[string]string, key string) string {
	if desc == nil {
		return ""
	}

	return desc[key]
}
