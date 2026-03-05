// Package databaserole implements the reconciler for DatabaseRole resources.
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
	"github.com/hupe1980/snowplane/internal/clients/clientfactory"
	"github.com/hupe1980/snowplane/internal/clients/snowflake"
	"github.com/hupe1980/snowplane/internal/controller/reconciler"
	"github.com/hupe1980/snowplane/internal/controller/refresolver"
	"github.com/hupe1980/snowplane/internal/drift"
	"github.com/hupe1980/snowplane/internal/ratelimit"
	"github.com/hupe1980/snowplane/internal/tracked"
)

const (
	finalizerName = "snowplane.hupe1980.github.io/databaserole"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake database roles.
type Service interface {
	Observe(ctx context.Context, name snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error)
	Create(ctx context.Context, opts snowflake.CreateDatabaseRoleOptions) error
	Alter(ctx context.Context, opts snowflake.AlterDatabaseRoleOptions) error
	Drop(ctx context.Context, name snowflake.DatabaseObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
// When useRole is non-empty the factory pins a connection, switches to that
// role, and returns a cleanup function that restores the original role.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new DatabaseRole reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewDatabaseRoleClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory. This is intended for integration tests that
// inject mock Snowflake services while still going through SetupWithManager.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for DatabaseRole resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.DatabaseRole, Service, *snowflake.DatabaseRoleObservation]{
		ResourceNameVal:  "databaserole",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.DatabaseRole { return &snowplanev1alpha1.DatabaseRole{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(role *snowplanev1alpha1.DatabaseRole) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(role.Status.DatabaseName)
			return snowflake.NewDatabaseObjectIdentifier(dbName, role.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.DatabaseObjectIdentifier) (*snowflake.DatabaseRoleObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.DatabaseRoleObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterDatabaseRoleOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.DatabaseObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.DatabaseRole, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.DatabaseRole, obs *reconciler.Observation[*snowflake.DatabaseRoleObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, role *snowplanev1alpha1.DatabaseRole) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, role,
				role.Namespace, role.Spec.DatabaseRef, role.Spec.DatabaseName, role.Status.DatabaseName)
			if err != nil {
				return err
			}

			role.Status.DatabaseName = dbFQN

			refresolver.SetDatabaseResolvedCondition(role, role.Spec.DatabaseRef, role.Spec.DatabaseName, dbFQN)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.DatabaseRoleList{} }, ".spec.databaseRef.name", "listing database roles for database watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, role *snowplanev1alpha1.DatabaseRole) error {
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

func applyObservation(role *snowplanev1alpha1.DatabaseRole, obs *snowflake.DatabaseRoleObservation) {
	if obs.ShowOutput != nil {
		role.Status.FullyQualifiedName = snowflake.NewDatabaseObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		role.Status.DatabaseName = obs.ShowOutput.DatabaseName

		role.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(role *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier) snowflake.CreateDatabaseRoleOptions {
	return snowflake.CreateDatabaseRoleOptions{
		Name:    id,
		Comment: role.Spec.Comment,
	}
}

func buildAlterOptions(role *snowplanev1alpha1.DatabaseRole, id snowflake.DatabaseObjectIdentifier, obs *snowflake.DatabaseRoleObservation) snowflake.AlterDatabaseRoleOptions {
	opts := snowflake.AlterDatabaseRoleOptions{Name: id}

	// Detect fields that were previously managed but are now nil -> UNSET.
	opts.UnsetFields = tracked.ComputeUnset(&role.Spec, role.Status.TrackedParameters)

	if role.Spec.Comment != nil {
		if obs.ShowOutput == nil || *role.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = role.Spec.Comment
		}
	}

	return opts
}

// detectDrift compares desired spec against the observed state and
// returns a structured drift result.
func detectDrift(role *snowplanev1alpha1.DatabaseRole, obs *snowflake.DatabaseRoleObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", role.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(role.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)

		// Mutable fields.
		d.CompareString("COMMENT", role.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
