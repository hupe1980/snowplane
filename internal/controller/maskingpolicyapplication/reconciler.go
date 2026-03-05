// Package maskingpolicyapplication implements the reconciler for MaskingPolicyApplication resources.
package maskingpolicyapplication

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
	finalizerName     = "snowplane.hupe1980.github.io/maskingpolicyapplication"
	mpaIndexPolicyRef = ".mpa.refs.policyRef.name"
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
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return newMaskingPolicyApplicationService(snowflake.NewMaskingPolicyApplicationClient(exec))
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// maskingPolicyApplicationAlterOptions implements reconciler.AlterOptions.
type maskingPolicyApplicationAlterOptions struct{}

func (o *maskingPolicyApplicationAlterOptions) HasChanges() bool { return false }

// newAdapter creates the BaseAdapter for MaskingPolicyApplication resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation]{
		ResourceNameVal:  "maskingpolicyapplication",
		FinalizerNameVal: finalizerName,
		NewObjectFn: func() *snowplanev1alpha1.MaskingPolicyApplication {
			return &snowplanev1alpha1.MaskingPolicyApplication{}
		},
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.MaskingPolicyApplication) (reconciler.Identifier, error) {
			if obj.Spec.PolicyName == nil || *obj.Spec.PolicyName == "" {
				return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
			}

			return snowflake.MaskingPolicyApplicationIdentifier{
				PolicyName: *obj.Spec.PolicyName,
				TableName:  obj.Spec.TableName,
				ColumnName: obj.Spec.ColumnName,
			}, nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.MaskingPolicyApplicationIdentifier) (*snowflake.MaskingPolicyApplicationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.MaskingPolicyApplicationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.MaskingPolicyApplication, _ snowflake.MaskingPolicyApplicationIdentifier) error {
			return svc.SetMaskingPolicy(ctx, snowflake.SetMaskingPolicyOptions{
				PolicyName:   *obj.Spec.PolicyName,
				TableName:    obj.Spec.TableName,
				ColumnName:   obj.Spec.ColumnName,
				UsingColumns: obj.Spec.UsingColumns,
			})
		}),
		AlterFn: func(_ context.Context, _ Service, _ reconciler.AlterOptions) error { return nil },
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.MaskingPolicyApplicationIdentifier) error {
			return svc.UnsetMaskingPolicy(ctx, snowflake.UnsetMaskingPolicyOptions{
				TableName:  id.TableName,
				ColumnName: id.ColumnName,
			})
		}),
		ValidateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.MaskingPolicyApplication) error {
			// Identity fields are protected by CEL XValidation rules.
			return nil
		},
		BuildAlterOptsFn: func(_ context.Context, _ *snowplanev1alpha1.MaskingPolicyApplication, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) (reconciler.AlterOptions, error) {
			return &maskingPolicyApplicationAlterOptions{}, nil
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.MaskingPolicyApplication, obs *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) {
			detail := obs.Detail
			if detail != nil && detail.Exists {
				obj.Status.FullyQualifiedName = snowflake.MaskingPolicyApplicationIdentifier{
					PolicyName: *obj.Spec.PolicyName,
					TableName:  obj.Spec.TableName,
					ColumnName: obj.Spec.ColumnName,
				}.FullyQualifiedName()

				obj.Status.ObservedPolicyName = detail.PolicyName
			}
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.MaskingPolicyApplication) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.MaskingPolicyApplication, obs *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) *drift.Result {
			d := drift.New()

			detail := obs.Detail
			if detail != nil && detail.Exists {
				if obj.Status.PolicyName != "" {
					d.CompareStringValueFold("POLICY_NAME", *obj.Spec.PolicyName, obj.Status.PolicyName, true)
				}
			}

			return d.Result()
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.MaskingPolicyApplication) error {
			if ref := obj.Spec.PolicyRef; ref != nil {
				logger := log.FromContext(ctx)

				fqn, err := refresolver.ResolveLocalRef(ctx, c, obj.Namespace, ref.Name, ref.Namespace, func() refresolver.ReferableObject {
					mp := &snowplanev1alpha1.MaskingPolicy{}
					mp.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   snowplanev1alpha1.GroupVersion.Group,
						Version: snowplanev1alpha1.GroupVersion.Version,
						Kind:    "MaskingPolicy",
					})

					return mp
				})
				if err != nil {
					return refresolver.HandleRefError(ctx, obj, recorder, "MaskingPolicy", ref.Name, err)
				}

				obj.Spec.PolicyName = &fqn
				obj.Status.PolicyName = fqn

				logger.V(1).Info("maskingpolicyapplication policyRef resolved", "policyName", fqn)
			} else if obj.Spec.PolicyName != nil {
				obj.Status.PolicyName = *obj.Spec.PolicyName
			}

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.MaskingPolicyApplication{},
				mpaIndexPolicyRef,
				func(o sigs.Object) []string {
					mpa, ok := o.(*snowplanev1alpha1.MaskingPolicyApplication)
					if !ok || mpa.Spec.PolicyRef == nil {
						return nil
					}

					return []string{mpa.Spec.PolicyRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", mpaIndexPolicyRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.MaskingPolicy{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(
					c,
					func() sigs.ObjectList { return &snowplanev1alpha1.MaskingPolicyApplicationList{} },
					mpaIndexPolicyRef,
					"listing maskingpolicyapplications for maskingpolicy watch",
				)),
			)

			return nil
		},
	}
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
