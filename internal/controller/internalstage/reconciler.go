// Package internalstage implements the reconciler for InternalStage resources.
package internalstage

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
	finalizerName = "snowplane.hupe1980.github.io/internalstage"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake internal stages.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.InternalStageObservation, error)
	Create(ctx context.Context, opts snowflake.CreateInternalStageOptions) error
	Alter(ctx context.Context, opts snowflake.AlterInternalStageOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new InternalStage reconciler.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.InternalStage, Service, *snowflake.InternalStageObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewInternalStageClient(exec)
		}),
	)
}

// NewReconcilerWithServiceFactory is like NewReconciler but lets the caller
// supply a custom ServiceFactory for testing.
func NewReconcilerWithServiceFactory(
	c sigs.Client,
	factory *clientfactory.ClientFactory,
	recorder record.EventRecorder,
	rl *ratelimit.Limiter,
	sf ServiceFactory,
) *reconciler.GenericReconciler[*snowplanev1alpha1.InternalStage, Service, *snowflake.InternalStageObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for InternalStage resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.InternalStage, Service, *snowflake.InternalStageObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.InternalStage, Service, *snowflake.InternalStageObservation]{
		ResourceNameVal:  "internalstage",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.InternalStage { return &snowplanev1alpha1.InternalStage{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(stage *snowplanev1alpha1.InternalStage) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, stage.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.InternalStageObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.InternalStageObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.InternalStage, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterInternalStageOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.InternalStage, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.InternalStageObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.InternalStage, obs *reconciler.Observation[*snowflake.InternalStageObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.InternalStage, obs *reconciler.Observation[*snowflake.InternalStageObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, stage *snowplanev1alpha1.InternalStage) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, stage,
				stage.Namespace, stage.Spec.DatabaseRef, stage.Spec.DatabaseName, stage.Status.DatabaseName)
			if err != nil {
				return err
			}

			stage.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, stage,
				stage.Namespace, stage.Spec.SchemaRef, stage.Spec.SchemaName, stage.Status.SchemaName)
			if err != nil {
				return err
			}

			stage.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(stage, stage.Spec.DatabaseRef, stage.Spec.DatabaseName, stage.Spec.SchemaRef, stage.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.InternalStage{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.InternalStage)
					if !ok || s.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{s.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.InternalStage{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.InternalStage)
					if !ok || s.Spec.SchemaRef == nil {
						return nil
					}

					return []string{s.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.InternalStageList{} }, ".spec.databaseRef.name", "listing internal stages for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.InternalStageList{} }, ".spec.schemaRef.name", "listing internal stages for schema watch")),
			)

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, stage *snowplanev1alpha1.InternalStage) error {
	if reconciler.ShouldSkipImmutableValidation(stage) {
		return nil
	}

	if stage.Status.ShowOutput != nil {
		if stage.Status.ShowOutput.Name != "" && !strings.EqualFold(stage.Spec.Name, stage.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", stage.Status.ShowOutput.Name, stage.Spec.Name)
		}

		if stage.Status.ShowOutput.DatabaseName != "" && stage.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, stage.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", stage.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if stage.Status.ShowOutput.SchemaName != "" && stage.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, stage.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", stage.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(stage *snowplanev1alpha1.InternalStage, obs *snowflake.InternalStageObservation) {
	if obs.ShowOutput != nil {
		stage.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		stage.Status.DatabaseName = obs.ShowOutput.DatabaseName
		stage.Status.SchemaName = obs.ShowOutput.SchemaName

		stage.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(stage *snowplanev1alpha1.InternalStage, id snowflake.SchemaObjectIdentifier) snowflake.CreateInternalStageOptions {
	opts := snowflake.CreateInternalStageOptions{
		Name:       id,
		FileFormat: stage.Spec.FileFormat,
		Comment:    stage.Spec.Comment,
	}

	if stage.Spec.Encryption != nil {
		opts.Encryption = &snowflake.InternalStageEncryptionOptions{
			Type: stage.Spec.Encryption.Type,
		}
	}

	if stage.Spec.Directory != nil {
		opts.Directory = &snowflake.InternalStageDirectoryCreateOptions{
			Enable:          stage.Spec.Directory.Enable,
			RefreshOnCreate: stage.Spec.Directory.RefreshOnCreate,
		}
	}

	return opts
}

func buildAlterOptions(stage *snowplanev1alpha1.InternalStage, id snowflake.SchemaObjectIdentifier, obs *snowflake.InternalStageObservation) snowflake.AlterInternalStageOptions {
	opts := snowflake.AlterInternalStageOptions{Name: id}

	if stage.Spec.Comment != nil {
		if obs.ShowOutput == nil || *stage.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = stage.Spec.Comment
		}
	}

	if stage.Spec.FileFormat != nil {
		opts.FileFormat = stage.Spec.FileFormat
	}

	if stage.Spec.Directory != nil {
		opts.Directory = &snowflake.InternalStageDirectoryCreateOptions{
			Enable: stage.Spec.Directory.Enable,
		}
	}

	opts.UnsetFields = tracked.ComputeUnset(&stage.Spec, stage.Status.TrackedParameters)

	return opts
}

func detectDrift(stage *snowplanev1alpha1.InternalStage, obs *snowflake.InternalStageObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", stage.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(stage.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(stage.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareString("COMMENT", stage.Spec.Comment, obs.ShowOutput.Comment, false)

		// Directory enabled — drift-detect when the user specified a directory config.
		if stage.Spec.Directory != nil {
			d.CompareBoolValue("DIRECTORY_ENABLED", stage.Spec.Directory.Enable, obs.ShowOutput.DirectoryEnabled, false)
		}
	}

	return d.Result()
}
