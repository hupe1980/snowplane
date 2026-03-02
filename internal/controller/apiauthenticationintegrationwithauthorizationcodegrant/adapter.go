package apiauthenticationintegrationwithauthorizationcodegrant

import (
	"context"
	"fmt"
	"strings"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/tracked"
)

type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string {
	return "apiauthenticationintegrationwithauthorizationcodegrant"
}
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant {
	return &snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant) (reconciler.Identifier, error) {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation], error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Create(ctx, buildCreateOptions(obj, aid))
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterAPIAuthenticationIntegrationOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil && obj.Status.ShowOutput.Name != nil {
		if *obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, *obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", *obj.Status.ShowOutput.Name, obj.Spec.Name)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	opts := buildAlterOptions(obj, aid, obs.Detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) {
	applyObservation(obj, obs.Detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant, obs *reconciler.Observation[*snowflake.APIAuthenticationIntegrationObservation]) *drift.Result {
	return detectDrift(obj, obs.Detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.APIAuthenticationIntegrationWithAuthorizationCodeGrant, Service, *snowflake.APIAuthenticationIntegrationObservation] = (*adapter)(nil)
