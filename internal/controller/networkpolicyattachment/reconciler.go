// Package networkpolicyattachment implements the reconciler for NetworkPolicyAttachment resources.
package networkpolicyattachment

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
	finalizerName = "snowplane.hupe1980.github.io/networkpolicyattachment"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake network policy attachments.
type Service interface {
	Observe(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error)
	SetNetworkPolicy(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error
	UnsetNetworkPolicy(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new NetworkPolicyAttachment reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation]{
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

	return newNetworkPolicyAttachmentService(snowflake.NewNetworkPolicyAttachmentClient(sfC)), cleanup, nil
}

// networkPolicyAttachmentService wraps NetworkPolicyAttachmentClient to satisfy the Service interface.
type networkPolicyAttachmentService struct {
	client *snowflake.NetworkPolicyAttachmentClient
}

func newNetworkPolicyAttachmentService(c *snowflake.NetworkPolicyAttachmentClient) *networkPolicyAttachmentService {
	return &networkPolicyAttachmentService{client: c}
}

func (s *networkPolicyAttachmentService) Observe(ctx context.Context, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *networkPolicyAttachmentService) SetNetworkPolicy(ctx context.Context, opts snowflake.SetNetworkPolicyOptions) error {
	return s.client.SetNetworkPolicy(ctx, opts)
}

func (s *networkPolicyAttachmentService) UnsetNetworkPolicy(ctx context.Context, opts snowflake.UnsetNetworkPolicyOptions) error {
	return s.client.UnsetNetworkPolicy(ctx, opts)
}
