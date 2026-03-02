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
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// Field index key for NetworkPolicyAttachment cross-resource watches.
const (
	npaIndexPolicyRef = ".npa.refs.policyRef.name"
)

// adapter implements reconciler.ResourceAdapter for NetworkPolicyAttachment.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.NetworkPolicyAttachment, Service, *snowflake.NetworkPolicyAttachmentObservation] = (*adapter)(nil)

func (a *adapter) ResourceName() string  { return "networkpolicyattachment" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.NetworkPolicyAttachment {
	return &snowplanev1alpha1.NetworkPolicyAttachment{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves the optional PolicyRef to a network policy name.
func (a *adapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.NetworkPolicyAttachment) error {
	if ref := obj.Spec.PolicyRef; ref != nil {
		logger := log.FromContext(ctx)

		fqn, err := refresolver.ResolveLocalRef(ctx, a.client, obj.Namespace, ref.Name, func() refresolver.ReferableObject {
			np := &snowplanev1alpha1.NetworkPolicy{}
			np.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   snowplanev1alpha1.GroupVersion.Group,
				Version: snowplanev1alpha1.GroupVersion.Version,
				Kind:    "NetworkPolicy",
			})

			return np
		})
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "NetworkPolicy", ref.Name, err)
		}

		obj.Spec.PolicyName = &fqn
		obj.Status.PolicyName = fqn

		logger.V(1).Info("networkpolicyattachment policyRef resolved", "policyName", fqn)
	} else if obj.Spec.PolicyName != nil {
		obj.Status.PolicyName = *obj.Spec.PolicyName
	}

	return nil
}

// BuildIdentifier constructs a NetworkPolicyAttachmentIdentifier from the spec.
func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.NetworkPolicyAttachment) (reconciler.Identifier, error) {
	if obj.Spec.PolicyName == nil || *obj.Spec.PolicyName == "" {
		return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
	}

	return snowflake.NetworkPolicyAttachmentIdentifier{
		PolicyName: *obj.Spec.PolicyName,
		TargetType: obj.Spec.TargetType,
		TargetName: obj.Spec.TargetName,
	}, nil
}

// SetupWatches configures a field indexer and cross-resource watch for NetworkPolicy refs.
func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		// PolicyRef indexer + watch.
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
				a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.NetworkPolicyAttachmentList{} },
				npaIndexPolicyRef,
				"listing networkpolicyattachments for networkpolicy watch",
			)),
		)

		return nil
	}
}

// Observe queries Snowflake for the current state of the network policy attachment.
func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation], error) {
	npaID, err := reconciler.AssertIdentifier[snowflake.NetworkPolicyAttachmentIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, npaID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// Create attaches the network policy to the target.
func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.NetworkPolicyAttachment, _ reconciler.Identifier) error {
	return svc.SetNetworkPolicy(ctx, snowflake.SetNetworkPolicyOptions{
		PolicyName: *obj.Spec.PolicyName,
		TargetType: obj.Spec.TargetType,
		TargetName: obj.Spec.TargetName,
	})
}

// Alter is a no-op — network policy attachments have no mutable fields.
func (a *adapter) Alter(_ context.Context, _ Service, opts reconciler.AlterOptions) error {
	return nil
}

// Drop detaches the network policy from the target.
func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	npaID, err := reconciler.AssertIdentifier[snowflake.NetworkPolicyAttachmentIdentifier](id)
	if err != nil {
		return err
	}

	return svc.UnsetNetworkPolicy(ctx, snowflake.UnsetNetworkPolicyOptions{
		TargetType: npaID.TargetType,
		TargetName: npaID.TargetName,
	})
}

// ValidateImmutableFields checks immutability of identity fields.
func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.NetworkPolicyAttachment) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Identity fields are protected by CEL XValidation rules.
	return nil
}

// networkPolicyAttachmentAlterOptions implements reconciler.AlterOptions.
type networkPolicyAttachmentAlterOptions struct{}

func (o *networkPolicyAttachmentAlterOptions) HasChanges() bool { return false }

// BuildAlterOptions returns empty alter options — no mutable fields beyond identity.
func (a *adapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.NetworkPolicyAttachment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) (reconciler.AlterOptions, error) {
	return &networkPolicyAttachmentAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.NetworkPolicyAttachment, obs *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) {
	detail := obs.Detail
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.NetworkPolicyAttachmentIdentifier{
			PolicyName: *obj.Spec.PolicyName,
			TargetType: obj.Spec.TargetType,
			TargetName: obj.Spec.TargetName,
		}.FullyQualifiedName()

		obj.Status.ObservedPolicyName = detail.PolicyName
	}
}

// ComputeTrackedParameters returns nil — network policy attachments don't track parameters.
func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.NetworkPolicyAttachment) []string {
	return nil
}

// DetectDrift compares the spec against the observed value.
func (a *adapter) DetectDrift(obj *snowplanev1alpha1.NetworkPolicyAttachment, obs *reconciler.Observation[*snowflake.NetworkPolicyAttachmentObservation]) *drift.Result {
	d := drift.New()

	detail := obs.Detail
	if detail != nil && detail.Exists {
		// Immutable fields — comparing against the identifier.
		if obj.Status.PolicyName != "" {
			d.CompareStringValueFold("POLICY_NAME", *obj.Spec.PolicyName, obj.Status.PolicyName, true)
		}
	}

	return d.Result()
}
