// Package tableconstraint implements the reconciler for TableConstraint resources.
package tableconstraint

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

const (
	finalizerName = "snowplane.hupe1980.github.io/tableconstraint"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake table constraints.
type Service interface {
	Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error)
	AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error
	AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error
	DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new TableConstraint reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation]{
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
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TableConstraint, Service, *snowflake.TableConstraintObservation]{
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

	return newTableConstraintService(snowflake.NewTableConstraintClient(sfC)), cleanup, nil
}

// tableConstraintService wraps TableConstraintClient to satisfy the Service interface.
type tableConstraintService struct {
	client *snowflake.TableConstraintClient
}

func newTableConstraintService(c *snowflake.TableConstraintClient) *tableConstraintService {
	return &tableConstraintService{client: c}
}

func (s *tableConstraintService) Observe(ctx context.Context, id snowflake.TableConstraintIdentifier, constraintType string) (*snowflake.TableConstraintObservation, error) {
	return s.client.Observe(ctx, id, constraintType)
}

func (s *tableConstraintService) AddConstraint(ctx context.Context, opts snowflake.AddConstraintOptions) error {
	return s.client.AddConstraint(ctx, opts)
}

func (s *tableConstraintService) AlterConstraint(ctx context.Context, opts snowflake.AlterConstraintOptions) error {
	return s.client.AlterConstraint(ctx, opts)
}

func (s *tableConstraintService) DropConstraint(ctx context.Context, id snowflake.TableConstraintIdentifier) error {
	return s.client.DropConstraint(ctx, id)
}
