// Package accountrole implements the reconciler for AccountRole resources.
package accountrole

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
	finalizerName = "snowplane.hupe1980.github.io/accountrole"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake account roles.
type Service interface {
	Observe(ctx context.Context, name snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error)
	Create(ctx context.Context, opts snowflake.CreateAccountRoleOptions) error
	Alter(ctx context.Context, opts snowflake.AlterAccountRoleOptions) error
	Drop(ctx context.Context, name snowflake.AccountObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new AccountRole reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewAccountRoleClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(sf))
}

// newAdapter creates the BaseAdapter for AccountRole resources.
func newAdapter(sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation]{
		ResourceNameVal:  "accountrole",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.AccountRole { return &snowplanev1alpha1.AccountRole{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.AccountRole) (reconciler.Identifier, error) {
			return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) (*snowflake.AccountRoleObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.AccountRoleObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.AccountRole, id snowflake.AccountObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterAccountRoleOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.AccountObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.AccountRole, id snowflake.AccountObjectIdentifier, obs *reconciler.Observation[*snowflake.AccountRoleObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.AccountRole, obs *reconciler.Observation[*snowflake.AccountRoleObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.AccountRole, obs *reconciler.Observation[*snowflake.AccountRoleObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
	}
}

func validateImmutableFields(_ context.Context, role *snowplanev1alpha1.AccountRole) error {
	if reconciler.ShouldSkipImmutableValidation(role) {
		return nil
	}

	if role.Status.ShowOutput != nil {
		if role.Status.ShowOutput.Name != "" && !strings.EqualFold(role.Spec.Name, role.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", role.Status.ShowOutput.Name, role.Spec.Name)
		}
	}

	return nil
}

func applyObservation(role *snowplanev1alpha1.AccountRole, obs *snowflake.AccountRoleObservation) {
	if obs.ShowOutput != nil {
		role.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()

		role.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(role *snowplanev1alpha1.AccountRole, id snowflake.AccountObjectIdentifier) snowflake.CreateAccountRoleOptions {
	return snowflake.CreateAccountRoleOptions{
		Name:    id,
		Comment: role.Spec.Comment,
	}
}

func buildAlterOptions(role *snowplanev1alpha1.AccountRole, id snowflake.AccountObjectIdentifier, obs *snowflake.AccountRoleObservation) snowflake.AlterAccountRoleOptions {
	opts := snowflake.AlterAccountRoleOptions{Name: id}

	// Detect fields that were previously managed but are now nil -> UNSET.
	opts.UnsetFields = tracked.ComputeUnset(&role.Spec, role.Status.TrackedParameters)

	if role.Spec.Comment != nil {
		if obs.ShowOutput == nil || *role.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = role.Spec.Comment
		}
	}

	return opts
}

// detectDrift compares desired spec against the observed state and
// returns a structured drift result.
func detectDrift(role *snowplanev1alpha1.AccountRole, obs *snowflake.AccountRoleObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", role.Spec.Name, obs.ShowOutput.Name, true)

		// Mutable fields.
		d.CompareString("COMMENT", role.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
