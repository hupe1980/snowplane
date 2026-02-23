package warehouse

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for Warehouse.
type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "warehouse" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Warehouse {
	return &snowplanev1alpha1.Warehouse{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.Warehouse) error {
	return nil
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.Warehouse) reconciler.Identifier {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name)
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc { return nil }

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Warehouse, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, aid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	if err := svc.Create(ctx, opts); err != nil {
		return err
	}

	// Track resource constraint for change detection on future reconciles.
	if obj.Spec.ResourceConstraint != nil {
		obj.Status.LastAppliedResourceConstraint = string(*obj.Spec.ResourceConstraint)
	}

	return nil
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterWarehouseOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, aid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, wh *snowplanev1alpha1.Warehouse) error {
	if reconciler.ShouldSkipImmutableValidation(wh) {
		return nil
	}

	if wh.Status.ShowOutput != nil {
		if wh.Status.ShowOutput.Name != "" && !strings.EqualFold(wh.Spec.Name, wh.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", wh.Status.ShowOutput.Name, wh.Spec.Name)
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Warehouse, id reconciler.Identifier, obs *reconciler.Observation) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail, err := reconciler.AssertDetail[*snowflake.WarehouseObservation](obs)
	if err != nil {
		return nil, err
	}

	opts := buildAlterOptions(obj, aid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Warehouse, obs *reconciler.Observation) {
	detail, ok := obs.Detail.(*snowflake.WarehouseObservation)
	if !ok {
		return
	}

	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Warehouse) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Warehouse, obs *reconciler.Observation) *drift.Result {
	detail, ok := obs.Detail.(*snowflake.WarehouseObservation)
	if !ok {
		return drift.New().Result()
	}

	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.Warehouse) {}

func (a *adapter) PostUpdate(wh *snowplanev1alpha1.Warehouse, altered bool, alterOpts reconciler.AlterOptions) {
	// Commit resource constraint only after a successful ALTER, reading from
	// the per-reconciliation AlterOptions (no shared mutable state).
	if altered {
		if opts, ok := alterOpts.(*snowflake.AlterWarehouseOptions); ok && opts.ResourceConstraint != nil {
			wh.Status.LastAppliedResourceConstraint = *opts.ResourceConstraint
		}
	}

	// Clear tracked value when resource constraint is removed from spec.
	if wh.Spec.ResourceConstraint == nil {
		wh.Status.LastAppliedResourceConstraint = ""
	}
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Warehouse, Service] = (*adapter)(nil)
