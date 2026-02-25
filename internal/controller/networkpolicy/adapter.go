package networkpolicy

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for NetworkPolicy.
type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "networkpolicy" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.NetworkPolicy {
	return &snowplanev1alpha1.NetworkPolicy{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(_ context.Context, _ *snowplanev1alpha1.NetworkPolicy) error {
	return nil
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.NetworkPolicy) (reconciler.Identifier, error) {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc { return nil }

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.NetworkPolicyObservation], error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.NetworkPolicyObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.NetworkPolicy, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, aid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterNetworkPolicyOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, np *snowplanev1alpha1.NetworkPolicy) error {
	if reconciler.ShouldSkipImmutableValidation(np) {
		return nil
	}

	if np.Status.ShowOutput != nil {
		if np.Status.ShowOutput.Name != "" && !strings.EqualFold(np.Spec.Name, np.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", np.Status.ShowOutput.Name, np.Spec.Name)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.NetworkPolicy, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.NetworkPolicyObservation]) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, aid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.NetworkPolicy, obs *reconciler.Observation[*snowflake.NetworkPolicyObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.NetworkPolicy) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.NetworkPolicy, obs *reconciler.Observation[*snowflake.NetworkPolicyObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) PostCreate(_ *snowplanev1alpha1.NetworkPolicy)                                    {}
func (a *adapter) PostUpdate(_ *snowplanev1alpha1.NetworkPolicy, _ bool, _ reconciler.AlterOptions) {}
func (a *adapter) SupportsCreateOrAlter() bool                                                      { return false }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.NetworkPolicy, Service, *snowflake.NetworkPolicyObservation] = (*adapter)(nil)
