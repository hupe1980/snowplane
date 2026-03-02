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
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// Field index key for MaskingPolicyApplication cross-resource watches.
const (
	mpaIndexPolicyRef = ".mpa.refs.policyRef.name"
)

// adapter implements reconciler.ResourceAdapter for MaskingPolicyApplication.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.MaskingPolicyApplication, Service, *snowflake.MaskingPolicyApplicationObservation] = (*adapter)(nil)

func (a *adapter) ResourceName() string  { return "maskingpolicyapplication" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.MaskingPolicyApplication {
	return &snowplanev1alpha1.MaskingPolicyApplication{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

// PreReconcile resolves the optional PolicyRef to a fully qualified masking policy name.
func (a *adapter) PreReconcile(ctx context.Context, obj *snowplanev1alpha1.MaskingPolicyApplication) error {
	if ref := obj.Spec.PolicyRef; ref != nil {
		logger := log.FromContext(ctx)

		fqn, err := refresolver.ResolveLocalRef(ctx, a.client, obj.Namespace, ref.Name, func() refresolver.ReferableObject {
			mp := &snowplanev1alpha1.MaskingPolicy{}
			mp.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   snowplanev1alpha1.GroupVersion.Group,
				Version: snowplanev1alpha1.GroupVersion.Version,
				Kind:    "MaskingPolicy",
			})

			return mp
		})
		if err != nil {
			return refresolver.HandleRefError(ctx, obj, a.recorder, "MaskingPolicy", ref.Name, err)
		}

		obj.Spec.PolicyName = fqn
		obj.Status.PolicyName = fqn

		logger.V(1).Info("maskingpolicyapplication policyRef resolved", "policyName", fqn)
	} else if obj.Spec.PolicyName != "" {
		obj.Status.PolicyName = obj.Spec.PolicyName
	}

	return nil
}

// BuildIdentifier constructs a MaskingPolicyApplicationIdentifier from the spec.
func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.MaskingPolicyApplication) (reconciler.Identifier, error) {
	if obj.Spec.PolicyName == "" {
		return nil, fmt.Errorf("policyName must be set (either directly or via policyRef)")
	}

	return snowflake.MaskingPolicyApplicationIdentifier{
		PolicyName: obj.Spec.PolicyName,
		TableName:  obj.Spec.TableName,
		ColumnName: obj.Spec.ColumnName,
	}, nil
}

// SetupWatches configures a field indexer and cross-resource watch for MaskingPolicy refs.
func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		// PolicyRef indexer + watch.
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
				a.client,
				func() sigs.ObjectList { return &snowplanev1alpha1.MaskingPolicyApplicationList{} },
				mpaIndexPolicyRef,
				"listing maskingpolicyapplications for maskingpolicy watch",
			)),
		)

		return nil
	}
}

// Observe queries Snowflake for the current state of the masking policy application.
func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation], error) {
	mpaID, err := reconciler.AssertIdentifier[snowflake.MaskingPolicyApplicationIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, mpaID)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]{Exists: obs.Exists, Detail: obs}, nil
}

// Create applies the masking policy to the table column.
func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.MaskingPolicyApplication, _ reconciler.Identifier) error {
	return svc.SetMaskingPolicy(ctx, snowflake.SetMaskingPolicyOptions{
		PolicyName:   obj.Spec.PolicyName,
		TableName:    obj.Spec.TableName,
		ColumnName:   obj.Spec.ColumnName,
		UsingColumns: obj.Spec.UsingColumns,
	})
}

// Alter is a no-op — masking policy applications have no mutable fields.
func (a *adapter) Alter(_ context.Context, _ Service, opts reconciler.AlterOptions) error {
	return nil
}

// Drop removes the masking policy from the table column.
func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	mpaID, err := reconciler.AssertIdentifier[snowflake.MaskingPolicyApplicationIdentifier](id)
	if err != nil {
		return err
	}

	return svc.UnsetMaskingPolicy(ctx, snowflake.UnsetMaskingPolicyOptions{
		TableName:  mpaID.TableName,
		ColumnName: mpaID.ColumnName,
	})
}

// ValidateImmutableFields checks immutability of identity fields.
func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.MaskingPolicyApplication) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	// Identity fields are protected by CEL XValidation rules.
	return nil
}

// maskingPolicyApplicationAlterOptions implements reconciler.AlterOptions.
type maskingPolicyApplicationAlterOptions struct{}

func (o *maskingPolicyApplicationAlterOptions) HasChanges() bool { return false }

// BuildAlterOptions returns empty alter options — no mutable fields beyond identity.
func (a *adapter) BuildAlterOptions(_ context.Context, _ *snowplanev1alpha1.MaskingPolicyApplication, _ reconciler.Identifier, _ *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) (reconciler.AlterOptions, error) {
	return &maskingPolicyApplicationAlterOptions{}, nil
}

// ApplyObservation maps the observation into the CR's status.
func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.MaskingPolicyApplication, obs *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) {
	detail := obs.Detail
	if detail != nil && detail.Exists {
		obj.Status.FullyQualifiedName = snowflake.MaskingPolicyApplicationIdentifier{
			PolicyName: obj.Spec.PolicyName,
			TableName:  obj.Spec.TableName,
			ColumnName: obj.Spec.ColumnName,
		}.FullyQualifiedName()

		obj.Status.ObservedPolicyName = detail.PolicyName
	}
}

// ComputeTrackedParameters returns nil — masking policy applications don't track parameters.
func (a *adapter) ComputeTrackedParameters(_ *snowplanev1alpha1.MaskingPolicyApplication) []string {
	return nil
}

// DetectDrift compares the spec against the observed value.
func (a *adapter) DetectDrift(obj *snowplanev1alpha1.MaskingPolicyApplication, obs *reconciler.Observation[*snowflake.MaskingPolicyApplicationObservation]) *drift.Result {
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
