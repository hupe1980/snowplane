package databaserole

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
)

// adapter implements reconciler.ResourceAdapter for DatabaseRole.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "databaserole" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.DatabaseRole {
	return &snowplanev1alpha1.DatabaseRole{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, role *snowplanev1alpha1.DatabaseRole) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, role,
		role.Namespace, role.Spec.DatabaseRef, role.Spec.DatabaseName, role.Status.DatabaseName)
	if err != nil {
		return err
	}

	role.Status.DatabaseName = dbFQN

	refresolver.SetDatabaseResolvedCondition(role, role.Spec.DatabaseRef, role.Spec.DatabaseName, dbFQN)

	return nil
}

func (a *adapter) BuildIdentifier(role *snowplanev1alpha1.DatabaseRole) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(role.Status.DatabaseName)
	return snowflake.NewDatabaseObjectIdentifier(dbName, role.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.DatabaseRole{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				dr, ok := o.(*snowplanev1alpha1.DatabaseRole)
				if !ok || dr.Spec.DatabaseRef == nil {
					return nil
				}

				return []string{dr.Spec.DatabaseRef.Name}
			},
		); err != nil {
			return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
		}

		bldr.Watches(
			&snowplanev1alpha1.Database{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.DatabaseRoleList{} }, ".spec.databaseRef.name", "listing database roles for database watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.DatabaseRoleObservation], error) {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, did)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.DatabaseRoleObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.DatabaseRole, id reconciler.Identifier) error {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, did)
	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterDatabaseRoleOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, did)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, role *snowplanev1alpha1.DatabaseRole) error {
	if reconciler.ShouldSkipImmutableValidation(role) {
		return nil
	}

	if role.Status.ShowOutput != nil {
		if role.Status.ShowOutput.Name != "" && !strings.EqualFold(role.Spec.Name, role.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", role.Status.ShowOutput.Name, role.Spec.Name)
		}

		if role.Status.ShowOutput.DatabaseName != "" && role.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(role.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, role.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", role.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.DatabaseRole, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) (reconciler.AlterOptions, error) {
	did, err := reconciler.AssertIdentifier[snowflake.DatabaseObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, did, detail)
	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.DatabaseRole, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.DatabaseRole) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.DatabaseRole, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] = (*adapter)(nil)
