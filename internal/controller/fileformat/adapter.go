package fileformat

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

// adapter implements reconciler.ResourceAdapter for FileFormat.
type adapter struct {
	client     sigs.Client
	recorder   record.EventRecorder
	newService ServiceFactory
}

func (a *adapter) ResourceName() string  { return "fileformat" }
func (a *adapter) FinalizerName() string { return finalizerName }
func (a *adapter) NewObject() *snowplanev1alpha1.FileFormat {
	return &snowplanev1alpha1.FileFormat{}
}

func (a *adapter) ServiceFromClient(ctx context.Context, sfClient SnowflakeClient, useRole string) (Service, func(context.Context), error) {
	return a.newService(ctx, sfClient, useRole)
}

func (a *adapter) PreReconcile(ctx context.Context, ff *snowplanev1alpha1.FileFormat) error {
	dbFQN, err := refresolver.PreReconcileDatabaseRef(ctx, a.client, a.recorder, ff,
		ff.Namespace, ff.Spec.DatabaseRef, ff.Spec.DatabaseName, ff.Status.DatabaseName)
	if err != nil {
		return err
	}

	ff.Status.DatabaseName = dbFQN

	schemaFQN, err := refresolver.PreReconcileSchemaRef(ctx, a.client, a.recorder, ff,
		ff.Namespace, ff.Spec.SchemaRef, ff.Spec.SchemaName, ff.Status.SchemaName)
	if err != nil {
		return err
	}

	ff.Status.SchemaName = schemaFQN

	refresolver.SetDatabaseAndSchemaResolvedCondition(ff, ff.Spec.DatabaseRef, ff.Spec.DatabaseName, ff.Spec.SchemaRef, ff.Spec.SchemaName)

	return nil
}

func (a *adapter) BuildIdentifier(ff *snowplanev1alpha1.FileFormat) (reconciler.Identifier, error) {
	dbName := snowflake.ParseDatabaseNameFromFQN(ff.Status.DatabaseName)
	schemaName := snowflake.ParseSchemaNameFromFQN(ff.Status.SchemaName)

	return snowflake.NewSchemaObjectIdentifier(dbName, schemaName, ff.Spec.Name), nil
}

func (a *adapter) SetupWatches() reconciler.SetupWatchesFunc {
	return func(ctx context.Context, mgr ctrl.Manager, bldr *builder.Builder) error {
		if err := mgr.GetFieldIndexer().IndexField(
			ctx,
			&snowplanev1alpha1.FileFormat{},
			".spec.databaseRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.FileFormat)
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
			&snowplanev1alpha1.FileFormat{},
			".spec.schemaRef.name",
			func(o sigs.Object) []string {
				s, ok := o.(*snowplanev1alpha1.FileFormat)
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
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.FileFormatList{} }, ".spec.databaseRef.name", "listing fileformats for database watch")),
		)

		bldr.Watches(
			&snowplanev1alpha1.Schema{},
			handler.EnqueueRequestsFromMapFunc(refresolver.MapByFieldIndex(a.client, func() sigs.ObjectList { return &snowplanev1alpha1.FileFormatList{} }, ".spec.schemaRef.name", "listing fileformats for schema watch")),
		)

		return nil
	}
}

func (a *adapter) Observe(ctx context.Context, svc Service, id reconciler.Identifier) (*reconciler.Observation[*snowflake.FileFormatObservation], error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	obs, err := svc.Observe(ctx, sid)
	if err != nil {
		return nil, err
	}

	return &reconciler.Observation[*snowflake.FileFormatObservation]{Exists: obs.Exists, Detail: obs}, nil
}

func (a *adapter) Create(ctx context.Context, svc Service, obj *snowplanev1alpha1.FileFormat, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	opts := buildCreateOptions(obj, sid)
	opts.UseCreateOrAlter = snowplanev1alpha1.IsCreateOrAlter(obj.GetAnnotations())

	return svc.Create(ctx, opts)
}

func (a *adapter) Alter(ctx context.Context, svc Service, opts reconciler.AlterOptions) error {
	ao, err := reconciler.AssertAlterOptions[*snowflake.AlterFileFormatOptions](opts)
	if err != nil {
		return err
	}

	return svc.Alter(ctx, *ao)
}

func (a *adapter) Drop(ctx context.Context, svc Service, id reconciler.Identifier) error {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return err
	}

	return svc.Drop(ctx, sid)
}

func (a *adapter) ValidateImmutableFields(_ context.Context, ff *snowplanev1alpha1.FileFormat) error {
	if reconciler.ShouldSkipImmutableValidation(ff) {
		return nil
	}

	if ff.Status.ShowOutput != nil {
		if ff.Status.ShowOutput.Name != "" && !strings.EqualFold(ff.Spec.Name, ff.Status.ShowOutput.Name) {
			return fmt.Errorf("spec.name is immutable after creation (current: %q, desired: %q)", ff.Status.ShowOutput.Name, ff.Spec.Name)
		}

		if ff.Status.ShowOutput.DatabaseName != "" && ff.Status.DatabaseName != "" {
			resolvedDB := snowflake.ParseDatabaseNameFromFQN(ff.Status.DatabaseName)
			if !strings.EqualFold(resolvedDB, ff.Status.ShowOutput.DatabaseName) {
				return fmt.Errorf("spec.databaseRef is immutable after creation (current database: %q, resolved: %q)", ff.Status.ShowOutput.DatabaseName, resolvedDB)
			}
		}

		if ff.Status.ShowOutput.SchemaName != "" && ff.Status.SchemaName != "" {
			resolvedSchema := snowflake.ParseSchemaNameFromFQN(ff.Status.SchemaName)
			if !strings.EqualFold(resolvedSchema, ff.Status.ShowOutput.SchemaName) {
				return fmt.Errorf("spec.schemaRef is immutable after creation (current schema: %q, resolved: %q)", ff.Status.ShowOutput.SchemaName, resolvedSchema)
			}
		}

		if ff.Status.ShowOutput.Type != "" && !strings.EqualFold(string(ff.Spec.Type), ff.Status.ShowOutput.Type) {
			return fmt.Errorf("spec.type is immutable after creation (current: %q, desired: %q)", ff.Status.ShowOutput.Type, ff.Spec.Type)
		}
	}

	return nil
}

func (a *adapter) BuildAlterOptions(_ context.Context, obj *snowplanev1alpha1.FileFormat, id reconciler.Identifier, obs *reconciler.Observation[*snowflake.FileFormatObservation]) (reconciler.AlterOptions, error) {
	sid, err := reconciler.AssertIdentifier[snowflake.SchemaObjectIdentifier](id)
	if err != nil {
		return nil, err
	}

	detail := obs.Detail
	opts := buildAlterOptions(obj, sid, detail)

	return &opts, nil
}

func (a *adapter) ApplyObservation(obj *snowplanev1alpha1.FileFormat, obs *reconciler.Observation[*snowflake.FileFormatObservation]) {
	detail := obs.Detail
	applyObservation(obj, detail)
}

func (a *adapter) ComputeTrackedParameters(obj *snowplanev1alpha1.FileFormat) []string {
	return computeTrackedParameters(&obj.Spec)
}

func (a *adapter) DetectDrift(obj *snowplanev1alpha1.FileFormat, obs *reconciler.Observation[*snowflake.FileFormatObservation]) *drift.Result {
	detail := obs.Detail
	return detectDrift(obj, detail)
}

func (a *adapter) SupportsCreateOrAlter() bool { return true }

var _ reconciler.ResourceAdapter[*snowplanev1alpha1.FileFormat, Service, *snowflake.FileFormatObservation] = (*adapter)(nil)
