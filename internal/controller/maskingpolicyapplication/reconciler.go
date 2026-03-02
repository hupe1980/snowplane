// Package maskingpolicyapplication implements the reconciler for MaskingPolicyApplication resources.
package maskingpolicyapplication

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
	finalizerName = "snowplane.hupe1980.github.io/maskingpolicyapplication"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake masking policy applications.
type Service interface {
	Observe(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error)
	SetMaskingPolicy(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error
	UnsetMaskingPolicy(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new MaskingPolicyApplication reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation]{
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

	return newMaskingPolicyApplicationService(snowflake.NewMaskingPolicyApplicationClient(sfC)), cleanup, nil
}

// maskingPolicyApplicationService wraps MaskingPolicyApplicationClient to satisfy the Service interface.
type maskingPolicyApplicationService struct {
	client *snowflake.MaskingPolicyApplicationClient
}

func newMaskingPolicyApplicationService(c *snowflake.MaskingPolicyApplicationClient) *maskingPolicyApplicationService {
	return &maskingPolicyApplicationService{client: c}
}

func (s *maskingPolicyApplicationService) Observe(ctx context.Context, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *maskingPolicyApplicationService) SetMaskingPolicy(ctx context.Context, opts snowflake.SetMaskingPolicyOptions) error {
	return s.client.SetMaskingPolicy(ctx, opts)
}

func (s *maskingPolicyApplicationService) UnsetMaskingPolicy(ctx context.Context, opts snowflake.UnsetMaskingPolicyOptions) error {
	return s.client.UnsetMaskingPolicy(ctx, opts)
}
