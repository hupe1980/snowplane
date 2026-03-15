// Package apiintegration implements the reconciler for APIIntegration resources.
package apiintegration

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/helpers"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/apiintegration"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake API integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAPIIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAPIIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new APIIntegration reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.APIIntegration, Service, *snowflake.APIIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewAPIIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.APIIntegration, Service, *snowflake.APIIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for APIIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.APIIntegration, Service, *snowflake.APIIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.APIIntegration, Service, *snowflake.APIIntegrationObservation]{
		ResourceNameVal:  "apiintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.APIIntegration { return &snowplanev1alpha1.APIIntegration{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.APIIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.APIIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.APIIntegrationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.APIIntegration, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterAPIIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.APIIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.APIIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.APIIntegration, obs *reconciler.Observation[*snowflake.APIIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.APIIntegration, obs *reconciler.Observation[*snowflake.APIIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.APIIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.APIIntegration, obs *snowflake.APIIntegrationObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.APIIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateAPIIntegrationOptions {
	return snowflake.CreateAPIIntegrationOptions{
		Name:               id,
		APIProvider:        obj.Spec.APIProvider,
		Enabled:            obj.Spec.Enabled,
		APIAllowedPrefixes: obj.Spec.APIAllowedPrefixes,
		APIBlockedPrefixes: obj.Spec.APIBlockedPrefixes,
		APIAWSRoleARN:      obj.Spec.APIAWSRoleARN,
		AzureTenantID:      obj.Spec.AzureTenantID,
		AzureADAppID:       obj.Spec.AzureADApplicationID,
		GoogleAudience:     obj.Spec.GoogleAudience,
		APIKey:             obj.Spec.APIKey,
		Comment:            obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.APIIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.APIIntegrationObservation) snowflake.AlterAPIIntegrationOptions {
	opts := snowflake.AlterAPIIntegrationOptions{
		Name: id,
	}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Compare Enabled against observed value.
	if obj.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = obj.Spec.Enabled
		}
	}

	// Comment - only set if changed.
	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	if obs != nil && obs.DescribeOutput != nil {
		dm := obs.DescribeOutput

		// API_ALLOWED_PREFIXES — diff-check.
		actualAllowed := helpers.ParseCommaListFromMap(dm, "API_ALLOWED_PREFIXES")
		if !helpers.StringSlicesEqualFold(obj.Spec.APIAllowedPrefixes, actualAllowed) {
			if len(obj.Spec.APIAllowedPrefixes) > 0 {
				list := make([]string, len(obj.Spec.APIAllowedPrefixes))
				copy(list, obj.Spec.APIAllowedPrefixes)
				opts.APIAllowedPrefixes = &list
			}
		}

		// API_BLOCKED_PREFIXES — diff-check.
		actualBlocked := helpers.ParseCommaListFromMap(dm, "API_BLOCKED_PREFIXES")
		if !helpers.StringSlicesEqualFold(obj.Spec.APIBlockedPrefixes, actualBlocked) {
			if len(obj.Spec.APIBlockedPrefixes) > 0 {
				list := make([]string, len(obj.Spec.APIBlockedPrefixes))
				copy(list, obj.Spec.APIBlockedPrefixes)
				opts.APIBlockedPrefixes = &list
			}
		}

		// Provider-specific fields — diff-check.
		if obj.Spec.APIAWSRoleARN != nil && *obj.Spec.APIAWSRoleARN != helpers.DescribeValue(dm, "API_AWS_ROLE_ARN") {
			opts.APIAWSRoleARN = obj.Spec.APIAWSRoleARN
		}

		if obj.Spec.AzureADApplicationID != nil && *obj.Spec.AzureADApplicationID != helpers.DescribeValue(dm, "AZURE_AD_APPLICATION_ID") {
			opts.AzureADAppID = obj.Spec.AzureADApplicationID
		}

		if obj.Spec.APIKey != nil && *obj.Spec.APIKey != helpers.DescribeValue(dm, "API_KEY") {
			opts.APIKey = obj.Spec.APIKey
		}
	} else {
		// No observation — send everything unconditionally.
		if len(obj.Spec.APIAllowedPrefixes) > 0 {
			list := make([]string, len(obj.Spec.APIAllowedPrefixes))
			copy(list, obj.Spec.APIAllowedPrefixes)
			opts.APIAllowedPrefixes = &list
		}

		if len(obj.Spec.APIBlockedPrefixes) > 0 {
			list := make([]string, len(obj.Spec.APIBlockedPrefixes))
			copy(list, obj.Spec.APIBlockedPrefixes)
			opts.APIBlockedPrefixes = &list
		}

		opts.APIAWSRoleARN = obj.Spec.APIAWSRoleARN
		opts.AzureADAppID = obj.Spec.AzureADApplicationID
		opts.APIKey = obj.Spec.APIKey
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.APIIntegration, obs *snowflake.APIIntegrationObservation) *drift.Result {
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
		d.CompareStringValue("API_AWS_ROLE_ARN", helpers.StringValueOrEmpty(obj.Spec.APIAWSRoleARN), helpers.DescribeValue(obs.DescribeOutput, "API_AWS_ROLE_ARN"), false)
		d.CompareStringValue("AZURE_AD_APPLICATION_ID", helpers.StringValueOrEmpty(obj.Spec.AzureADApplicationID), helpers.DescribeValue(obs.DescribeOutput, "AZURE_AD_APPLICATION_ID"), false)
		d.CompareStringValue("AZURE_TENANT_ID", helpers.StringValueOrEmpty(obj.Spec.AzureTenantID), helpers.DescribeValue(obs.DescribeOutput, "AZURE_TENANT_ID"), true)
		d.CompareStringValue("GOOGLE_AUDIENCE", helpers.StringValueOrEmpty(obj.Spec.GoogleAudience), helpers.DescribeValue(obs.DescribeOutput, "GOOGLE_AUDIENCE"), true)

		helpers.CompareListFromDescribeMap(d, "API_ALLOWED_PREFIXES", obj.Spec.APIAllowedPrefixes, obs.DescribeOutput, false)
		helpers.CompareListFromDescribeMap(d, "API_BLOCKED_PREFIXES", obj.Spec.APIBlockedPrefixes, obs.DescribeOutput, false)
	}

	return d.Result()
}
