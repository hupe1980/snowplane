// Package cortexsearchservice implements the reconciler for CortexSearchService resources.
package cortexsearchservice

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
	finalizerName = "snowplane.hupe1980.github.io/cortexsearchservice"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake Cortex Search Services.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error)
	Create(ctx context.Context, opts snowflake.CreateCortexSearchServiceOptions) error
	Alter(ctx context.Context, opts snowflake.AlterCortexSearchServiceOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new CortexSearchService reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.CortexSearchService, Service, *snowflake.CortexSearchServiceObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewCortexSearchServiceClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.CortexSearchService, Service, *snowflake.CortexSearchServiceObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for CortexSearchService resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.CortexSearchService, Service, *snowflake.CortexSearchServiceObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.CortexSearchService, Service, *snowflake.CortexSearchServiceObservation]{
		ResourceNameVal:  "cortexsearchservice",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.CortexSearchService { return &snowplanev1alpha1.CortexSearchService{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.CortexSearchService) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.CortexSearchServiceObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.CortexSearchServiceObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.CortexSearchService, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterCortexSearchServiceOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.CortexSearchService, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.CortexSearchServiceObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.CortexSearchService, obs *reconciler.Observation[*snowflake.CortexSearchServiceObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.CortexSearchService, obs *reconciler.Observation[*snowflake.CortexSearchServiceObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.CortexSearchService) error {
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

			warehouseName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, obj,
				obj.Namespace, obj.Spec.WarehouseRef, obj.Spec.WarehouseName, obj.Status.WarehouseName,
				"Warehouse",
				func() *snowplanev1alpha1.Warehouse { return &snowplanev1alpha1.Warehouse{} },
				snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
				func(w *snowplanev1alpha1.Warehouse) string { return w.Spec.Name },
			)
			if err != nil {
				return err
			}

			obj.Status.WarehouseName = warehouseName

			refresolver.SetAllReferencesResolvedCondition(obj,
				refresolver.RefDescriptor{KindLabel: "Database", Ref: obj.Spec.DatabaseRef, RawName: obj.Spec.DatabaseName},
				refresolver.RefDescriptor{KindLabel: "Schema", Ref: obj.Spec.SchemaRef, RawName: obj.Spec.SchemaName},
				refresolver.RefDescriptor{KindLabel: "Warehouse", Ref: obj.Spec.WarehouseRef, RawName: obj.Spec.WarehouseName},
			)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.CortexSearchService{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.CortexSearchService)
					if !ok || obj.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{obj.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.CortexSearchService{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.CortexSearchService)
					if !ok || obj.Spec.SchemaRef == nil {
						return nil
					}

					return []string{obj.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.CortexSearchServiceList{} }, ".spec.databaseRef.name", "listing cortex search services for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.CortexSearchServiceList{} }, ".spec.schemaRef.name", "listing cortex search services for schema watch")),
			)

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.CortexSearchService{},
				".spec.warehouseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.CortexSearchService)
					if !ok || obj.Spec.WarehouseRef == nil {
						return nil
					}

					return []string{obj.Spec.WarehouseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.warehouseRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Warehouse{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.CortexSearchServiceList{} }, ".spec.warehouseRef.name", "listing cortex search services for warehouse watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.CortexSearchService) error {
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

		// searchColumn is immutable — set at CREATE time via ON clause.
		if obj.Status.ShowOutput.SearchColumn != "" {
			if !strings.EqualFold(obj.Spec.On, obj.Status.ShowOutput.SearchColumn) {
				return fmt.Errorf("spec.on is immutable after creation (current: %q, desired: %q)", obj.Status.ShowOutput.SearchColumn, obj.Spec.On)
			}
		}
	}

	return nil
}

func applyObservation(obj *snowplanev1alpha1.CortexSearchService, obs *snowflake.CortexSearchServiceObservation) {
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

	if obs.DescribeOutput != nil {
		obj.Status.DescribeOutput = obs.DescribeOutput
	}
}

func buildCreateOptions(obj *snowplanev1alpha1.CortexSearchService, id snowflake.SchemaObjectIdentifier) snowflake.CreateCortexSearchServiceOptions {
	opts := snowflake.CreateCortexSearchServiceOptions{
		Name:                       id,
		On:                         obj.Spec.On,
		Attributes:                 obj.Spec.Attributes,
		Warehouse:                  obj.Status.WarehouseName,
		TargetLag:                  obj.Spec.TargetLag,
		Query:                      obj.Spec.Query,
		EmbeddingModel:             obj.Spec.EmbeddingModel,
		FullIndexBuildIntervalDays: obj.Spec.FullIndexBuildIntervalDays,
		Comment:                    obj.Spec.Comment,
	}

	if obj.Spec.RefreshMode != nil {
		rm := string(*obj.Spec.RefreshMode)
		opts.RefreshMode = &rm
	}

	if obj.Spec.Initialize != nil {
		init := string(*obj.Spec.Initialize)
		opts.Initialize = &init
	}

	return opts
}

func buildAlterOptions(obj *snowplanev1alpha1.CortexSearchService, id snowflake.SchemaObjectIdentifier, obs *snowflake.CortexSearchServiceObservation) snowflake.AlterCortexSearchServiceOptions {
	opts := snowflake.AlterCortexSearchServiceOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Target lag — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(obj.Spec.TargetLag, obs.ShowOutput.TargetLag) {
		tl := obj.Spec.TargetLag
		opts.TargetLag = &tl
	}

	// Warehouse — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(obj.Status.WarehouseName, obs.ShowOutput.Warehouse) {
		wh := obj.Status.WarehouseName
		opts.Warehouse = &wh
	}

	// Comment — set if changed.
	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// FullIndexBuildIntervalDays — always send if specified.
	if obj.Spec.FullIndexBuildIntervalDays != nil {
		opts.FullIndexBuildIntervalDays = obj.Spec.FullIndexBuildIntervalDays
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.CortexSearchService, obs *snowflake.CortexSearchServiceObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValueFold("SEARCH_COLUMN", obj.Spec.On, obs.ShowOutput.SearchColumn, true)

		// Mutable fields.
		d.CompareStringValueFold("TARGET_LAG", obj.Spec.TargetLag, obs.ShowOutput.TargetLag, false)
		d.CompareStringValueFold("WAREHOUSE", obj.Status.WarehouseName, obs.ShowOutput.Warehouse, false)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
	}

	return d.Result()
}
