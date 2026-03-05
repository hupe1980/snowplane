// Package externaltable implements the reconciler for ExternalTable resources.
package externaltable

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
)

const (
	finalizerName = "snowplane.hupe1980.github.io/external-table"
)

// SnowflakeClient is the Snowflake client interface used by this package.
type SnowflakeClient = clientfactory.SnowflakeClient

// Service defines operations the reconciler needs against Snowflake external tables.
type Service interface {
	Observe(ctx context.Context, name snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error)
	Create(ctx context.Context, opts snowflake.CreateExternalTableOptions) error
	Alter(ctx context.Context, opts snowflake.AlterExternalTableOptions) error
	Drop(ctx context.Context, name snowflake.SchemaObjectIdentifier) error
}

// ServiceFactory creates a Service from a Snowflake client.
type ServiceFactory func(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error)

// NewReconciler returns a new ExternalTable reconciler.
func NewReconciler(c sigs.Client, factory *clientfactory.ClientFactory, recorder record.EventRecorder, rl *ratelimit.Limiter) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] {
	return NewReconcilerWithServiceFactory(c, factory, recorder, rl,
		reconciler.MakeServiceFactory(func(exec snowflake.SQLExecutor) Service {
			return snowflake.NewExternalTableClient(exec)
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
) *reconciler.GenericReconciler[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] {
	return reconciler.NewGenericReconciler(c, factory, recorder, rl, newAdapter(c, recorder, sf))
}

// newAdapter creates the BaseAdapter for ExternalTable resources.
func newAdapter(c sigs.Client, recorder record.EventRecorder, sf ServiceFactory) *reconciler.BaseAdapter[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation] {
	return &reconciler.BaseAdapter[*snowplanev1alpha1.ExternalTable, Service, *snowflake.ExternalTableObservation]{
		ResourceNameVal:  "externaltable",
		FinalizerNameVal: finalizerName,
		NewObjectFn:      func() *snowplanev1alpha1.ExternalTable { return &snowplanev1alpha1.ExternalTable{} },
		ServiceFactoryFn: sf,
		BuildIdentifierFn: func(et *snowplanev1alpha1.ExternalTable) (reconciler.Identifier, error) {
			dbName := snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName)
			schemaName := snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName)
			return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, et.Spec.Name), nil
		},
		ObserveFn: reconciler.MakeObserve(
			func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) (*snowflake.ExternalTableObservation, error) {
				return svc.Observe(ctx, id)
			},
			func(obs *snowflake.ExternalTableObservation) bool { return obs.Exists },
		),
		CreateFn: reconciler.MakeCreate(func(ctx context.Context, svc Service, obj *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier) error {
			opts := buildCreateOptions(obj, id)
			return svc.Create(ctx, opts)
		}),
		AlterFn: reconciler.MakeAlter(func(ctx context.Context, svc Service, opts *snowflake.AlterExternalTableOptions) error {
			return svc.Alter(ctx, *opts)
		}),
		DropFn: reconciler.MakeDrop(func(ctx context.Context, svc Service, id snowflake.SchemaObjectIdentifier) error {
			return svc.Drop(ctx, id)
		}),
		ValidateImmutableFn: validateImmutableFields,
		BuildAlterOptsFn: reconciler.MakeBuildAlterOpts(func(_ context.Context, obj *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) (reconciler.AlterOptions, error) {
			opts := buildAlterOptions(obj, id, obs.Detail)
			return &opts, nil
		}),
		ApplyObservationFn: func(obj *snowplanev1alpha1.ExternalTable, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) {
			applyObservation(obj, obs.Detail)
		},
		DetectDriftFn: func(obj *snowplanev1alpha1.ExternalTable, obs *reconciler.Observation[*snowflake.ExternalTableObservation]) *drift.Result {
			return detectDrift(obj, obs.Detail)
		},
		PreReconcileFn: func(ctx context.Context, et *snowplanev1alpha1.ExternalTable) error {
			dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, c, recorder, et,
				et.Namespace, et.Spec.DatabaseRef, et.Spec.DatabaseName, et.Status.DatabaseName)
			if err != nil {
				return err
			}

			et.Status.DatabaseName = dbFQN

			schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, c, recorder, et,
				et.Namespace, et.Spec.SchemaRef, et.Spec.SchemaName, et.Status.SchemaName)
			if err != nil {
				return err
			}

			et.Status.SchemaName = schemaFQN

			refresolver.SetDatabaseAndSchemaResolvedCondition(et, et.Spec.DatabaseRef, et.Spec.DatabaseName, et.Spec.SchemaRef, et.Spec.SchemaName)

			return nil
		},
		SetupWatchesFn: func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
			if err := mgr.GetFieldIndexer().IndexField(
				ctx,
				&snowplanev1alpha1.ExternalTable{},
				".spec.databaseRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.ExternalTable)
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
				&snowplanev1alpha1.ExternalTable{},
				".spec.schemaRef.name",
				func(o sigs.Object) []string {
					s, ok := o.(*snowplanev1alpha1.ExternalTable)
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
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalTableList{} }, ".spec.databaseRef.name", "listing external tables for database watch")),
			)

			bldr.Watches(
				&snowplanev1alpha1.Schema{},
				handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(c, func() sigs.ObjectList { return &snowplanev1alpha1.ExternalTableList{} }, ".spec.schemaRef.name", "listing external tables for schema watch")),
			)

			return nil
		},
	}
}

func validateImmutableFields(_ context.Context, et *snowplanev1alpha1.ExternalTable) error {
	if reconciler.ShouldSkipImmutableValidation(et) {
		return nil
	}

	if et.Status.ShowOutput != nil {
		if et.Status.ShowOutput.Name != "" && !strings.EqualFold(et.Spec.Name, et.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", et.Status.ShowOutput.Name, et.Spec.Name)
		}

		if et.Status.ShowOutput.DatabaseName != "" && et.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, et.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", et.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if et.Status.ShowOutput.SchemaName != "" && et.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, et.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", et.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}
	}

	return nil
}

func applyObservation(et *snowplanev1alpha1.ExternalTable, obs *snowflake.ExternalTableObservation) {
	if obs.ShowOutput != nil {
		et.Status.FullyQualifiedName = snowflake.NewSchemaObjectIdentifier(
			obs.ShowOutput.DatabaseName,
			obs.ShowOutput.SchemaName,
			obs.ShowOutput.Name,
		).FullyQualifiedName()
		et.Status.DatabaseName = obs.ShowOutput.DatabaseName
		et.Status.SchemaName = obs.ShowOutput.SchemaName

		et.Status.ShowOutput = obs.ShowOutput
	}
}

func buildCreateOptions(et *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier) snowflake.CreateExternalTableOptions {
	opts := snowflake.CreateExternalTableOptions{
		Name:            id,
		Location:        et.Spec.Location,
		FileFormat:      et.Spec.FileFormat,
		PartitionBy:     et.Spec.PartitionBy,
		PartitionType:   et.Spec.PartitionType,
		Pattern:         et.Spec.Pattern,
		RefreshOnCreate: et.Spec.RefreshOnCreate,
		AutoRefresh:     et.Spec.AutoRefresh,
		AwsSnsTopic:     et.Spec.AwsSnsTopic,
		TableFormat:     et.Spec.TableFormat,
		Integration:     et.Spec.Integration,
		Comment:         et.Spec.Comment,
	}

	for _, col := range et.Spec.Columns {
		opts.Columns = append(opts.Columns, snowflake.ExternalTableColumnOpt{
			Name: col.Name,
			Type: col.Type,
			As:   col.As,
		})
	}

	return opts
}

func buildAlterOptions(et *snowplanev1alpha1.ExternalTable, id snowflake.SchemaObjectIdentifier, obs *snowflake.ExternalTableObservation) snowflake.AlterExternalTableOptions {
	opts := snowflake.AlterExternalTableOptions{Name: id}

	// AUTO_REFRESH is the only mutable field.
	// Since it uses nounset, ComputeUnset will never produce AUTO_REFRESH.
	// We only SET when spec differs from observed.
	if et.Spec.AutoRefresh != nil {
		if obs == nil || obs.ShowOutput == nil {
			opts.AutoRefresh = et.Spec.AutoRefresh
		} else {
			// Compare with SHOW output. auto_refresh is not directly in SHOW EXTERNAL TABLES,
			// so we always set if specified.
			opts.AutoRefresh = et.Spec.AutoRefresh
		}
	}

	return opts
}

func detectDrift(et *snowplanev1alpha1.ExternalTable, obs *snowflake.ExternalTableObservation) *drift.Result {
	d := drift.New()

	if obs.ShowOutput != nil {
		// Immutable fields.
		d.CompareStringValueFold("NAME", et.Spec.Name, obs.ShowOutput.Name, true)
		d.CompareStringValueFold("DATABASE", snowflake.ParseDatabaseNameFromFQN(et.Status.DatabaseName), obs.ShowOutput.DatabaseName, true)
		d.CompareStringValueFold("SCHEMA", snowflake.ParseSchemaNameFromFQN(et.Status.SchemaName), obs.ShowOutput.SchemaName, true)

		// Comment is immutable but we still detect drift.
		if et.Spec.Comment != nil {
			observedComment := obs.ShowOutput.Comment
			d.CompareString("COMMENT", et.Spec.Comment, observedComment, true)
		}

		// Location drift.
		if et.Spec.Location != "" && obs.ShowOutput.Location != "" {
			// Location in SHOW output may differ in format from spec; normalize for comparison.
			if !strings.EqualFold(et.Spec.Location, obs.ShowOutput.Location) {
				d.CompareStringValue("LOCATION", et.Spec.Location, obs.ShowOutput.Location, true)
			}
		}
	}

	return d.Result()
}
