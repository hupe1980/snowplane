// Package streamondirectorytable implements the reconciler for StreamOnDirectoryTable resources.
package streamondirectorytable

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
	finalizerName = "snowplane.hupe1980.github.io/streamondirectorytable"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake streams.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStreamOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStreamOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new StreamOnDirectoryTable reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDirectoryTable, Service, *snowflake.StreamObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewStreamClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnDirectoryTable, Service, *snowflake.StreamObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for StreamOnDirectoryTable resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.StreamOnDirectoryTable, Service, *snowflake.StreamObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.StreamOnDirectoryTable, Service, *snowflake.StreamObservation]{
		ResourceNameVal:  "streamondirectorytable",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.StreamOnDirectoryTable { return &snowplanev1alpha1.StreamOnDirectoryTable{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.StreamOnDirectoryTable) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)

			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.StreamObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.StreamOnDirectoryTable, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterStreamOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.StreamOnDirectoryTable, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.StreamObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.StreamOnDirectoryTable, obs *reconciler.Observation[*snowflake.StreamObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.StreamOnDirectoryTable, obs *reconciler.Observation[*snowflake.StreamObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.StreamOnDirectoryTable) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.DatabaseRef, obj.Spec.DatabaseName, obj.Status.DatabaseName)
			if err != nil {
				return err
			}

			obj.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.SchemaRef, obj.Spec.SchemaName, obj.Status.SchemaName)
			if err != nil {
				return err
			}

			obj.Status.SchemaName = schemaFQN

			stageName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.StageRef, obj.Spec.StageName, obj.Status.StageName,
				"Stage",
				func() *snowplanev1alpha1.Stage { return &snowplanev1alpha1.Stage{} },
				snowplanev1alpha1.GroupVersion.WithKind("Stage"),
				func(st *snowplanev1alpha1.Stage) string { return st.Spec.Name },
			)
			if err != nil {
				return err
			}

			obj.Status.StageName = stageName

			refresolver.SetAllReferencesResolvedCondition(obj,
				refresolver.RefDescriptor{KindLabel: "Database", Ref: obj.Spec.DatabaseRef, RawName: obj.Spec.DatabaseName},
				refresolver.RefDescriptor{KindLabel: "Schema", Ref: obj.Spec.SchemaRef, RawName: obj.Spec.SchemaName},
				refresolver.RefDescriptor{KindLabel: "Stage", Ref: obj.Spec.StageRef, RawName: obj.Spec.StageName},
			)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.StreamOnDirectoryTable{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnDirectoryTable)
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
				&snowplanev1alpha1.StreamOnDirectoryTable{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnDirectoryTable)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnDirectoryTableList{} }, ".spec.databaseRef.name", "listing stream-on-directory-table for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnDirectoryTableList{} }, ".spec.schemaRef.name", "listing stream-on-directory-table for schema watch")),
			)

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.StreamOnDirectoryTable{},
				".spec.stageRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnDirectoryTable)
					if !ok || s.Spec.StageRef == nil {
						return nil
					}

					return []string{s.Spec.StageRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.stageRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Stage{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnDirectoryTableList{} }, ".spec.stageRef.name", "listing stream-on-directory-table for stage watch")),
			)

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.StreamOnDirectoryTable) error {
	if reconciler.ShouldSkipImmutableValidation(obj) {
		return nil
	}

	if obj.Status.ShowOutput != nil {
		if obj.Status.ShowOutput.Name != "" && !strings.EqualFold(obj.Spec.Name, obj.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.Name, obj.Spec.Name)
		}

		if obj.Status.ShowOutput.DatabaseName != "" && obj.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, obj.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", obj.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if obj.Status.ShowOutput.SchemaName != "" && obj.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, obj.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", obj.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.StreamOnDirectoryTable, obs *snowflake.StreamObservation) {
	if obs.ShowOutput != nil {
		obj.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		obj.Status.DatabaseName = obs.ShowOutput.DatabaseName
		obj.Status.SchemaName = obs.ShowOutput.SchemaName

		obj.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.StreamOnDirectoryTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateStreamOptions {
	return snowflake.CreateStreamOptions{
		Name:       id,
		SourceType: snowflake.StreamSourceStage,
		SourceName: obj.Status.StageName,
		Comment:    obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.StreamOnDirectoryTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.StreamObservation) snowflake.AlterStreamOptions {
	opts := snowflake.AlterStreamOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.StreamOnDirectoryTable, obs *snowflake.StreamObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("SOURCE", obj.Status.StageName, obs.ShowOutput.TableName, true)
		d.CompareStringValueFold("MODE", "DEFAULT", obs.ShowOutput.Mode, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
