// Package emailnotificationintegration implements the reconciler for EmailNotificationIntegration resources.
package emailnotificationintegration

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
	finalizerName = "snowplane.hupe1980.github.io/emailnotificationintegration"
)

// SnowflakeClient is an alias for the client factory's SnowflakeClient type.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines the Snowflake operations for email notification integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.EmailNotificationIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateEmailNotificationIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterEmailNotificationIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewEmailNotificationIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for EmailNotificationIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.EmailNotificationIntegration, Service, *snowflake.EmailNotificationIntegrationObservation]{
		ResourceNameVal:  "emailnotificationintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.EmailNotificationIntegration {
			return &snowplanev1alpha1.EmailNotificationIntegration{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.EmailNotificationIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.EmailNotificationIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.EmailNotificationIntegrationObservation) bool {
				return obs.Exists
			},
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.EmailNotificationIntegration, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterEmailNotificationIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.EmailNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.EmailNotificationIntegration, obs *reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.EmailNotificationIntegration, obs *reconciler.Observation[*snowflake.EmailNotificationIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.EmailNotificationIntegration) error {
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

func applyObservation(obj *snowplanev1alpha1.EmailNotificationIntegration, obs *snowflake.EmailNotificationIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.EmailNotificationIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateEmailNotificationIntegrationOptions {
	return snowflake.CreateEmailNotificationIntegrationOptions{
		Name:              id,
		Enabled:           obj.Spec.Enabled,
		AllowedRecipients: obj.Spec.AllowedRecipients,
		DefaultRecipients: obj.Spec.DefaultRecipients,
		DefaultSubject:    obj.Spec.DefaultSubject,
		Comment:           obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.EmailNotificationIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.EmailNotificationIntegrationObservation) snowflake.AlterEmailNotificationIntegrationOptions {
	opts := snowflake.AlterEmailNotificationIntegrationOptions{
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

	if len(obj.Spec.AllowedRecipients) > 0 {
		list := make([]string, len(obj.Spec.AllowedRecipients))
		copy(list, obj.Spec.AllowedRecipients)
		opts.AllowedRecipients = &list
	}

	if len(obj.Spec.DefaultRecipients) > 0 {
		list := make([]string, len(obj.Spec.DefaultRecipients))
		copy(list, obj.Spec.DefaultRecipients)
		opts.DefaultRecipients = &list
	}

	opts.DefaultSubject = obj.Spec.DefaultSubject

	return opts
}

func detectDrift(obj *snowplanev1alpha1.EmailNotificationIntegration, obs *snowflake.EmailNotificationIntegrationObservation) *drift.Result {
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
		compareListFromDescribe(d, "ALLOWED_RECIPIENTS", obj.Spec.AllowedRecipients, obs.DescribeOutput)
		compareListFromDescribe(d, "DEFAULT_RECIPIENTS", obj.Spec.DefaultRecipients, obs.DescribeOutput)
		d.CompareStringValue("DEFAULT_SUBJECT", stringValueOrEmpty(obj.Spec.DefaultSubject), describeValue(obs.DescribeOutput, "DEFAULT_SUBJECT"), false)
	}

	return d.Result()
}

func describeValue(desc map[string]string, key string) string {
	if desc == nil {
		return ""
	}

	return desc[key]
}

func stringValueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func compareListFromDescribe(d *drift.Detector, key string, specList []string, desc map[string]string) {
	descList := parseCommaList(desc, key)
	specJoined := strings.Join(specList, ",")
	descJoined := strings.Join(descList, ",")
	d.CompareStringValueFold(key, specJoined, descJoined, false)
}

func parseCommaList(desc map[string]string, key string) []string {
	if desc == nil {
		return nil
	}

	raw, ok := desc[key]
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
