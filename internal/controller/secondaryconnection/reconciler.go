// Package secondaryconnection implements the reconciler for SecondaryConnection resources.
package secondaryconnection

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
	finalizerName = "snowplane.hupe1980.github.io/secondaryconnection"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake SecondaryConnections.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.SecondaryConnectionObservation, error)
	Create(ctx context.Context, opts snowflake.CreateSecondaryConnectionOptions) error
	Alter(ctx context.Context, opts snowflake.AlterSecondaryConnectionOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new SecondaryConnection reconciler.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.SecondaryConnection, Service, *snowflake.SecondaryConnectionObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewSecondaryConnectionClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.SecondaryConnection, Service, *snowflake.SecondaryConnectionObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.SecondaryConnection, Service, *snowflake.SecondaryConnectionObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.SecondaryConnection, Service, *snowflake.SecondaryConnectionObservation]{
		ResourceNameVal:  "secondaryconnection",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.SecondaryConnection {
			return &snowplanev1alpha1.SecondaryConnection{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.SecondaryConnection) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.SecondaryConnectionObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.SecondaryConnectionObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.SecondaryConnection, id snowflake.AccountObjectIdentifier) error {
			return svc.Create(ctx, buildCreateOptions(obj, id))
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterSecondaryConnectionOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.SecondaryConnection, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.SecondaryConnectionObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.SecondaryConnection, obs *reconciler.Observation[*snowflake.SecondaryConnectionObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.SecondaryConnection, obs *reconciler.Observation[*snowflake.SecondaryConnectionObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.SecondaryConnection) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}

		// asReplicaOf is immutable — verify against the primary shown in SHOW CONNECTIONS.
		if obj.Status.ShowOutput.PrimaryName != "" && !strings.EqualFold(obj.Spec.AsReplicaOf, obj.Status.ShowOutput.PrimaryName) {
			return fmt.Errorf("spec.asReplicaOf is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.PrimaryName, obj.Spec.AsReplicaOf)
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.SecondaryConnection, obs *snowflake.SecondaryConnectionObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = obs.ShowOutput.Name
		obj.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.SecondaryConnection, id snowflake.AccountObjectIdentifier) snowflake.CreateSecondaryConnectionOptions {
	return snowflake.CreateSecondaryConnectionOptions{
		Name:        id,
		AsReplicaOf: obj.Spec.AsReplicaOf,
		Comment:     obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.SecondaryConnection, id snowflake.AccountObjectIdentifier, obs *snowflake.SecondaryConnectionObservation) snowflake.AlterSecondaryConnectionOptions {
	opts := snowflake.AlterSecondaryConnectionOptions{
		Name: id,
	}

	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Comment != nil {
		if obs == nil || obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.SecondaryConnection, obs *snowflake.SecondaryConnectionObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareStringValueFold("AS_REPLICA_OF", obj.Spec.AsReplicaOf, obs.ShowOutput.PrimaryName, true)
	}

	return d.Result()
}
