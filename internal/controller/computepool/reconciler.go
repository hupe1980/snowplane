// Package computepool implements the reconciler for ComputePool resources.
package computepool

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
	finalizerName = "snowplane.hupe1980.github.io/computepool"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake compute pools.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.ComputePoolObservation, error)
	Create(ctx context.Context, opts snowflake.CreateComputePoolOptions) error
	Alter(ctx context.Context, opts snowflake.AlterComputePoolOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ComputePool reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ComputePool, Service, *snowflake.ComputePoolObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewComputePoolClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ComputePool, Service, *snowflake.ComputePoolObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for ComputePool resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ComputePool, Service, *snowflake.ComputePoolObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ComputePool, Service, *snowflake.ComputePoolObservation]{
		ResourceNameVal:  "computepool",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.ComputePool { return &snowplanev1alpha1.ComputePool{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.ComputePool) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.ComputePoolObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ComputePoolObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ComputePool, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterComputePoolOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ComputePool, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.ComputePoolObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ComputePool, obs *reconciler.Observation[*snowflake.ComputePoolObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ComputePool, obs *reconciler.Observation[*snowflake.ComputePoolObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, cp *snowplanev1alpha1.ComputePool) error {
	if reconciler.ShouldSkipImmutableValidation(cp) {
		return nil
	}

	if cp.Status.ShowOutput != nil {
		if cp.Status.ShowOutput.Name != "" && !strings.EqualFold(cp.Spec.Name, cp.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", cp.Status.ShowOutput.Name, cp.Spec.Name)
		}

		if cp.Status.ShowOutput.InstanceFamily != "" && !strings.EqualFold(cp.Spec.InstanceFamily, cp.Status.ShowOutput.InstanceFamily) {
			return fmt.Errorf("spec.instanceFamily is immutable after creation (current: %q, desired: %q)", cp.Status.ShowOutput.InstanceFamily, cp.Spec.InstanceFamily)
		}
	}

	return nil
}

func applyObservation(cp *snowplanev1alpha1.ComputePool, obs *snowflake.ComputePoolObservation) {
	if obs.ShowOutput != nil {
		cp.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()
		cp.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(cp *snowplanev1alpha1.ComputePool, id snowflake.AccountObjectIdentifier) snowflake.CreateComputePoolOptions {
	return snowflake.CreateComputePoolOptions{
		Name:            id,
		MinNodes:        cp.Spec.MinNodes,
		MaxNodes:        cp.Spec.MaxNodes,
		InstanceFamily:  cp.Spec.InstanceFamily,
		AutoResume:      cp.Spec.AutoResume,
		AutoSuspendSecs: cp.Spec.AutoSuspendSecs,
		Comment:         cp.Spec.Comment,
	}
}

func buildAlterOptions(cp *snowplanev1alpha1.ComputePool, id snowflake.AccountObjectIdentifier, obs *snowflake.ComputePoolObservation) snowflake.AlterComputePoolOptions {
	opts := snowflake.AlterComputePoolOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&cp.Spec, cp.Status.TrackedParameters)

	if obs.ShowOutput != nil {
		if cp.Spec.MinNodes != obs.ShowOutput.MinNodes {
			v := cp.Spec.MinNodes
			opts.MinNodes = &v
		}

		if cp.Spec.MaxNodes != obs.ShowOutput.MaxNodes {
			v := cp.Spec.MaxNodes
			opts.MaxNodes = &v
		}

		if cp.Spec.AutoResume != nil {
			current := strings.EqualFold(obs.ShowOutput.AutoResume, "true")
			if *cp.Spec.AutoResume != current {
				opts.AutoResume = cp.Spec.AutoResume
			}
		}

		if cp.Spec.AutoSuspendSecs != nil && *cp.Spec.AutoSuspendSecs != obs.ShowOutput.AutoSuspend {
			opts.AutoSuspendSecs = cp.Spec.AutoSuspendSecs
		}

		if cp.Spec.Comment != nil && *cp.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = cp.Spec.Comment
		}
	}

	return opts
}

func detectDrift(cp *snowplanev1alpha1.ComputePool, obs *snowflake.ComputePoolObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", cp.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("INSTANCE_FAMILY", cp.Spec.InstanceFamily, obs.ShowOutput.InstanceFamily, true)
		d.CompareInt32("MIN_NODES", &cp.Spec.MinNodes, &obs.ShowOutput.MinNodes, false)
		d.CompareInt32("MAX_NODES", &cp.Spec.MaxNodes, &obs.ShowOutput.MaxNodes, false)

		if cp.Spec.AutoResume != nil {
			obsResume := strings.EqualFold(obs.ShowOutput.AutoResume, "true")
			d.CompareBool("AUTO_RESUME", cp.Spec.AutoResume, &obsResume, false)
		}

		d.CompareInt32("AUTO_SUSPEND_SECS", cp.Spec.AutoSuspendSecs, &obs.ShowOutput.AutoSuspend, false)
		d.CompareString("COMMENT", cp.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
