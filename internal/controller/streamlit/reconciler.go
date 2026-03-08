// Package streamlit implements the reconciler for Streamlit resources.
package streamlit

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
	finalizerName = "snowplane.hupe1980.github.io/streamlit"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake Streamlits.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error)
	Create(ctx context.Context, opts snowflake.CreateStreamlitOptions) error
	Alter(ctx context.Context, opts snowflake.AlterStreamlitOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new Streamlit reconciler backed by the generic framework.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.Streamlit, Service, *snowflake.StreamlitObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewStreamlitClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.Streamlit, Service, *snowflake.StreamlitObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for Streamlit resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.Streamlit, Service, *snowflake.StreamlitObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.Streamlit, Service, *snowflake.StreamlitObservation]{
		ResourceNameVal:  "streamlit",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.Streamlit { return &snowplanev1alpha1.Streamlit{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(obj *snowplanev1alpha1.Streamlit) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, obj.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.StreamlitObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.StreamlitObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.Streamlit, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterStreamlitOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.Streamlit, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.StreamlitObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.Streamlit, obs *reconciler.Observation[*snowflake.StreamlitObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.Streamlit, obs *reconciler.Observation[*snowflake.StreamlitObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		LateInitializeFn: lateInitialize,
		PreReconcileFn: func(ctx context.Context, obj *snowplanev1alpha1.Streamlit) error {
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
				&snowplanev1alpha1.Streamlit{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.Streamlit)
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
				&snowplanev1alpha1.Streamlit{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.Streamlit)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamlitList{} }, ".spec.databaseRef.name", "listing streamlits for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamlitList{} }, ".spec.schemaRef.name", "listing streamlits for schema watch")),
			)

			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.Streamlit{},
				".spec.warehouseRef.name",
				func(o sigs.Object) []string {
					obj, ok := o.(*snowplanev1alpha1.Streamlit)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.StreamlitList{} }, ".spec.warehouseRef.name", "listing streamlits for warehouse watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, obj *snowplanev1alpha1.Streamlit) error {
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

func applyObservation(obj *snowplanev1alpha1.Streamlit, obs *snowflake.StreamlitObservation) {
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

func buildCreateOptions(obj *snowplanev1alpha1.Streamlit, id snowflake.SchemaObjectIdentifier) snowflake.CreateStreamlitOptions {
	opts := snowflake.CreateStreamlitOptions{
		Name:                       id,
		From:                       obj.Spec.From,
		MainFile:                   obj.Spec.MainFile,
		Comment:                    obj.Spec.Comment,
		Title:                      obj.Spec.Title,
		ExternalAccessIntegrations: obj.Spec.ExternalAccessIntegrations,
	}

	// Use resolved warehouse name from status.
	if obj.Status.WarehouseName != "" {
		opts.QueryWarehouse = &obj.Status.WarehouseName
	}

	return opts
}

func buildAlterOptions(obj *snowplanev1alpha1.Streamlit, id snowflake.SchemaObjectIdentifier, obs *snowflake.StreamlitObservation) snowflake.AlterStreamlitOptions {
	opts := snowflake.AlterStreamlitOptions{Name: id}
	opts.UnsetFields = tracked.ComputeUnset(&obj.Spec, obj.Status.TrackedParameters)

	// Query warehouse — send if it differs.
	if obs.ShowOutput != nil && obj.Status.WarehouseName != "" &&
		!strings.EqualFold(obj.Status.WarehouseName, obs.ShowOutput.QueryWarehouse) {
		wh := obj.Status.WarehouseName
		opts.QueryWarehouse = &wh
	}

	// Main file — use describe output for comparison.
	if obj.Spec.MainFile != nil && obs.DescribeOutput != nil {
		if !strings.EqualFold(*obj.Spec.MainFile, obs.DescribeOutput.MainFile) {
			opts.MainFile = obj.Spec.MainFile
		}
	}

	// Comment — set if changed.
	if obj.Spec.Comment != nil {
		if obs.ShowOutput == nil || *obj.Spec.Comment != obs.ShowOutput.Comment {
			opts.Comment = obj.Spec.Comment
		}
	}

	// Title — set if changed.
	if obj.Spec.Title != nil {
		if obs.ShowOutput == nil || *obj.Spec.Title != obs.ShowOutput.Title {
			opts.Title = obj.Spec.Title
		}
	}

	// External access integrations — set if changed.
	if len(obj.Spec.ExternalAccessIntegrations) > 0 {
		if obs.DescribeOutput == nil {
			eai := obj.Spec.ExternalAccessIntegrations
			opts.ExternalAccessIntegrations = &eai
		} else {
			actualEAI := parseCommaList(obs.DescribeOutput.ExternalAccessIntegrations)
			if !stringSliceEqualFold(obj.Spec.ExternalAccessIntegrations, actualEAI) {
				eai := obj.Spec.ExternalAccessIntegrations
				opts.ExternalAccessIntegrations = &eai
			}
		}
	}

	return opts
}

func detectDrift(obj *snowplanev1alpha1.Streamlit, obs *snowflake.StreamlitObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", obj.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(obj.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(obj.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Mutable fields.
		d.CompareStringValueFold("QUERY_WAREHOUSE", obj.Status.WarehouseName, obs.ShowOutput.QueryWarehouse, false)
		d.CompareString("COMMENT", obj.Spec.Comment, obs.ShowOutput.Comment, false)
		d.CompareString("TITLE", obj.Spec.Title, obs.ShowOutput.Title, false)

		// External access integrations.
		if obs.DescribeOutput != nil {
			actualEAI := parseCommaList(obs.DescribeOutput.ExternalAccessIntegrations)
			d.CompareStringSliceFold("EXTERNAL_ACCESS_INTEGRATIONS", obj.Spec.ExternalAccessIntegrations, actualEAI, false)
		}
	}

	return d.Result()
}

// parseCommaList splits a comma-separated string, trims whitespace, and returns non-empty values.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}

	return result
}

// stringSliceEqualFold compares two string slices using case-insensitive, order-independent comparison.
func stringSliceEqualFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[strings.ToUpper(s)]++
	}

	for _, s := range b {
		key := strings.ToUpper(s)
		if seen[key] <= 0 {
			return false
		}
		seen[key]--
	}

	return true
}
