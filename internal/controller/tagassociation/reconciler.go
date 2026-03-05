// Package tagassociation implements the reconciler for TagAssociation resources.
package tagassociation

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
	finalizerName = "snowplane.hupe1980.github.io/tagassociation"
	taIndexTagRef = ".ta.refs.tagRef.name"
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
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return newTagAssociationService(snowflake.NewTagAssociationClient(exec))
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for TagAssociation resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation]{
		ResourceNameVal:  "tagassociation",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.TagAssociation { return &snowplanev1alpha1.TagAssociation{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.TagAssociation) (reconciler.Identifier, error) {
			if obj.Spec.TagName == nil || *obj.Spec.TagName == "" {
				return nil, fmt.Errorf("tagName must be set (either directly or via tagRef)")
			}

			return snowflake.TagAssociationIdentifier{
				TagName:    *obj.Spec.TagName,
				ObjectType: obj.Spec.ObjectType,
				ObjectName: obj.Spec.ObjectName,
			}, nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.TagAssociationIdentifier) (*snowflake.TagAssociationObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.TagAssociationObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.TagAssociation, _ snowflake.TagAssociationIdentifier) error {
			return svc.SetTag(ctx, snowflake.SetTagOptions{
				TagName:    *obj.Spec.TagName,
				TagValue:   obj.Spec.TagValue,
				ObjectType: obj.Spec.ObjectType,
				ObjectName: obj.Spec.ObjectName,
			})
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *tagAssociationAlterOptions) error {
			return svc.SetTag(ctx, opts.setOpts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.TagAssociationIdentifier) error {
			return svc.UnsetTag(ctx, snowflake.UnsetTagOptions(id))
		}),
		ValidateImmutableFn: func(_ context.Context, _ *snowplanev1alpha1.TagAssociation) error {
			// Identity fields are protected by CEL XValidation rules.
			return nil
		},
		BuildAlterOptsFn: func(_ context.Context, obj *snowplanev1alpha1.TagAssociation, _ reconciler.Identifier, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) (reconciler.AlterOptions, error) {
			return buildAlterOptions(obj, obs.Detail)
		},
		ApplyObservationFn: func(obj *snowplanev1alpha1.TagAssociation, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) {
			applyObservation(obj, obs.Detail)
		},
		TrackedParamsFn: func(_ *snowplanev1alpha1.TagAssociation) []string { return nil },
		DetectDriftFn: func(obj *snowplanev1alpha1.TagAssociation, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.TagAssociation) error {
			if ref := obj.Spec.TagRef; ref != nil {
				logger := log.FromContext(ctx)

				fqn, err := refresolver.ResolveLocalRef(ctx, c, obj.Namespace, ref.Name, ref.Namespace, func() refresolver.ReferableObject {
					t := &snowplanev1alpha1.Tag{}
					t.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   snowplanev1alpha1.GroupVersion.Group,
						Version: snowplanev1alpha1.GroupVersion.Version,
						Kind:    "Tag",
					})

					return t
				})
				if err != nil {
					return refresolver.HandleRefError(ctx, obj, recorder, "Tag", ref.Name, err)
				}

				obj.Spec.TagName = &fqn
				obj.Status.TagName = fqn

				logger.V(1).Info("tagassociation tagRef resolved", "tagName", fqn)
			} else if obj.Spec.TagName != nil {
				obj.Status.TagName = *obj.Spec.TagName
			}

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.TagAssociation{},
				taIndexTagRef,
				func(o sigs.Object) []string {
					ta, ok := o.(*snowplanev1alpha1.TagAssociation)
					if !ok || ta.Spec.TagRef == nil {
						return nil
					}

					return []string{ta.Spec.TagRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for %s: %w", taIndexTagRef, err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Tag{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(
					c,
					func() sigs.ObjectList { return &snowplanev1alpha1.TagAssociationList{} },
					taIndexTagRef,
					"listing tagassociations for tag watch",
				)),
			)

			return nil
		},
	}
}

// tagAssociationAlterOptions implements reconciler.AlterOptions for tag associations.
type tagAssociationAlterOptions struct {
	setOpts snowflake.SetTagOptions
}

func (o *tagAssociationAlterOptions) HasChanges() bool {
	return o.setOpts.TagValue != ""
}

// buildAlterOptions diffs the spec value vs the observed value.
func buildAlterOptions(obj *snowplanev1alpha1.TagAssociation, detail *snowflake.TagAssociationObservation) (reconciler.AlterOptions, error) {
	// If tag value matches, no changes needed.
	if detail != nil && detail.TagValue == obj.Spec.TagValue {
		return &tagAssociationAlterOptions{}, nil
	}

	return &tagAssociationAlterOptions{
		setOpts: snowflake.SetTagOptions{
			TagName:    *obj.Spec.TagName,
			TagValue:   obj.Spec.TagValue,
			ObjectType: obj.Spec.ObjectType,
			ObjectName: obj.Spec.ObjectName,
		},
	}, nil
}

// applyObservation maps the observation into the CR's status.
func applyObservation(obj *snowplanev1alpha1.TagAssociation, detail *snowflake.TagAssociationObservation) {
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.TagAssociationIdentifier{
			TagName:    *obj.Spec.TagName,
			ObjectType: obj.Spec.ObjectType,
			ObjectName: obj.Spec.ObjectName,
		}.FullyQualifiedName()

		obj.Status.ObservedValue = &snowplanev1alpha1.TagAssociationObservedValue{
			TagValue: detail.TagValue,
		}
	}
}

// detectDrift compares the spec tag value against the observed value.
func detectDrift(obj *snowplanev1alpha1.TagAssociation, detail *snowflake.TagAssociationObservation) *drift.Result {
	d := drift.New()

	if detail != nil && detail.Exists {
		d.CompareStringValue("TAG_VALUE", obj.Spec.TagValue, detail.TagValue, false)

		// Immutable fields — comparing against the identifier.
		if obj.Status.TagName != "" {
			d.CompareStringValueFold("TAG_NAME", *obj.Spec.TagName, obj.Status.TagName, true)
		}

		// OBJECT_TYPE and OBJECT_NAME are identity fields protected by CEL
		// XValidation rules. No observed counterpart exists from Snowflake,
		// so drift detection is not applicable for these fields.
	}

	return d.Result()
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
