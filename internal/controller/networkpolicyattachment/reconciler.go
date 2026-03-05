// Package networkpolicyattachment implements the reconciler for NetworkPolicyAttachment resources.
package networkpolicyattachment

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
	finalizerName     = "snowplane.hupe1980.github.io/networkpolicyattachment"
	npaIndexPolicyRef = ".npa.refs.policyRef.name"
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
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return newNetworkPolicyAttachmentService(snowflake.NewNetworkPolicyAttachmentClient(exec))
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// networkPolicyAttachmentAlterOptions implements reconciler.AlterOptions.
type networkPolicyAttachmentAlterOptions struct{}

func (o *networkPolicyAttachmentAlterOptions) HasChanges() bool { return false }

// newAdapter creates the BaseAdapter for NetworkPolicyAttachment resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation]{
		ResourceNameVal:  "networkpolicyattachment",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.NetworkPolicyAttachment {
			return &snowplanev1alpha1.NetworkPolicyAttachment{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.NetworkPolicyAttachment) (reconciler.Identifier, error) {
			if obj.Spec.PolicyName == nil || *obj.Spec.PolicyName == "" {
				return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
			}

			return snowflake.NetworkPolicyAttachmentIdentifier{
				PolicyName: *obj.Spec.PolicyName,
				TargetType: obj.Spec.TargetType,
				TargetName: obj.Spec.TargetName,
			}, nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.NetworkPolicyAttachmentIdentifier) (*snowflake.NetworkPolicyAttachmentObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.NetworkPolicyAttachmentObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.NetworkPolicyAttachment, _ snowflake.NetworkPolicyAttachmentIdentifier) error {
			return svc.SetNetworkPolicy(ctx, snowflake.SetNetworkPolicyOptions{
				PolicyName: *obj.Spec.PolicyName,
				TargetType: obj.Spec.TargetType,
				TargetName: obj.Spec.TargetName,
			})
		}),
		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.NetworkPolicyAttachmentIdentifier) error {
			return svc.UnsetNetworkPolicy(ctx, snowflake.UnsetNetworkPolicyOptions{
				TargetType: id.TargetType,
				TargetName: id.TargetName,
			})
		}),
		ValidateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.NetworkPolicyAttachment) error {
			// Identity fields are protected by CEL XValidation rules.
			return nil
		},
		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.NetworkPolicyAttachment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) (reconciler.AlterOptions, error) {
			return &networkPolicyAttachmentAlterOptions{}, nil
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.NetworkPolicyAttachment, obs *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) {
			detail := obs.Detail
			if detail != nil && detail.Exists {
				obj.Status.FullyQualifiedName = snowflake.NetworkPolicyAttachmentIdentifier{
					PolicyName: *obj.Spec.PolicyName,
					TargetType: obj.Spec.TargetType,
					TargetName: obj.Spec.TargetName,
				}.FullyQualifiedName()

				obj.Status.ObservedPolicyName = detail.PolicyName
			}
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.NetworkPolicyAttachment) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.NetworkPolicyAttachment, obs *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) *drift.Result {
			d := drift.New()

			detail := obs.Detail
			if detail != nil && detail.Exists {
				if obj.Status.PolicyName != "" {
					d.CompareStringValueFold("POLICY_NAME", *obj.Spec.PolicyName, obj.Status.PolicyName, true)
				}
			}

			return d.Result()
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.NetworkPolicyAttachment) error {
			if ref := obj.Spec.PolicyRef; ref != nil {
				logger := log.FromContext(ctx)

				fqn, err := refresolver.ResolveLocalRef(ctx, c, obj.Namespace, ref.Name, ref.Namespace, func() refresolver.ReferableObject {
					np := &snowplanev1alpha1.NetworkPolicy{}
					np.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   snowplanev1alpha1.GroupVersion.Group,
						Version: snowplanev1alpha1.GroupVersion.Version,
						Kind:    "NetworkPolicy",
					})

					return np
				})
				if err != nil {
					return refresolver.HandleRefError(ctx, obj, recorder, "NetworkPolicy", ref.Name, err)
				}

				obj.Spec.PolicyName = &fqn
				obj.Status.PolicyName = fqn

				logger.V(1).Info("networkpolicyattachment policyRef resolved", "policyName", fqn)
			} else if obj.Spec.PolicyName != nil {
				obj.Status.PolicyName = *obj.Spec.PolicyName
			}

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.NetworkPolicyAttachment{},
				npaIndexPolicyRef,
				func(o sigs.Object) []string {
					npa, ok := o.(*snowplanev1alpha1.NetworkPolicyAttachment)
					if !ok || npa.Spec.PolicyRef == nil {
						return nil
					}

					return []string{npa.Spec.PolicyRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", npaIndexPolicyRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.NetworkPolicy{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(
					c,
					func() sigs.ObjectList { return &snowplanev1alpha1.NetworkPolicyAttachmentList{} },
					npaIndexPolicyRef,
					"listing networkpolicyattachments for networkpolicy watch",
				)),
			)

			return nil
		},
	}
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
