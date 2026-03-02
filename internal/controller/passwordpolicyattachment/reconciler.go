// Package passwordpolicyattachment implements the reconciler for PasswordPolicyAttachment resources.
package passwordpolicyattachment

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
	finalizerName = "snowplane.hupe1980.github.io/passwordpolicyattachment"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake password policy attachments.
type Service interface {
	Observe(ctx context.Context, id snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error)
	SetPasswordPolicy(ctx context.Context, opts snowflake.SetPasswordPolicyOptions) error
	UnsetPasswordPolicy(ctx context.Context, opts snowflake.UnsetPasswordPolicyOptions) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new PasswordPolicyAttachment reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation] {
	a := &adapter{client: c, recorder: recorder, newService: defaultServiceFactory}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation]{
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation] {
	a := &adapter{client: c, recorder: recorder, newService: sf}

	return &reconciler.GenericReconciler[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation]{
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

	return newPasswordPolicyAttachmentService(snowflake.NewPasswordPolicyAttachmentClient(sfC)), cleanup, nil
}

// passwordPolicyAttachmentService wraps PasswordPolicyAttachmentClient to satisfy the Service interface.
type passwordPolicyAttachmentService struct {
	client *snowflake.PasswordPolicyAttachmentClient
}

func newPasswordPolicyAttachmentService(c *snowflake.PasswordPolicyAttachmentClient) *passwordPolicyAttachmentService {
	return &passwordPolicyAttachmentService{client: c}
}

func (s *passwordPolicyAttachmentService) Observe(ctx context.Context, id snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error) {
	return s.client.Observe(ctx, id)
}

func (s *passwordPolicyAttachmentService) SetPasswordPolicy(ctx context.Context, opts snowflake.SetPasswordPolicyOptions) error {
	return s.client.SetPasswordPolicy(ctx, opts)
}

func (s *passwordPolicyAttachmentService) UnsetPasswordPolicy(ctx context.Context, opts snowflake.UnsetPasswordPolicyOptions) error {
	return s.client.UnsetPasswordPolicy(ctx, opts)
}
