// Package passwordpolicyattachment implements the reconciler for PasswordPolicyAttachment resources.
package passwordpolicyattachment

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
)

const (
	finalizerName     = "snowplane.hupe1980.github.io/passwordpolicyattachment"
	ppaIndexPolicyRef = ".ppa.refs.policyRef.name"
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
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return newPasswordPolicyAttachmentService(snowflake.NewPasswordPolicyAttachmentClient(exec))
		}),
	)
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
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// passwordPolicyAttachmentAlterOptions implements reconciler.AlterOptions.
type passwordPolicyAttachmentAlterOptions struct{}

func (o *passwordPolicyAttachmentAlterOptions) HasChanges() bool { return false }

// newAdapter creates the BaseAdapter for PasswordPolicyAttachment resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation]{
		ResourceNameVal:  "passwordpolicyattachment",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.PasswordPolicyAttachment {
			return &snowplanev1alpha1.PasswordPolicyAttachment{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.PasswordPolicyAttachment) (reconciler.Identifier, error) {
			if obj.Spec.PolicyName == nil || *obj.Spec.PolicyName == "" {
				return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
			}

			return snowflake.PasswordPolicyAttachmentIdentifier{
				PolicyName: *obj.Spec.PolicyName,
				TargetType: obj.Spec.TargetType,
				TargetName: obj.Spec.TargetName,
			}, nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.PasswordPolicyAttachmentIdentifier) (*snowflake.PasswordPolicyAttachmentObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.PasswordPolicyAttachmentObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.PasswordPolicyAttachment, _ snowflake.PasswordPolicyAttachmentIdentifier) error {
			return svc.SetPasswordPolicy(ctx, snowflake.SetPasswordPolicyOptions{
				PolicyName: *obj.Spec.PolicyName,
				TargetType: obj.Spec.TargetType,
				TargetName: obj.Spec.TargetName,
			})
		}),
		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.PasswordPolicyAttachmentIdentifier) error {
			return svc.UnsetPasswordPolicy(ctx, snowflake.UnsetPasswordPolicyOptions{
				TargetType: id.TargetType,
				TargetName: id.TargetName,
			})
		}),
		ValidateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.PasswordPolicyAttachment) error {
			// Identity fields are protected by CEL XValidation rules.
			return nil
		},
		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.PasswordPolicyAttachment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) (reconciler.AlterOptions, error) {
			return &passwordPolicyAttachmentAlterOptions{}, nil
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.PasswordPolicyAttachment, obs *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) {
			detail := obs.Detail
			if detail != nil && detail.Exists {
				obj.Status.FullyQualifiedName = snowflake.PasswordPolicyAttachmentIdentifier{
					PolicyName: *obj.Spec.PolicyName,
					TargetType: obj.Spec.TargetType,
					TargetName: obj.Spec.TargetName,
				}.FullyQualifiedName()

				obj.Status.ObservedPolicyName = detail.PolicyName
			}
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.PasswordPolicyAttachment) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.PasswordPolicyAttachment, obs *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) *drift.Result {
			d := drift.New()

			detail := obs.Detail
			if detail != nil && detail.Exists {
				if obj.Status.PolicyName != "" {
					d.CompareStringValueFold("POLICY_NAME", *obj.Spec.PolicyName, obj.Status.PolicyName, true)
				}
			}

			return d.Result()
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.PasswordPolicyAttachment) error {
			if ref := obj.Spec.PolicyRef; ref != nil {
				logger := log.FromContext(ctx)

				fqn, err := refresolver.ResolveLocalRef(ctx, c, obj.Namespace, ref.Name, ref.Namespace, func() refresolver.ReferableObject {
					pp := &snowplanev1alpha1.PasswordPolicy{}
					pp.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   snowplanev1alpha1.GroupVersion.Group,
						Version: snowplanev1alpha1.GroupVersion.Version,
						Kind:    "PasswordPolicy",
					})

					return pp
				})
				if err != nil {
					return refresolver.HandleRefError(ctx, obj, recorder, "PasswordPolicy", ref.Name, err)
				}

				obj.Spec.PolicyName = &fqn
				obj.Status.PolicyName = fqn

				logger.V(1).Info("passwordpolicyattachment policyRef resolved", "policyName", fqn)
			} else if obj.Spec.PolicyName != nil {
				obj.Status.PolicyName = *obj.Spec.PolicyName
			}

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.PasswordPolicyAttachment{},
				ppaIndexPolicyRef,
				func(o sigs.Object) []string {
					ppa, ok := o.(*snowplanev1alpha1.PasswordPolicyAttachment)
					if !ok || ppa.Spec.PolicyRef == nil {
						return nil
					}

					return []string{ppa.Spec.PolicyRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", ppaIndexPolicyRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.PasswordPolicy{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(
					c,
					func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyAttachmentList{} },
					ppaIndexPolicyRef,
					"listing passwordpolicyattachments for passwordpolicy watch",
				)),
			)

			return nil
		},
	}
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
