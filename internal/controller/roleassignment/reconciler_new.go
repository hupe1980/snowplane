// Package roleassignment implements the reconcilers for AccountRoleAssignment
// and DatabaseRoleAssignment resources.
package roleassignment

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

// Service defines operations the reconciler needs against Snowflake role assignments.
// Unlike privilege grants, role assignments have no ALTER operation. They are either
// granted (created) or revoked (dropped).
type Service interface {
	Observe(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error)
	GrantRole(ctx context.Context, opts snowflake.GrantRoleOptions) error
	RevokeRole(ctx context.Context, opts snowflake.RevokeRoleOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// defaultServiceFactory is the production ServiceFactory.
func defaultServiceFactory(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	sfC, cleanup, err := reconciler.WithUseRole(ctx, sfClient, useRole)
	if err != nil {
		return nil, nil, err
	}

	return newRoleAssignmentService(snowflake.NewRoleAssignmentClient(sfC)), cleanup, nil
}

// roleAssignmentService wraps RoleAssignmentClient to satisfy the Service interface.
type roleAssignmentService struct {
	client *snowflake.RoleAssignmentClient
}

func newRoleAssignmentService(c *snowflake.RoleAssignmentClient) *roleAssignmentService {
	return &roleAssignmentService{client: c}
}

func (s *roleAssignmentService) Observe(ctx context.Context, id snowflake.RoleAssignmentIdentifier) (*snowflake.RoleAssignmentObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *roleAssignmentService) GrantRole(ctx context.Context, opts snowflake.GrantRoleOptions) error {
	return s.client.GrantRole(ctx, opts)
}

func (s *roleAssignmentService) RevokeRole(ctx context.Context, opts snowflake.RevokeRoleOptions) error {
	return s.client.RevokeRole(ctx, opts)
}

// ---------------------------------------------------------------------------
// AccountRoleAssignment reconciler
// ---------------------------------------------------------------------------

// NewAccountRoleAssignmentReconciler returns a new reconciler for AccountRoleAssignment.
func NewAccountRoleAssignmentReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	a := &accountRoleAssignmentAdapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewAccountRoleAssignmentReconcilerWithServiceFactory lets callers inject a custom ServiceFactory (for tests).
func NewAccountRoleAssignmentReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	a := &accountRoleAssignmentAdapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.AccountRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// ---------------------------------------------------------------------------
// DatabaseRoleAssignment reconciler
// ---------------------------------------------------------------------------

// NewDatabaseRoleAssignmentReconciler returns a new reconciler for DatabaseRoleAssignment.
func NewDatabaseRoleAssignmentReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	a := &databaseRoleAssignmentAdapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}

// NewDatabaseRoleAssignmentReconcilerWithServiceFactory lets callers inject a custom ServiceFactory (for tests).
func NewDatabaseRoleAssignmentReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation] {
	a := &databaseRoleAssignmentAdapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRoleAssignment, Service, *snowflake.RoleAssignmentObservation]{
		Client:      c,
		Factory:     factory,
		Recorder:    recorder,
		RateLimiter: rl,
		Adapter:     a,
	}
}
