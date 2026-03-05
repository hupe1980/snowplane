// Package dynamictable implements the reconciler for DynamicTable resources.
package dynamictable

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
	finalizerName = "snowplane.hupe1980.github.io/dynamictable"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake dynamic tables.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error)
	Create(ctx context.Context, opts snowflake.CreateDynamicTableOptions) error
	Alter(ctx context.Context, opts snowflake.AlterDynamicTableOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new DynamicTable reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewDynamicTableClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for DynamicTable resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.DynamicTable, Service, *snowflake.DynamicTableObservation]{
		ResourceNameVal:  "dynamictable",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.DynamicTable { return &snowplanev1alpha1.DynamicTable{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(dt *snowplanev1alpha1.DynamicTable) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, dt.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.DynamicTableObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.DynamicTableObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterDynamicTableOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.DynamicTable, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.DynamicTable, obs *reconciler.Observation[*snowflake.DynamicTableObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, dt *snowplanev1alpha1.DynamicTable) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, dt,
				dt.Namespace, dt.Spec.DatabaseRef, dt.Spec.DatabaseName, dt.Status.DatabaseName)
			if err != nil {
				return err
			}

			dt.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, dt,
				dt.Namespace, dt.Spec.SchemaRef, dt.Spec.SchemaName, dt.Status.SchemaName)
			if err != nil {
				return err
			}

			dt.Status.SchemaName = schemaFQN

			warehouseName, err := refresolver.PreReconcileSourceRef(ctx, c, recorder, dt,
				dt.Namespace, dt.Spec.WarehouseRef, dt.Spec.WarehouseName, dt.Status.WarehouseName,
				"Warehouse",
				func() *snowplanev1alpha1.Warehouse { return &snowplanev1alpha1.Warehouse{} },
				snowplanev1alpha1.GroupVersion.WithKind("Warehouse"),
				func(w *snowplanev1alpha1.Warehouse) string { return w.Spec.Name },
			)
			if err != nil {
				return err
			}

			dt.Status.WarehouseName = warehouseName

			refresolver.SetAllReferencesResolvedCondition(dt,
				refresolver.RefDescriptor{KindLabel: "Database", Ref: dt.Spec.DatabaseRef, RawName: dt.Spec.DatabaseName},
				refresolver.RefDescriptor{KindLabel: "Schema", Ref: dt.Spec.SchemaRef, RawName: dt.Spec.SchemaName},
				refresolver.RefDescriptor{KindLabel: "Warehouse", Ref: dt.Spec.WarehouseRef, RawName: dt.Spec.WarehouseName},
			)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.DynamicTable{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					dt, ok := o.(*snowplanev1alpha1.DynamicTable)
					if !ok || dt.Spec.DatabaseRef == nil {
						return nil
					}

					return []string{dt.Spec.DatabaseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.databaseRef.name: %w", err)
			}

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.DynamicTable{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					dt, ok := o.(*snowplanev1alpha1.DynamicTable)
					if !ok || dt.Spec.SchemaRef == nil {
						return nil
					}

					return []string{dt.Spec.SchemaRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.schemaRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Database{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }, ".spec.databaseRef.name", "listing dynamic tables for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }, ".spec.schemaRef.name", "listing dynamic tables for schema watch")),
			)

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.DynamicTable{},
				".spec.warehouseRef.name",
				func(o sigs.Object) []string {
					dt, ok := o.(*snowplanev1alpha1.DynamicTable)
					if !ok || dt.Spec.WarehouseRef == nil {
						return nil
					}

					return []string{dt.Spec.WarehouseRef.Name}
				},
			); err != nil {
				return fmt.Errorf("creating field indexer for .spec.warehouseRef.name: %w", err)
			}

			bldr.Watches(
				&snowplanev1alpha1.Warehouse{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.DynamicTableList{} }, ".spec.warehouseRef.name", "listing dynamic tables for warehouse watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, dt *snowplanev1alpha1.DynamicTable) error {
	if reconciler.ShouldSkipImmutableValidation(dt) {
		return nil
	}

	if dt.Status.ShowOutput != nil {
		if dt.Status.ShowOutput.Name != "" && !strings.EqualFold(dt.Spec.Name, dt.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.Name, dt.Spec.Name)
		}

		if dt.Status.ShowOutput.DatabaseName != "" && dt.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, dt.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", dt.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if dt.Status.ShowOutput.SchemaName != "" && dt.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, dt.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", dt.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		// query is immutable — defines the dynamic table's SQL content.
		if dt.Status.ShowOutput.Text != "" && dt.Spec.Query != dt.Status.ShowOutput.Text {
			return fmt.Errorf("spec.query is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.Text, dt.Spec.Query)
		}

		// refreshMode is immutable — set at CREATE time only.
		if dt.Status.ShowOutput.RefreshMode != "" && dt.Spec.RefreshMode != nil {
			if !strings.EqualFold(string(*dt.Spec.RefreshMode), dt.Status.ShowOutput.RefreshMode) {
				return fmt.Errorf("spec.refreshMode is immutable after creation (current: %q, desired: %q)", dt.Status.ShowOutput.RefreshMode, *dt.Spec.RefreshMode)
			}
		}
	}

	return nil
}

func applyObservation(dt *snowplanev1alpha1.DynamicTable, obs *snowflake.DynamicTableObservation) {
	if obs.ShowOutput != nil {
		dt.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		dt.Status.DatabaseName = obs.ShowOutput.DatabaseName
		dt.Status.SchemaName = obs.ShowOutput.SchemaName

		dt.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(dt *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateDynamicTableOptions {
	opts := snowflake.CreateDynamicTableOptions{
		Name:                       id,
		Query:                      dt.Spec.Query,
		TargetLag:                  dt.Spec.TargetLag,
		Warehouse:                  dt.Status.WarehouseName,
		Comment:                    dt.Spec.Comment,
		Transient:                  dt.Spec.Transient,
		ClusterBy:                  dt.Spec.ClusterBy,
		DataRetentionTimeInDays:    dt.Spec.DataRetentionTimeInDays,
		MaxDataExtensionTimeInDays: dt.Spec.MaxDataExtensionTimeInDays,
	}

	if dt.Spec.RefreshMode != nil {
		rm := string(*dt.Spec.RefreshMode)
		opts.RefreshMode = &rm
	}

	if dt.Spec.Initialize != nil {
		init := string(*dt.Spec.Initialize)
		opts.Initialize = &init
	}

	return opts
}

func buildAlterOptions(dt *snowplanev1alpha1.DynamicTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.DynamicTableObservation) snowflake.AlterDynamicTableOptions {
	opts := snowflake.AlterDynamicTableOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&dt.Spec, dt.Status.TrackedParameters)

	// Target lag — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(dt.Spec.TargetLag, obs.ShowOutput.TargetLag) {
		tl := dt.Spec.TargetLag
		opts.TargetLag = &tl
	}

	// Warehouse — always send if it differs.
	if obs.ShowOutput != nil && !strings.EqualFold(dt.Status.WarehouseName, obs.ShowOutput.Warehouse) {
		wh := dt.Status.WarehouseName
		opts.Warehouse = &wh
	}

	// Comment — set if changed.
	if dt.Spec.Comment != nil {
		if obs.ShowOutput == nil || *dt.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = dt.Spec.Comment
		}
	}

	// ClusterBy — set if specified and changed, or drop if previously managed but now removed.
	if len(dt.Spec.ClusterBy) > 0 {
		specCluster := strings.Join(dt.Spec.ClusterBy, ", ")
		if obs.ShowOutput == nil || specCluster != obs.ShowOutput.ClusterBy {
			opts.ClusterBy = dt.Spec.ClusterBy
		}
	} else {
		for _, p := range dt.Status.TrackedParameters {
			if p == "CLUSTER_BY" {
				opts.UnsetClusterBy = true
				break
			}
		}
	}

	// DataRetentionTimeInDays — always send if specified.
	if dt.Spec.DataRetentionTimeInDays != nil {
		opts.DataRetentionTimeInDays = dt.Spec.DataRetentionTimeInDays
	}

	// MaxDataExtensionTimeInDays — always send if specified.
	if dt.Spec.MaxDataExtensionTimeInDays != nil {
		opts.MaxDataExtensionTimeInDays = dt.Spec.MaxDataExtensionTimeInDays
	}

	return opts
}

func detectDrift(dt *snowplanev1alpha1.DynamicTable, obs *snowflake.DynamicTableObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields — cannot be changed via ALTER.
		d.CompareStringValueFold("NAME", dt.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(dt.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(dt.Status.SchemaName), obs.ShowOutput.SchemaName, true)
		d.CompareStringValue("QUERY", dt.Spec.Query, obs.ShowOutput.Text, true)

		if dt.Spec.RefreshMode != nil {
			d.CompareStringValueFold("REFRESH_MODE", string(*dt.Spec.RefreshMode), obs.ShowOutput.RefreshMode, true)
		}

		// Mutable fields.
		d.CompareStringValueFold("TARGET_LAG", dt.Spec.TargetLag, obs.ShowOutput.TargetLag, false)
		d.CompareStringValueFold("WAREHOUSE", dt.Status.WarehouseName, obs.ShowOutput.Warehouse, false)
		d.CompareString("COMMENT", dt.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareStringValue("CLUSTER_BY", strings.Join(dt.Spec.ClusterBy, ", "), obs.ShowOutput.ClusterBy, false)
	}

	return d.Result()
}
