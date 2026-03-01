// Package accountrole implements the reconciler for AccountRole resources.
package accountrole

import (
	"context"

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
	a := &adapter{newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
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
	a := &adapter{newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRole, Service, *snowflake.AccountRoleObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory used by NewReconciler.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewAccountRoleClient(sfC), cleanup, nil
}

func applyObservation(role *snowplanev1alpha1.AccountRole, obs *snowflake.AccountRoleObservation) {
	if obs.ShowOutput != nil {
		role.Status.FullyQualifiedName = snowflake.NewAccountObjectIdentifier(obs.ShowOutput.Name).FullyQualifiedName()

		role.Status.ShowOutput = &snowplanev1alpha1.AccountRoleShowOutput{
			CreatedOn:      obs.ShowOutput.CreatedOn,
			Name:           obs.ShowOutput.Name,
			Comment:        obs.ShowOutput.Comment,
			Owner:          obs.ShowOutput.Owner,
			GrantedToRoles: obs.ShowOutput.GrantedToRoles,
			GrantedRoles:   obs.ShowOutput.GrantedRoles,
		}
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
