package accountrole

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for AccountRole.
type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "accountrole" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.AccountRole {
	return &snowplanev1alpha1.AccountRole{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.AccountRole) error {
	return nil
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.AccountRole) reconciler.Identifier {
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

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.AccountRole, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, aid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterAccountRoleOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, role *snowplanev1alpha1.AccountRole) error {
	if reconciler.ShouldSkipImmutableValidation(role) {
		return nil
	}

	if role.Status.ShowOutput != nil {
		if role.Status.ShowOutput.Name != "" && !strings.EqualFold(role.Spec.Name, role.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", role.Status.ShowOutput.Name, role.Spec.Name)
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.AccountRole, id reconciler.Identifier, obs *reconciler.Observation) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail, err := reconciler.AssertDetail[*snowflake.AccountRoleObservation](obs)
	if err != nil {
		return nil, err
	}

	opts := buildAlterOptions(obj, aid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.AccountRole, obs *reconciler.Observation) {
	detail, ok := obs.Detail.(*snowflake.AccountRoleObservation)
	if !ok {
		return
	}

	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.AccountRole) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.AccountRole, obs *reconciler.Observation) *drift.Result {
	detail, ok := obs.Detail.(*snowflake.AccountRoleObservation)
	if !ok {
		return drift.New().Result()
	}

	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.AccountRole)                                    {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.AccountRole, _ bool, _ reconciler.AlterOptions) {}

func (a *adapter) SupportsCreateOrAlter() bool { return false }

// Compile-time interface check.
var _ reconciler.ResourceAdapter[*snowplanev1alpha1.AccountRole, Service] = (*adapter)(nil)
