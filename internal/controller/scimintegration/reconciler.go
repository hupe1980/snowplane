// Package scimintegration implements the reconciler for SCIMIntegration resources.
package scimintegration

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
	finalizerName = "snowplane.hupe1980.github.io/scimintegration"
)

// SnowflakeClient is an alias for the client factory's SnowflakeClient type.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines the Snowflake operations for SCIM security integrations.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SCIMIntegrationObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSCIMIntegrationOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSCIMIntegrationOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler creates a new reconciler using the default service factory.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SCIMIntegration, Service, *snowflake.SCIMIntegrationObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSCIMIntegrationClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.SCIMIntegration, Service, *snowflake.SCIMIntegrationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for SCIMIntegration resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SCIMIntegration, Service, *snowflake.SCIMIntegrationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SCIMIntegration, Service, *snowflake.SCIMIntegrationObservation]{
		ResourceNameVal:  "scimintegration",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.SCIMIntegration {
			return &snowplanev1alpha1.SCIMIntegration{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SCIMIntegration) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.SCIMIntegrationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SCIMIntegrationObservation) bool {
				return obs.Exists
			},
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SCIMIntegration, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSCIMIntegrationOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SCIMIntegration, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.SCIMIntegrationObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SCIMIntegration, obs *reconciler.Observation[*snowflake.SCIMIntegrationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SCIMIntegration, obs *reconciler.Observation[*snowflake.SCIMIntegrationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

// validateImmutableFields checks that immutable fields have not been changed.
func validateImmutableFields(_ context.Context, sci *snowplanev1alpha1.SCIMIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(sci) {
		return nil
	}

	if sci.Status.ShowOutput != nil && sci.Status.ShowOutput.Name != "" {
		if !strings.EqualFold(sci.Spec.Name, sci.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", sci.Status.ShowOutput.Name, sci.Spec.Name)
		}
	}

	// SCIMClient and RunAsRole are validated by CEL rules on the CRD,
	// but we also guard them here for safety via DESCRIBE output.
	if sci.Status.DescribeOutput != nil {
		if v, ok := sci.Status.DescribeOutput["SCIM_CLIENT"]; ok && v != "" {
			if !strings.EqualFold(sci.Spec.SCIMClient, v) {
				return fmt.Errorf("spec.scimClient is immutable after creation (current: %q, desired: %q)", v, sci.Spec.SCIMClient)
			}
		}

		if v, ok := sci.Status.DescribeOutput["RUN_AS_ROLE"]; ok && v != "" {
			if !strings.EqualFold(sci.Spec.RunAsRole, v) {
				return fmt.Errorf("spec.runAsRole is immutable after creation (current: %q, desired: %q)", v, sci.Spec.RunAsRole)
			}
		}
	}

	return nil
}

func applyObservation(sci *snowplanev1alpha1.SCIMIntegration, obs *snowflake.SCIMIntegrationObservation) {
	if obs.ShowOutput != nil {
		sci.Status.FullyQualifiedName = obs.ShowOutput.Name
		sci.Status.ShowOutput = obs.ShowOutput
	}

	if obs.DescribeOutput != nil {
		sci.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(sci *snowplanev1alpha1.SCIMIntegration, id snowflake.AccountObjectIdentifier) snowflake.CreateSCIMIntegrationOptions {
	return snowflake.CreateSCIMIntegrationOptions{
		Name:          id,
		Enabled:       sci.Spec.Enabled,
		SCIMClient:    sci.Spec.SCIMClient,
		RunAsRole:     sci.Spec.RunAsRole,
		NetworkPolicy: sci.Spec.NetworkPolicy,
		SyncPassword:  sci.Spec.SyncPassword,
		Comment:       sci.Spec.Comment,
	}
}

func buildAlterOptions(sci *snowplanev1alpha1.SCIMIntegration, id snowflake.AccountObjectIdentifier, obs *snowflake.SCIMIntegrationObservation) snowflake.AlterSCIMIntegrationOptions {
	opts := snowflake.AlterSCIMIntegrationOptions{
		Name: id,
	}

	opts.UnsetFields = tracked.ComputeUnset(&sci.Spec, sci.Status.TrackedParameters)

	if sci.Spec.Enabled != nil {
		if obs == nil || obs.ShowOutput == nil || *sci.Spec.Enabled != obs.ShowOutput.Enabled {
			opts.Enabled = sci.Spec.Enabled
		}
	}

	if sci.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *sci.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = sci.Spec.Comment
		}
	}

	opts.NetworkPolicy = sci.Spec.NetworkPolicy
	opts.SyncPassword = sci.Spec.SyncPassword

	return opts
}

func detectDrift(sci *snowplanev1alpha1.SCIMIntegration, obs *snowflake.SCIMIntegrationObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", sci.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", sci.Spec.Comment, obs.ShowOutput.Comment, false)

		if sci.Spec.Enabled != nil {
			obsEnabled := obs.ShowOutput.Enabled
			d.CompareBool("ENABLED", sci.Spec.Enabled, &obsEnabled, false)
		}
	}

	if obs.DescribeOutput != nil {
		d.CompareStringValueFold("SCIM_CLIENT", sci.Spec.SCIMClient, describeValue(obs, "SCIM_CLIENT"), true)
		d.CompareStringValueFold("RUN_AS_ROLE", sci.Spec.RunAsRole, describeValue(obs, "RUN_AS_ROLE"), true)

		if sci.Spec.NetworkPolicy != nil {
			d.CompareStringValueFold("NETWORK_POLICY", *sci.Spec.NetworkPolicy, describeValue(obs, "NETWORK_POLICY"), false)
		}

		if sci.Spec.SyncPassword != nil {
			d.CompareStringValueFold("SYNC_PASSWORD", fmt.Sprintf("%t", *sci.Spec.SyncPassword), describeValue(obs, "SYNC_PASSWORD"), false)
		}
	}

	return d.Result()
}

func describeValue(obs *snowflake.SCIMIntegrationObservation, key string) string {
	if obs.DescribeOutput == nil {
		return ""
	}

	return obs.DescribeOutput[key]
}
