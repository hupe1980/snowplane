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
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// Field index key for PasswordPolicyAttachment cross-resource watches.
const (
	ppaIndexPolicyRef = ".ppa.refs.policyRef.name"
)

// adapter implements reconciler.ResourceAdapter for PasswordPolicyAttachment.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.PasswordPolicyAttachment, Service, *snowflake.PasswordPolicyAttachmentObservation] = (*adapter)(nil)

func (a *adapter) ResourceName() string  { return "passwordpolicyattachment" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.PasswordPolicyAttachment {
	return &snowplanev1alpha1.PasswordPolicyAttachment{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves the optional PolicyRef to a fully qualified password policy name.
func (a *adapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.PasswordPolicyAttachment) error {
	if ref := obj.Spec.PolicyRef; ref != nil {
		logger := log.FromContext(ctx)

		fqn, err := refresolver.ResolveLocalRef(ctx, a.client, obj.Namespace, ref.Name, func() refresolver.ReferableObject {
			pp := &snowplanev1alpha1.PasswordPolicy{}
			pp.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   snowplanev1alpha1.GroupVersion.Group,
				Version: snowplanev1alpha1.GroupVersion.Version,
				Kind:    "PasswordPolicy",
			})

			return pp
		})
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "PasswordPolicy", ref.Name, err)
		}

		obj.Spec.PolicyName = fqn
		obj.Status.PolicyName = fqn

		logger.V(1).Info("passwordpolicyattachment policyRef resolved", "policyName", fqn)
	} else if obj.Spec.PolicyName != "" {
		obj.Status.PolicyName = obj.Spec.PolicyName
	}

	return nil
}

// BuildIdentifier constructs a PasswordPolicyAttachmentIdentifier from the spec.
func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.PasswordPolicyAttachment) (reconciler.Identifier, error) {
	if obj.Spec.PolicyName == "" {
		return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
	}

	return snowflake.PasswordPolicyAttachmentIdentifier{
		PolicyName: obj.Spec.PolicyName,
		TargetType: obj.Spec.TargetType,
		TargetName: obj.Spec.TargetName,
	}, nil
}

// SetupWatches configures a field indexer and cross-resource watch for PasswordPolicy refs.
func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		// PolicyRef indexer + watch.
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
				a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.PasswordPolicyAttachmentList{} },
				ppaIndexPolicyRef,
				"listing passwordpolicyattachments for passwordpolicy watch",
			)),
		)

		return nil
	}
}

// Observe queries Snowflake for the current state of the password policy attachment.
func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation], error) {
	ppaID, err := reconciler.AssertIdentifier[snowflake.PasswordPolicyAttachmentIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, ppaID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// Create attaches the password policy to the target.
func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.PasswordPolicyAttachment, _ reconciler.Identifier) error {
	return svc.SetPasswordPolicy(ctx, snowflake.SetPasswordPolicyOptions{
		PolicyName: obj.Spec.PolicyName,
		TargetType: obj.Spec.TargetType,
		TargetName: obj.Spec.TargetName,
	})
}

// Alter is a no-op — password policy attachments have no mutable fields.
func (a *adapter) Alter(_ context.Context, _ Service, opts reconciler.AlterOptions) error {
	return nil
}

// Drop detaches the password policy from the target.
func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	ppaID, err := reconciler.AssertIdentifier[snowflake.PasswordPolicyAttachmentIdentifier](id)
	if err != nil {
		return err
	}

	return svc.UnsetPasswordPolicy(ctx, snowflake.UnsetPasswordPolicyOptions{
		TargetType: ppaID.TargetType,
		TargetName: ppaID.TargetName,
	})
}

// ValidateImmutableFields checks immutability of identity fields.
func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.PasswordPolicyAttachment) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Identity fields are protected by CEL XValidation rules.
	return nil
}

// passwordPolicyAttachmentAlterOptions implements reconciler.AlterOptions.
type passwordPolicyAttachmentAlterOptions struct{}

func (o *passwordPolicyAttachmentAlterOptions) HasChanges() bool { return false }

// BuildAlterOptions returns empty alter options — no mutable fields beyond identity.
func (a *adapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.PasswordPolicyAttachment, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) (reconciler.AlterOptions, error) {
	return &passwordPolicyAttachmentAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.PasswordPolicyAttachment, obs *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) {
	detail := obs.Detail
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.PasswordPolicyAttachmentIdentifier{
			PolicyName: obj.Spec.PolicyName,
			TargetType: obj.Spec.TargetType,
			TargetName: obj.Spec.TargetName,
		}.FullyQualifiedName()

		obj.Status.ObservedPolicyName = detail.PolicyName
	}
}

// ComputeTrackedParameters returns nil — password policy attachments don't track parameters.
func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.PasswordPolicyAttachment) []string {
	return nil
}

// DetectDrift compares the spec against the observed value.
func (a *adapter) DetectDrift(obj *snowplanev1alpha1.PasswordPolicyAttachment, obs *reconciler.Observation[*snowflake.PasswordPolicyAttachmentObservation]) *drift.Result {
	d := drift.New()

	detail := obs.Detail
	if detail != nil && detail.Exists {
		// Immutable fields — comparing against the identifier.
		if obj.Status.PolicyName != "" {
			d.CompareStringValueFold("POLICY_NAME", obj.Spec.PolicyName, obj.Status.PolicyName, true)
		}
	}

	return d.Result()
}
