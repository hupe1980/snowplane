// Package grant implements the reconcilers for AccountRoleGrant,
// DatabaseRoleGrant, and ShareGrant resources.
package grant

import (
	"context"

	"k8s.io/client-go/tools/record"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake grants.
// Unlike other resources, grants have no ALTER operation. They are either
// granted (created) or revoked (dropped).
type Service interface {
	Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error)
	Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error
	Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return newGrantService(snowflake.NewGrantClient(sfC)), cleanup, nil
}

// grantService wraps GrantClient to satisfy the Service interface.
type grantService struct {
	client *snowflake.GrantClient
}

func newGrantService(c *snowflake.GrantClient) *grantService {
	return &grantService{client: c}
}

func (s *grantService) Observe(ctx context.Context, id snowflake.GrantIdentifier) (*snowflake.GrantObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *grantService) Grant(ctx context.Context, opts snowflake.CreateGrantOptions) error {
	return s.client.Grant(ctx, opts)
}

func (s *grantService) Revoke(ctx context.Context, opts snowflake.RevokeGrantOptions) error {
	return s.client.Revoke(ctx, opts)
}

// ---------------------------------------------------------------------------
// AccountRoleGrant reconciler
// ---------------------------------------------------------------------------

// NewAccountRoleGrantReconciler returns a new reconciler for AccountRoleGrant.
func NewAccountRoleGrantReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleGrant, Service, *snowflake.GrantObservation] {
	a := &accountRoleGrantAdapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewAccountRoleGrantReconcilerWithServiceFactory lets callers inject a custom ServiceFactory (for tests).
func NewAccountRoleGrantReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleGrant, Service, *snowflake.GrantObservation] {
	a := &accountRoleGrantAdapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// ---------------------------------------------------------------------------
// DatabaseRoleGrant reconciler
// ---------------------------------------------------------------------------

// NewDatabaseRoleGrantReconciler returns a new reconciler for DatabaseRoleGrant.
func NewDatabaseRoleGrantReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleGrant, Service, *snowflake.GrantObservation] {
	a := &databaseRoleGrantAdapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewDatabaseRoleGrantReconcilerWithServiceFactory lets callers inject a custom ServiceFactory (for tests).
func NewDatabaseRoleGrantReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleGrant, Service, *snowflake.GrantObservation] {
	a := &databaseRoleGrantAdapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// ---------------------------------------------------------------------------
// ShareGrant reconciler
// ---------------------------------------------------------------------------

// NewShareGrantReconciler returns a new reconciler for ShareGrant.
func NewShareGrantReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ShareGrant, Service, *snowflake.GrantObservation] {
	a := &shareGrantAdapter{client: c, recorder: recorder, newService: defaultServiceFactory}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ShareGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewShareGrantReconcilerWithServiceFactory lets callers inject a custom ServiceFactory (for tests).
func NewShareGrantReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.ShareGrant, Service, *snowflake.GrantObservation] {
	a := &shareGrantAdapter{client: c, recorder: recorder, newService: sf}
	return &reconciler.GenericReconciler[*snowplanev1alpha1.ShareGrant, Service, *snowflake.GrantObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}
