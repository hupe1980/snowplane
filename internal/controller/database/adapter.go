package database

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

// adapter implements reconciler.ResourceAdapter for Database.
type adapter struct {
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "database" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.Database {
	return &snowplanev1alpha1.Database{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) BuildIdentifier(obj *snowplanev1alpha1.Database) (reconciler.Identifier, error) {
	return snowflake.NewAccountObjectIdentifier(obj.Spec.Name), nil
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.DatabaseObservation], error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, aid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.DatabaseObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.Database, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, aid)
	opts.UseCreateOrAlter = obj.GetManagementPolicies().IsCreateOrAlter()

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterDatabaseOptions](opts)
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

func (a *adapter) ValidateImmutableFields(_ context.Context, db *snowplanev1alpha1.Database) error {
	if reconciler.ShouldSkipImmutableValidation(db) {
		return nil
	}

	if db.Status.ShowOutput != nil {
		isTransient := db.Status.ShowOutput.Kind == "TRANSIENT"
		if db.Spec.Transient != isTransient {
			return fmt.Errorf("spec.transient is immutable after creation (current: %v, desired: %v)", isTransient, db.Spec.Transient)
		}

		if db.Status.ShowOutput.Name != "" && !strings.EqualFold(db.Spec.Name, db.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", db.Status.ShowOutput.Name, db.Spec.Name)
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.Database, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.DatabaseObservation]) (reconciler.AlterOptions, error) {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, aid, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.Database, obs *reconciler.Observation[*snowflake.DatabaseObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.Database) []string {
	return tracked.ComputeTracked(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.Database, obs *reconciler.Observation[*snowflake.DatabaseObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

func (a *adapter) DropCascade(ctx context.Context, svc Service, id reconciler.Identifier) error {
	aid, err := reconciler.AssertIdentifier[snowflake.AccountObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.DropCascade(ctx, aid)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.Database, Service, *snowflake.DatabaseObservation] = (*adapter)(nil)
var _ reconciler.CascadeDropper[*snowplanev1alpha1.Database, Service] = (*adapter)(nil)
