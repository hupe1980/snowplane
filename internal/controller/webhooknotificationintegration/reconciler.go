// Package webhooknotificationintegration implements the reconciler for WebhookNotificationIntegration resources.
package webhooknotificationintegration

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
	finalizerName = "snowplane.hupe1980.github.io/webhooknotificationintegration"
)

// SnowflakeClient is an alias for the client factory's SnowflakeClient type.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines the Snowflake operations for webhook notification integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.WebhookNotificationIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateWebhookNotificationIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterWebhookNotificationIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.WebhookNotificationIntegration, Service, *snowflake.WebhookNotificationIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewWebhookNotificationIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.WebhookNotificationIntegration, Service, *snowflake.WebhookNotificationIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for WebhookNotificationIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.WebhookNotificationIntegration, Service, *snowflake.WebhookNotificationIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.WebhookNotificationIntegration, Service, *snowflake.WebhookNotificationIntegrationObservation]{
		ResourceNameVal:  "webhooknotificationintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.WebhookNotificationIntegration {
			return &snowplanev1alpha1.WebhookNotificationIntegration{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.WebhookNotificationIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.WebhookNotificationIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.WebhookNotificationIntegrationObservation) bool {
				return obs.Exists
			},
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.WebhookNotificationIntegration, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterWebhookNotificationIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.WebhookNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.WebhookNotificationIntegration, obs *reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.WebhookNotificationIntegration, obs *reconciler.Observation[*snowflake.WebhookNotificationIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.WebhookNotificationIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Name != "" {
		if !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.WebhookNotificationIntegration, obs *snowflake.WebhookNotificationIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.WebhookNotificationIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateWebhookNotificationIntegrationOptions {
	return snowflake.CreateWebhookNotificationIntegrationOptions{
		Name:                id,
		Enabled:             obj.Spec.Enabled,
		WebhookURL:          obj.Spec.WebhookURL,
		WebhookSecret:       obj.Spec.WebhookSecret,
		WebhookBodyTemplate: obj.Spec.WebhookBodyTemplate,
		WebhookHeaders:      obj.Spec.WebhookHeaders,
		Comment:             obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.WebhookNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.WebhookNotificationIntegrationObservation) snowflake.AlterWebhookNotificationIntegrationOptions {
	opts := snowflake.AlterWebhookNotificationIntegrationOptions{
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

	// WebhookURL is always sent (it's a required nounset field).
	url := obj.Spec.WebhookURL
	opts.WebhookURL = &url

	opts.WebhookSecret = obj.Spec.WebhookSecret
	opts.WebhookBodyTemplate = obj.Spec.WebhookBodyTemplate
	opts.WebhookHeaders = obj.Spec.WebhookHeaders

	return opts
}

func detectDrift(obj *snowplanev1alpha1.WebhookNotificationIntegration, obs *snowflake.WebhookNotificationIntegrationObservation) *drift.Result {
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
		d.CompareStringValue("WEBHOOK_URL", obj.Spec.WebhookURL, describeValue(obs.DescribeOutput, "WEBHOOK_URL"), false)
	}

	return d.Result()
}

func describeValue(desc map[string]string, key string) string {
	if desc == nil {
		return ""
	}

	return desc[key]
}
