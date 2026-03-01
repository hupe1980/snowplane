// Package tagassociation implements the reconciler for TagAssociation resources.
package tagassociation

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
	finalizerName = "snowplane.hupe1980.github.io/tagassociation"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake tag associations.
// Tag associations have no traditional ALTER — to change the value we re-issue SET TAG,
// and to remove the association we UNSET TAG.
type Service interface {
	Observe(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error)
	SetTag(ctx context.Context, opts snowflake.SetTagOptions) error
	UnsetTag(ctx context.Context, opts snowflake.UnsetTagOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new TagAssociation reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation]{
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

	return newTagAssociationService(snowflake.NewTagAssociationClient(sfC)), cleanup, nil
}

// tagAssociationService wraps TagAssociationClient to satisfy the Service interface.
type tagAssociationService struct {
	client *snowflake.TagAssociationClient
}

func newTagAssociationService(c *snowflake.TagAssociationClient) *tagAssociationService {
	return &tagAssociationService{client: c}
}

func (s *tagAssociationService) Observe(ctx context.Context, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *tagAssociationService) SetTag(ctx context.Context, opts snowflake.SetTagOptions) error {
	return s.client.SetTag(ctx, opts)
}

func (s *tagAssociationService) UnsetTag(ctx context.Context, opts snowflake.UnsetTagOptions) error {
	return s.client.UnsetTag(ctx, opts)
}
