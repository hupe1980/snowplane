package storageintegration

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for StorageIntegration.
type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "storageintegration" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.StorageIntegration {
	return &snowplanev1alpha1.StorageIntegration{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.StorageIntegration) (reconciler.Identifier, error) {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.StorageIntegrationObservation], error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.StorageIntegrationObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.StorageIntegration, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, aid)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterStorageIntegrationOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, si *snowplanev1alpha1.StorageIntegration) error {
	if reconciler.ShouldSkipImmutableValidation(si) {
		return nil
	}

	if si.Status.ShowOutput != nil {
		if si.Status.ShowOutput.Name != "" && !strings.EqualFold(si.Spec.Name, si.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", si.Status.ShowOutput.Name, si.Spec.Name)
		}

		// type (e.g., EXTERNAL_STAGE) is immutable.
		if si.Status.ShowOutput.Type != "" && !strings.EqualFold(string(si.Spec.Type), si.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", si.Status.ShowOutput.Type, si.Spec.Type)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.StorageIntegration, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.StorageIntegrationObservation]) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, aid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.StorageIntegration, obs *reconciler.Observation[*snowflake.StorageIntegrationObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.StorageIntegration) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.StorageIntegration, obs *reconciler.Observation[*snowflake.StorageIntegrationObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.StorageIntegration, Service, *snowflake.StorageIntegrationObservation] = (*adapter)(nil)
