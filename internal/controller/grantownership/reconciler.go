// Package grantownership implements the reconciler for GrantOwnership resources.
package grantownership

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
)

const (
	finalizerName = "snowplane.hupe1980.github.io/grantownership"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake ownership transfers.
type Service interface {
	Observe(ctx context.Context, id snowflake.GrantOwnershipIdentifier) (*snowflake.GrantOwnershipObservation, error)
	Create(ctx context.Context, opts snowflake.CreateGrantOwnershipOptions) error
	Drop(ctx context.Context, id snowflake.GrantOwnershipIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new GrantOwnership reconciler backed by the generic framework.
func NewReconciler(c client.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c client.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.GrantOwnership, Service, *snowflake.GrantOwnershipObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return snowflake.NewGrantOwnershipClient(sfC), cleanup, nil
}

// ownershipAlterOptions always reports no changes (ownership is immutable).
type ownershipAlterOptions struct{}

func (o *ownershipAlterOptions) HasChanges() bool { return false }

func applyObservation(g *snowplanev1alpha1.GrantOwnership, obs *snowflake.GrantOwnershipObservation) {
	g.Status.RoleName = resolveGranteeName(g)

	if obs.ShowOutput != nil {
		g.Status.ShowOutput = &snowplanev1alpha1.GrantOwnershipShowOutput{
			CreatedOn:   obs.ShowOutput.CreatedOn,
			Privilege:   obs.ShowOutput.Privilege,
			GrantedOn:   obs.ShowOutput.GrantedOn,
			Name:        obs.ShowOutput.Name,
			GrantedTo:   obs.ShowOutput.GrantedTo,
			GranteeName: obs.ShowOutput.GranteeName,
		}
	}
}

// resolveGranteeName extracts the resolved grantee name from the spec.
func resolveGranteeName(g *snowplanev1alpha1.GrantOwnership) string {
	if g.Spec.AccountRole != "" {
		return g.Spec.AccountRole
	}

	if g.Spec.DatabaseRole != "" {
		return g.Spec.DatabaseRole
	}

	return ""
}

// buildToRole constructs the TO clause value (e.g. "ROLE MY_ROLE").
func buildToRole(g *snowplanev1alpha1.GrantOwnership) string {
	return snowflake.BuildToClause(g.Spec.AccountRole, g.Spec.DatabaseRole, "")
}

func detectDrift(g *snowplanev1alpha1.GrantOwnership, obs *snowflake.GrantOwnershipObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		granteeName := resolveGranteeName(g)

		// Ownership target is immutable.
		d.CompareStringValueFold("GRANTEE", granteeName, obs.ShowOutput.GranteeName, true)
	}

	return d.Result()
}
