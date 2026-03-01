package tagassociation

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// Field index key for TagAssociation cross-resource watches.
const (
	taIndexTagRef = ".ta.refs.tagRef.name"
)

// adapter implements reconciler.ResourceAdapter for TagAssociation.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.TagAssociation, Service, *snowflake.TagAssociationObservation] = (*adapter)(nil)

func (a *adapter) ResourceName() string  { return "tagassociation" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.TagAssociation {
	return &snowplanev1alpha1.TagAssociation{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves the optional TagRef to a fully qualified tag name.
func (a *adapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.TagAssociation) error {
	if ref := obj.Spec.TagRef; ref != nil {
		logger := log.FromContext(ctx)

		fqn, err := refresolver.ResolveLocalRef(ctx, a.client, obj.Namespace, ref.Name, func() refresolver.ReferableObject {
			t := &snowplanev1alpha1.Tag{}
			t.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   snowplanev1alpha1.GroupVersion.Group,
				Version: snowplanev1alpha1.GroupVersion.Version,
				Kind:    "Tag",
			})

			return t
		})
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "Tag", ref.Name, err)
		}

		obj.Spec.TagName = fqn
		obj.Status.TagName = fqn

		logger.V(1).Info("tagassociation tagRef resolved", "tagName", fqn)
	} else if obj.Spec.TagName != "" {
		obj.Status.TagName = obj.Spec.TagName
	}

	return nil
}

// BuildIdentifier constructs a TagAssociationIdentifier from the spec.
func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.TagAssociation) (reconciler.Identifier, error) {
	if obj.Spec.TagName == "" {
		return nil, fmt.Errorf("tagName must be set (either directly or via tagRef)")
	}

	return snowflake.TagAssociationIdentifier{
		TagName:    obj.Spec.TagName,
		ObjectType: obj.Spec.ObjectType,
		ObjectName: obj.Spec.ObjectName,
	}, nil
}

// SetupWatches configures a field indexer and cross-resource watch for Tag refs.
func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		// TagRef indexer + watch.
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
				a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.TagAssociationList{} },
				taIndexTagRef,
				"listing tagassociations for tag watch",
			)),
		)

		return nil
	}
}

// Observe queries Snowflake for the current state of the tag association.
func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.TagAssociationObservation], error) {
	taID, err := reconciler.AssertIdentifier[snowflake.TagAssociationIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, taID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.TagAssociationObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// Create sets the tag on the object in Snowflake.
func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.TagAssociation, _ reconciler.Identifier) error {
	return svc.SetTag(ctx, snowflake.SetTagOptions{
		TagName:    obj.Spec.TagName,
		TagValue:   obj.Spec.TagValue,
		ObjectType: obj.Spec.ObjectType,
		ObjectName: obj.Spec.ObjectName,
	})
}

// Alter re-sets the tag with the updated value.
func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*tagAssociationAlterOptions](opts)
	if err != nil {
		return err
	}

	return svc.SetTag(ctx, ao.setOpts)
}

// Drop unsets the tag from the object in Snowflake.
func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	taID, err := reconciler.AssertIdentifier[snowflake.TagAssociationIdentifier](id)
	if err != nil {
		return err
	}

	return svc.UnsetTag(ctx, snowflake.UnsetTagOptions(taID))
}

// ValidateImmutableFields checks immutability of identity fields.
func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.TagAssociation) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Identity fields (tagName, objectType, objectName) are protected by CEL
	// XValidation rules. Runtime validation is only needed for tagRef resolution
	// consistency, which is checked during PreReconcile. No additional runtime
	// validation needed beyond what CEL provides.
	return nil
}

// tagAssociationAlterOptions implements reconciler.AlterOptions for tag associations.
type tagAssociationAlterOptions struct {
	setOpts snowflake.SetTagOptions
}

func (o *tagAssociationAlterOptions) HasChanges() bool {
	return o.setOpts.TagValue != ""
}

// BuildAlterOptions diffs the spec value vs the observed value.
func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.TagAssociation, _ reconciler.Identifier, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) (reconciler.AlterOptions, error) {
	detail := obs.Detail

	// If tag value matches, no changes needed.
	if detail != nil && detail.TagValue == obj.Spec.TagValue {
		return &tagAssociationAlterOptions{}, nil
	}

	return &tagAssociationAlterOptions{
		setOpts: snowflake.SetTagOptions{
			TagName:    obj.Spec.TagName,
			TagValue:   obj.Spec.TagValue,
			ObjectType: obj.Spec.ObjectType,
			ObjectName: obj.Spec.ObjectName,
		},
	}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.TagAssociation, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) {
	detail := obs.Detail
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.TagAssociationIdentifier{
			TagName:    obj.Spec.TagName,
			ObjectType: obj.Spec.ObjectType,
			ObjectName: obj.Spec.ObjectName,
		}.FullyQualifiedName()

		obj.Status.ObservedValue = &snowplanev1alpha1.TagAssociationObservedValue{
			TagValue: detail.TagValue,
		}
	}
}

// ComputeTrackedParameters returns nil — tag associations don't track parameters.
func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.TagAssociation) []string {
	return nil
}

// DetectDrift compares the spec tag value against the observed value.
func (a *adapter) DetectDrift(obj *snowplanev1alpha1.TagAssociation, obs *reconciler.Observation[*snowflake.TagAssociationObservation]) *drift.Result {
	d := drift.New()

	detail := obs.Detail
	if detail != nil && detail.Exists {
		d.CompareStringValue("TAG_VALUE", obj.Spec.TagValue, detail.TagValue, false)

		// Immutable fields — comparing against the identifier.
		if obj.Status.TagName != "" {
			d.CompareStringValueFold("TAG_NAME", obj.Spec.TagName, obj.Status.TagName, true)
		}

		d.CompareStringValueFold("OBJECT_TYPE", obj.Spec.ObjectType, obj.Spec.ObjectType, true)

		if !strings.EqualFold(obj.Spec.ObjectName, obj.Spec.ObjectName) {
			d.CompareStringValueFold("OBJECT_NAME", obj.Spec.ObjectName, obj.Spec.ObjectName, true)
		}
	}

	return d.Result()
}
