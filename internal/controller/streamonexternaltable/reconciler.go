// Package streamonexternaltable implements the reconciler for StreamOnExternalTable resources.
package streamonexternaltable

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
	finalizerName = "snowplane.hupe1980.github.io/streamonexternaltable"
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

// NewReconciler returns a new StreamOnExternalTable reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnExternalTable, Service, *snowflake.StreamObservation] {
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.StreamOnExternalTable, Service, *snowflake.StreamObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for StreamOnExternalTable resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.StreamOnExternalTable, Service, *snowflake.StreamObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.StreamOnExternalTable, Service, *snowflake.StreamObservation]{
		ResourceNameVal:  "streamonexternaltable",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.StreamOnExternalTable { return &snowplanev1alpha1.StreamOnExternalTable{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.StreamOnExternalTable) (reconciler.Identifier, error) {
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
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.StreamOnExternalTable, id snowflake.SchemaObjectIdentifier) error {
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
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.StreamOnExternalTable, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.StreamObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.StreamOnExternalTable, obs *reconciler.Observation[*snowflake.StreamObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.StreamOnExternalTable, obs *reconciler.Observation[*snowflake.StreamObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.StreamOnExternalTable) error {
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

			externalTableName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.ExternalTableRef, obj.Spec.ExternalTableName, obj.Status.ExternalTableName,
				"ExternalTable",
				func() *snowplanev1alpha1.ExternalTable { return &snowplanev1alpha1.ExternalTable{} },
				snowplanev1alpha1.GroupVersion.WithKind("ExternalTable"),
				func(et *snowplanev1alpha1.ExternalTable) string { return et.Spec.Name },
			)
			if err != nil {
				return err
			}

			obj.Status.ExternalTableName = externalTableName

			refresolver.SetAllReferencesResolvedCondition(obj,
				refresolver.RefDescriptor{KindLabel: "Database", Ref: obj.Spec.DatabaseRef, RawName: obj.Spec.DatabaseName},
				refresolver.RefDescriptor{KindLabel: "Schema", Ref: obj.Spec.SchemaRef, RawName: obj.Spec.SchemaName},
				refresolver.RefDescriptor{KindLabel: "ExternalTable", Ref: obj.Spec.ExternalTableRef, RawName: obj.Spec.ExternalTableName},
			)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.StreamOnExternalTable{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnExternalTable)
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
				&snowplanev1alpha1.StreamOnExternalTable{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnExternalTable)
					if !ok || s.Spec.SchemaRef == nil {
						return nil
					}

					return []string{s.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.StreamOnExternalTable{},
				".spec.externalTableRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.StreamOnExternalTable)
					if !ok || s.Spec.ExternalTableRef == nil {
						return nil
					}

					return []string{s.Spec.ExternalTableRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.externalTableRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnExternalTableList{} }, ".spec.databaseRef.name", "listing stream-on-external-table for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnExternalTableList{} }, ".spec.schemaRef.name", "listing stream-on-external-table for schema watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.ExternalTable{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamOnExternalTableList{} }, ".spec.externalTableRef.name", "listing stream-on-external-table for external-table watch")),
			)

			return nil
		},
	}
}

// validateImmutableFields checks that immutable fields have not changed.
func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.StreamOnExternalTable) error {
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

func applyObservation(obj *snowplanev1alpha1.StreamOnExternalTable, obs *snowflake.StreamObservation) {
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

func buildCreateOptions(obj *snowplanev1alpha1.StreamOnExternalTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateStreamOptions {
	return snowflake.CreateStreamOptions{
		Name:       id,
		SourceType: snowflake.StreamSourceExternalTable,
		SourceName: obj.Status.ExternalTableName,
		InsertOnly: obj.Spec.InsertOnly,
		Comment:    obj.Spec.Comment,
	}
}

func buildAlterOptions(obj *snowplanev1alpha1.StreamOnExternalTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.StreamObservation) snowflake.AlterStreamOptions {
	opts := snowflake.AlterStreamOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.StreamOnExternalTable, obs *snowflake.StreamObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("SOURCE", obj.Status.ExternalTableName, obs.ShowOutput.TableName, true)

		expectedMode := "DEFAULT"
		if obj.Spec.InsertOnly != nil && *obj.Spec.InsertOnly {
			expectedMode = "INSERT_ONLY"
		}

		d.CompareStringValueFold("MODE", expectedMode, obs.ShowOutput.Mode, true)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
